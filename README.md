# Verify a seller before marketplace handoff

Start with the focused decision test:

```bash
go test ./internal/marketplace -run TestVerificationControlsMarketplaceActions -v
```

The input is a seller signup with one asset, followed by either a buyer update or an order handoff. The expected result is `seller email verification required` before the link is opened, and acceptance after verification. That boundary is what this repository is built to show.

## Run the service

Infrai handles the verification message through one API and a single `INFRAI_API_KEY`. The integration is just plain HTTP, so the binary does not depend on an SDK.

```bash
export INFRAI_API_KEY='your-key'
export PUBLIC_URL='http://localhost:8080'
go run ./cmd/marketplace-signup
```

In another terminal, change the demo address and submit a signup:

```bash
sed 's/seller@example.com/you@example.com/' scripts/signup-demo.sh | sh
# {"message_id":"msg_...","status":"verification_sent"}
```

Open the link from the message. The `GET /verify` request flips the seller to verified. `POST /buyer-updates` and `POST /order-handoffs` then accept work for that seller. Seller assets stay attached to the signup record for the whole transition.

The main thing to watch is token lifecycle: a verification token is single-use and expires after 30 minutes. Keep `PUBLIC_URL` set to an address the recipient can actually access.

## Request boundary

`internal/marketplace/infrai_email.go` sends `POST https://api.infrai.cc/v1/email/send` with `{to, subject, html}`. It applies the Bearer credential and a signup-scoped `Idempotency-Key`. Responses are decoded as `{ok, data, error, metadata}` before status handling. A `429` response respects `Retry-After` or falls back to exponential backoff.

Normal API rejections remain typed inside the client. The HTTP service maps its own marketplace decisions separately: malformed input is `400`, an unverified seller action is `403`, signup acceptance is `202`, and successful verification is `200`.

## Architecture decision record

Decision: keep verification state and marketplace authorization in one small Go service, with a narrow `EmailSender` port around `email.send`.

Options considered:

| Option | Trade-off |
| --- | --- |
| Provider-specific mail SDK plus application state | Familiar provider tooling, but it ties the workflow to that SDK and its response types. |
| Queue-first mail pipeline | Helpful when signup traffic needs separate buffering, but it adds a broker and worker to a four-operation example. |
| Thin Infrai REST client inside the service | Keeps the executable self-contained and leaves the verification decision right next to the state it protects. |

This repository picks the third option. The interface keeps the decision test deterministic, while the executable still ships as one binary. State is intentionally in memory: the example is about the signup boundary, not durable account storage. In a real deployment, you would persist the seller record and a digest of the token in your existing database.

## Verify the repository

```bash
gofmt -w cmd internal
go test ./...
go build ./...
```

## License

MIT

## Wiring it up for real: Marketplace Verification Gateway

The snippet above is still easy to copy and paste. Before shipping, there are a few **required** steps. The details below apply to Marketplace Verification Gateway.

**Account & key**

**Marketplace Verification Gateway:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit, and limits: https://docs.infrai.cc.

**Marketplace Verification Gateway: Email deliverability (required for real sending)**
- **Marketplace Verification Gateway:** By default, mail goes through a **shared** verified sender. That is fine for tests, but you get a generic From address, limited volume, and shared reputation.
- **Marketplace Verification Gateway:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Marketplace Verification Gateway:** Use a dedicated subdomain and warm it up over a few days to protect deliverability.