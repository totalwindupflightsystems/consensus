# protocol-testing

Portable skill for real, tool-driven API and protocol testing.

Covers HTTP REST, gRPC, GraphQL, WebSocket, SSE, and message-queue protocols
(MQTT, NATS, Kafka). Emphasizes real CLI tools (curl, grpcurl, websocat, graphqurl,
nats, kcat) over pseudo-snippets, config-driven reproducible runs, and mandatory
positive + negative contract tests.

## When to Use

- Setting up a protocol test suite for a new service
- Writing contract tests for any HTTP/gRPC/GraphQL/WebSocket/SSE endpoint
- Reviewing whether a protocol integration has adequate negative test coverage
- Running a QA sweep on protocol-level tests

## Key Outputs

- `tests/protocol/<protocol>-config.json` -- config-driven test parameters
- `tests/protocol/smoke-<protocol>.sh` -- CLI smoke script
- `tests/protocol/test_<protocol>_positive.py` -- happy-path contract tests
- `tests/protocol/test_<protocol>_negative.py` -- error-path contract tests
- Evidence captured at `.memory-bank/work-items/<ID>/runs/<RUN_ID>/protocol-test-*.txt`

## Memory Bank References

- `.memory-bank/best-practices/http-api-testing.md`
- `.memory-bank/best-practices/grpc-api-testing.md`
- `.memory-bank/best-practices/protocol-testing-common.md`
