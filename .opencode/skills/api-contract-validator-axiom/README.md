# api-contract-validator-axiom

OpenAPI/AsyncAPI spec generation, contract drift detection, negative testing with schemathesis, API versioning discipline, and contract validation verdicts. Load this skill when working with HTTP APIs, event-driven systems, or any surface that exposes a machine-readable contract. Enforces the Axiom requirement that openapi.json must stay in sync with implementation.

## When to Load

Use AsyncAPI (not OpenAPI) for:
- Server-Sent Events (SSE) streams
- WebSocket bidirectional channels
- Message queue topics (MQTT, NATS, Kafka)
- Any publish/subscribe or event-driven interface

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
