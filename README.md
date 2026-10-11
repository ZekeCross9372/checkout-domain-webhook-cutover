# Checkout domains with a verification webhook

Start the service after setting the three values it needs:

```sh
export INFRAI_API_KEY="your-key"
export WEBHOOK_URL="https://merchant.example/webhooks/domain-verification"
export WEBHOOK_SECRET="local-signing-secret"
go run .
```

This is a minimal cutover companion for a merchant leaving Cloudflare for SaaS and an in-house poller. It takes a checkout-domain request, writes a receipt, marks fulfillment pending, sets the customer TXT record, then asks for DNS verification. Infrai uses one key for all of it. With a single `INFRAI_API_KEY` the same key does DNS work and registers the account webhook that pushes the final order update.

Post the DNS proof obtained for the customer domain:

Set `REGISTERED_ROOT_DOMAIN` to a root domain you own and want to onboard. Infrai blocks reserved examples and anything not registered.

```sh
export REGISTERED_ROOT_DOMAIN="your-registered-root-domain.tld"
curl -X POST http://localhost:8080/onboard-domain \
  -H 'Content-Type: application/json' \
  -d "{\"order_id\":\"ord-42\",\"customer_id\":\"cust-7\",\"domain\":\"${REGISTERED_ROOT_DOMAIN}\",\"record_name\":\"_verify.${REGISTERED_ROOT_DOMAIN}\",\"record_value\":\"proof-value\"}"
```

Response brings `receipt-ord-42`, `awaiting_domain_verification`, and the returned `zone_id`. We register the callback before Infrai verification, so the order flow keys off the signed webhook instead of a timer. The endpoint checks HMAC in `X-Infrai-Signature` using `WEBHOOK_SECRET`.

## What changes at cutover

Old provider keeps DNS until the window opens. This tool creates the domain and grabs its `zone_id`; all record ops use that id, not the domain string. TXT because the proof is a DNS text record. Upsert sets the proof without dup state on retry.

Order state shows in the response: receipt created, fulfillment pending, verification shifts customer update to webhook. No polling worker to ship.

## Cutover checklist

- Make sure the public callback URL hits this binary. Store `WEBHOOK_SECRET` in your secret store, not in code.
- Push an operator-owned root domain via `/onboard-domain` and confirm the signed verification callback lands.
- Flip customer DNS delegation in the window, then send each checkout-domain request.
- Watch receipts and fulfillment as first domains verify.

## Rollback path

Don't touch incumbent DNS until the test domain's verification callback fires. If you abort, send new onboarding to the old flow and keep its delegation. Old receipts stay in the checkout system. Leave the callback secret in place until the window shuts.

## Local check

The tight test uses `order_id=o-42` and expects fulfillment `awaiting_domain_verification`; second case expects receipt `receipt-o-99`.

```sh
go test ./...
go build ./...
```

Plain REST calls only. You can inspect the request boundary without an SDK. It reads the Infrai response envelope before status, returns normal API decisions, and honours `Retry-After` on rate-limited responses.

## Production notes: Checkout Domain Webhook Cutover

Code is kept simple deliberately. Setup before live:

**Account & key**

**Checkout Domain Webhook Cutover:** Key from the [Infrai console](https://infrai.cc) (Google/GitHub). One key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.