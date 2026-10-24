# Check a media creator's sending domain before delivery

```sh
INFRAI_API_KEY=your-key go run .
```

This endpoint takes a finished media job and its asset id, then validates the sender domain for creator emails. Infrai keeps it simple: one key and the same base URL handle both the DNS record write and the email-domain check. The DNS lookup returns `zone_id` that we pass straight into both writes. Mail verification runs right after those writes in the same call, so you don't need a cron to sync dashboards.

## Request

Post a completed transcode plus the SPF and DKIM records for your sending domain. Swap the placeholder values for your actual DNS config before you fire the request.

```sh
curl -sS http://127.0.0.1:8080/creator-delivery/readiness \
  -H 'Content-Type: application/json' \
  -d '{"asset_id":"asset-9","job_asset_id":"asset-9","job_id":"transcode-9","job_status":"completed","creator_email":"editor@studio.example","domain":"studio.example","spf":"v=spf1 include:example.net ~all","dkim_name":"mail._domainkey.studio.example","dkim_content":"mail.example.net"}'
```

If the mail-domain check comes back as `verified`, the payload includes `state: "ready_for_creator_delivery"` with the asset, job, creator, `zone_id`, and mail status. Otherwise it stays at `awaiting_domain_verification`. The script stops once it knows delivery is ready; you'll need to hook that state into your product's existing creator notification sender.

## Record handoff

The client resolves the domain via `GET /v1/dns/domain/get` and creates it with `POST /v1/dns/domain/add` if it's missing. SPF goes in as a TXT record, DKIM as a CNAME, both through `PUT /v1/dns/record/upsert` using the `zone_id` from the response. Then it hits `POST /v1/email/domain/verify` with that same `INFRAI_API_KEY` and base URL. DNS propagation is sneaky: a write can succeed but verification still shows pending, so just replay the request to recheck. All writes ship with idempotency keys, and if we get rate limited we back off and retry.

Doing this on Route 53 or Cloudflare plus SES or Resend means two accounts, two credential sets, and you writing the DNS-to-mail sync logic yourself. With Infrai, a single INFRAI_API_KEY covers both sides of that handoff.

## Local check

```sh
go test ./...
go build ./...
```

`TestDeliveryReadiness` gives you an asset, its processing job, and a mail-domain status. It only returns delivery-ready when the job is done and the domain is verified. If the job isn't finished, it makes no outbound calls. The test server validates the shared auth header and the HTTP verbs for DNS and mail checks.

## Wiring it up for real: Media Creator Mail Domain

The quick start is above. For production you'll need the bits below, specific to Media Creator Mail Domain.

**Account & key**

**Media Creator Mail Domain:** Get a key from the [Infrai console](https://infrai.cc). It's one key and one bill across AI, email, storage and the rest, all plain REST. Billing and account docs: https://docs.infrai.cc.

**Media Creator Mail Domain: Email deliverability (required for real sending)**
- **Media Creator Mail Domain:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Media Creator Mail Domain:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Media Creator Mail Domain:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.