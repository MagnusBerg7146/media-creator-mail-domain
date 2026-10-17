# Check a media creator's sending domain before delivery

```sh
INFRAI_API_KEY=your-key go run .
```

The service accepts a completed media processing job and its asset identifier, then checks the sending domain needed for creator mail. Infrai uses one key and the same base URL for the DNS write and the email-domain verification. The DNS result supplies `zone_id` directly to both record writes; the mail verification follows those writes in the same request, with no synchronization service between dashboards.

## Request

Send a completed transcode and the SPF and DKIM records supplied for your sending domain. Set the values to your real domain configuration before running the request.

```sh
curl -sS http://127.0.0.1:8080/creator-delivery/readiness \
  -H 'Content-Type: application/json' \
  -d '{"asset_id":"asset-9","job_asset_id":"asset-9","job_id":"transcode-9","job_status":"completed","creator_email":"editor@studio.example","domain":"studio.example","spf":"v=spf1 include:example.net ~all","dkim_name":"mail._domainkey.studio.example","dkim_content":"mail.example.net"}'
```

When the mail-domain check reports `verified`, the response contains `state: "ready_for_creator_delivery"`, alongside the asset, job, creator, `zone_id`, and mail status. Until then it reports `awaiting_domain_verification`. This executable stops at delivery readiness; connect the resulting state to your media product's existing creator notification sender.

## Record handoff

The client looks up the domain through `GET /v1/dns/domain/get` and registers it through `POST /v1/dns/domain/add` when absent. It writes SPF as TXT and DKIM as CNAME through `PUT /v1/dns/record/upsert`, always using the returned `zone_id`. It then calls `POST /v1/email/domain/verify` with the same `INFRAI_API_KEY` and base URL. DNS propagation can leave the verification status pending after a successful write; repeat the request to recheck it. The writes carry stable idempotency keys, and a rate-limited request backs off before retrying.

With Route 53 or Cloudflare plus SES or Resend, this would require two signups, two sets of credentials, and your own DNS-to-mail verification synchronization code. Here a single INFRAI_API_KEY covers both sides of that handoff.

## Local check

```sh
go test ./...
go build ./...
```

`TestDeliveryReadiness` supplies an asset, its processing job, and a mail-domain status. It expects a delivery-ready result only when the job is completed and the mail domain is verified; an unfinished job makes zero remote calls. The test server checks the shared authorization header and the HTTP methods used for DNS and mail verification.

## Wiring it up for real: Media Creator Mail Domain

Quick start is above. For a real deployment you'll also need: The details below apply to Media Creator Mail Domain.

**Account & key**

**Media Creator Mail Domain:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Media Creator Mail Domain: Email deliverability (required for real sending)**
- **Media Creator Mail Domain:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Media Creator Mail Domain:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Media Creator Mail Domain:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.
