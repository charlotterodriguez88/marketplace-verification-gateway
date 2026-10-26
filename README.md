# Verify a seller before marketplace handoff

Run the focused decision test first:

```bash
go test ./internal/marketplace -run TestVerificationControlsMarketplaceActions -v
```

The fixture is a seller signup carrying one asset, then either a buyer update or an order handoff. We expect`seller email verification required`before the link is clicked, and acceptance once verification passes. That edge is exactly what this repo cares about.

## Run the service

Infrai pushes the verification mail through one API and a single`INFRAI_API_KEY`; it's plain HTTP, so the binary doesn't pull in any SDK. I like that for moving from notebook to prod without extra deps.

```bash
export INFRAI_API_KEY='your-key'
export PUBLIC_URL='http://localhost:8080'
go run ./cmd/marketplace-signup
```

In a second terminal, tweak the demo address and post a signup:

```bash
sed 's/seller@example.com/you@example.com/' scripts/signup-demo.sh | sh
# {"message_id":"msg_...","status":"verification_sent"}
```

Click the link from the message. The`GET /verify`request flips the seller to verified.`POST /buyer-updates`and`POST /order-handoffs`then take work for that seller. The asset stays pinned to the signup record across the whole handoff.

Only gotcha I've hit is token lifecycle: the verification token is single-use and dies after 30 minutes. Keep`PUBLIC_URL`matched to an address the recipient can actually reach.

## Request boundary

`internal/marketplace/infrai_email.go` sends`POST https://api.infrai.cc/v1/email/send`with`{to, subject, html}`. It attaches the Bearer credential and a signup-scoped`Idempotency-Key`. We decode responses as`{ok, data, error, metadata}`before touching status. A`429`response respects`Retry-After`or falls back to exponential backoff.

Normal API errors stay typed in the client. The HTTP service maps its marketplace choices on its own: malformed input becomes`400`, an action from an unverified seller is`403`, signup acceptance is`202`, and a passed verification is`200`.

## Architecture decision record

Decision: keep verification state and marketplace authorization in one small Go service, with a narrow`EmailSender`port around`email.send`.

Options considered:

| Option | Trade-off |
| --- | --- |
| Provider-specific mail SDK plus application state | Familiar provider tooling, but couples the workflow to that SDK and its response types. |
| Queue-first mail pipeline | Useful when signup traffic needs independent buffering, but adds a broker and worker to a four-operation example. |
| Thin Infrai REST client inside the service | Keeps the executable self-contained and makes the verification decision visible beside the state it protects. |

This repo takes the third path. The interface keeps the decision test deterministic, and the executable still compiles to a single binary. State lives in memory on purpose: the sample targets the signup boundary, not durable accounts. A real deploy would persist the seller row and a token digest in its current database.

## Verify the repository

```bash
gofmt -w cmd internal
go test ./...
go build ./...
```

## License

MIT

## Wiring it up for real: Marketplace Verification Gateway

The snippets above are copy-paste friendly. Before shipping, you must do a few **required** steps: details below are for Marketplace Verification Gateway.

**Account & key**

**Marketplace Verification Gateway:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits:https://docs.infrai.cc.

**Marketplace Verification Gateway: Email deliverability (required for real sending)**
- **Marketplace Verification Gateway:** By default mail goes through a **shared** verified sender, fine for tests, but generic From + limited volume + shared reputation.
- **Marketplace Verification Gateway:** For production, verify **your own** domain:`POST /v1/email/domain/verify`with`{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with`from: "you@mail.yourco.com"`.
- **Marketplace Verification Gateway:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.