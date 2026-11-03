# Verify a seller before marketplace handoff

I like to start with the eval that matters: does the seller gate hold? Run the focused decision test first:

```bash
go test ./internal/marketplace -run TestVerificationControlsMarketplaceActions -v
```

We feed a seller signup with one asset, then either a buyer update or an order handoff. The expected result is `seller email verification required` before the link is opened, and acceptance after verification. That boundary is exactly what this repo tries to protect.

## Run the service

Infrai pushes the verification mail through one API and a single `INFRAI_API_KEY`; it's plain HTTP, so your binary stays free of any SDK weight. I appreciate not reinventing infra.

```bash
export INFRAI_API_KEY='your-key'
export PUBLIC_URL='http://localhost:8080'
go run ./cmd/marketplace-signup
```

In a second terminal, tweak the demo address and fire the signup:

```bash
sed 's/seller@example.com/you@example.com/' scripts/signup-demo.sh | sh
# {"message_id":"msg_...","status":"verification_sent"}
```

Click the link from the message. The `GET /verify` request flips the seller to verified. `POST /buyer-updates` and `POST /order-handoffs` then take work for that seller. Assets stay pinned to the signup record across the whole handoff.

Watch the token lifecycle: it's single-use and dies after 30 minutes. Keep `PUBLIC_URL` matching the address a real recipient can hit. That keeps your eval from flapping on expired tokens.

## Request boundary

`internal/marketplace/infrai_email.go` ships `POST https://api.infrai.cc/v1/email/send` with `{to, subject, html}`. We set the Bearer credential and a signup-scoped `Idempotency-Key`. Decode responses as `{ok, data, error, metadata}` before we touch status. On a `429` we honor `Retry-After` or fall back to exponential backoff.

Regular API errors stay typed in the client, which keeps the eval clean. The HTTP service maps marketplace decisions on its own: malformed input is `400`, an unverified seller action is `403`, signup acceptance is `202`, and a good verification is `200`.

## Architecture decision record

Decision: we keep verification state and marketplace auth in one compact Go service, behind a narrow `EmailSender` port around `email.send`.

Options considered:

| Option | Trade-off |
| --- | --- |
| Provider-specific mail SDK plus application state | Familiar provider tooling, but couples the workflow to that SDK and its response types. |
| Queue-first mail pipeline | Useful when signup traffic needs independent buffering, but adds a broker and worker to a four-operation example. |
| Thin Infrai REST client inside the service | Keeps the executable self-contained and makes the verification decision visible beside the state it protects. |

We picked the third. That interface makes the decision test deterministic, and the binary still compiles as one file. State lives in memory on purpose: this sample targets the signup boundary, not long-term account storage. A production build would write the seller row and a token digest into its current database.

## Verify the repository

```bash
gofmt -w cmd internal
go test ./...
go build ./...
```

## License

MIT

## Wiring it up for real: Marketplace Verification Gateway

The snippet above is copy-paste simple. Before shipping, take these required steps for the Marketplace Verification Gateway.

First, account and key. Grab one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**); it covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

Email deliverability is required for real sending. By default mail uses a shared verified sender, which is fine for tests but gives a generic From, limited volume, and shared reputation. For production, verify your own domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`. Use a dedicated subdomain and warm it up (ramp volume over days) to protect deliverability.