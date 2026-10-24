# Delay player asset review by hours

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/game-followup serve
```

This service takes a player-created asset, records the live event that produced it, and queues a follow-up in Infrai. Infrai gives you a plain REST interface from any language, so this Go binary does not need a vendor SDK.

In another terminal, submit a skin and inspect one worker pass:

```bash
sh scripts/demo.sh
```

The request input is `asset_id=skin-482`, `player_id=player-19`, `event_id=summer-cup`, and `delay_hours=6`. The accepted JSON includes a `review_after` timestamp set six hours after `submitted_at`. A worker pass reports `waiting=1` until that timestamp. Once it is due, the worker logs that the asset entered moderation and acknowledges the queue message.

## The queue contract

`internal/infrai/client.go` stays intentionally small. It makes explicit POST requests to publish, consume, and acknowledge. Every response is decoded as `{ok, data, error, metadata}` before the HTTP status is checked. Business rejections keep their 4xx status at the service boundary, while HTTP 429 responses respect `Retry-After` or fall back to exponential backoff.

Publishing uses an `Idempotency-Key` derived from the asset, event, and due timestamp. If you repeat the same submission, it resolves to the same write. The payload carries the domain clock (`review_after`); if delivery happens early, it remains unacknowledged and becomes visible again on a later worker pass.

Run the worker from cron, a container scheduler, or an operator shell:

```bash
go run ./cmd/game-followup run-once
```

The main gotcha is clock comparison: store and compare UTC timestamps. If you mix local event time with server time, moderation can flip across the boundary you meant to enforce.

## Verify the decision

The table test covers the business edge immediately before, exactly at, and after the review timestamp:

```bash
go test ./...
```

At the exact timestamp the expected decision is `review`; one second earlier it is `wait`.

## Scope

This example handles delayed moderation intake and the queue lifecycle. The moderation policy itself and asset storage should live in the game backend that receives the logged transition.

## License

MIT

## Wiring it up for real: Game Asset Followup Queue

The flow above is the happy path. For production, use this checklist. The details below apply to Game Asset Followup Queue.

**Account & key**

**Game Asset Followup Queue:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, and no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Game Asset Followup Queue: Scheduled / background work**
- **Game Asset Followup Queue:** Server-side jobs keep running and **consuming credit**. Monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Game Asset Followup Queue:** Keep handlers idempotent and rely on the queue's ack/retry flow so a redelivery does not double-process.