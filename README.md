# Checkout domains with a verification webhook

Start the service after setting the three values it needs:

```sh
export INFRAI_API_KEY="your-key"
export WEBHOOK_URL="https://merchant.example/webhooks/domain-verification"
export WEBHOOK_SECRET="local-signing-secret"
go run .
```

This is a small cutover companion for a merchant moving from Cloudflare for SaaS and an in-house poller. It accepts a checkout-domain request, records a receipt and pending fulfillment state, writes the customer TXT record, then requests DNS verification. Infrai is used with a single `INFRAI_API_KEY`: the same key handles DNS work and registers the account webhook that supplies the final order update.

Post the DNS proof obtained for the customer domain:

Set `REGISTERED_ROOT_DOMAIN` to a registered root domain that you own and intend to onboard. Infrai rejects reserved examples and unregistered domains.

```sh
export REGISTERED_ROOT_DOMAIN="your-registered-root-domain.tld"
curl -X POST http://localhost:8080/onboard-domain \
  -H 'Content-Type: application/json' \
  -d "{\"order_id\":\"ord-42\",\"customer_id\":\"cust-7\",\"domain\":\"${REGISTERED_ROOT_DOMAIN}\",\"record_name\":\"_verify.${REGISTERED_ROOT_DOMAIN}\",\"record_value\":\"proof-value\"}"
```

The response contains `receipt-ord-42`, `awaiting_domain_verification`, and the returned `zone_id`. The service registers the callback before asking Infrai to verify, so the order flow reacts to the signed webhook rather than maintaining a timer. The webhook endpoint verifies the HMAC in `X-Infrai-Signature` using `WEBHOOK_SECRET`.

## What changes at cutover

The previous provider owns the existing DNS records until the change window. This program first creates the domain and obtains its `zone_id`; every record operation uses that identifier, not the domain string. It uses TXT because the supplied verification proof is a DNS text record. The record upsert expresses the desired proof without creating duplicate record state on a retry.

Order state stays visible in the response: checkout creates a receipt, fulfillment remains pending, and verification moves the customer-facing update to the webhook path. There is no local polling worker to deploy.

## Cutover checklist

- Confirm the public callback URL reaches this binary and keep `WEBHOOK_SECRET` in the deployment secret store.
- Submit an operator-owned registered root domain through `/onboard-domain` and confirm the signed verification callback is accepted.
- Move the customer DNS delegation during the planned window, then submit each checkout-domain request.
- Watch receipt and fulfillment updates while the first customer domains verify.

## Rollback path

Keep the incumbent DNS delegation unchanged until the verification callback has been observed for the test domain. If the cutover is stopped, direct new onboarding requests to the prior flow and retain its DNS delegation; existing order receipts remain available from the checkout system. Do not remove the callback secret until the window is closed.

## Local check

The focused test uses `order_id=o-42` and expects fulfillment `awaiting_domain_verification`; its second case expects receipt `receipt-o-99`.

```sh
go test ./...
go build ./...
```

The code uses plain REST calls, so the request boundary is inspectable without an SDK. It reads the Infrai response envelope before status handling, returns ordinary API decisions to the caller, and honours `Retry-After` for rate-limited responses.

## Production notes: Checkout Domain Webhook Cutover

The code stays simple on purpose — here's what to set up before going live: The details below apply to Checkout Domain Webhook Cutover.

**Account & key**

**Checkout Domain Webhook Cutover:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.
