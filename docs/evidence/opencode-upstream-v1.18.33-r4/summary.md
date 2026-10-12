# Pinned opencode upstream compatibility evidence

- Upstream: `https://github.com/anomalyco/opencode.git`
- Version: `1.18.33`
- Revision: `7945de208964a49300d7f770d1a71d078db9a4c4`
- Lock SHA-256: `b68e7ece1128eb383663e55ffca4f9f08e451530cc09fc9cf84c489bf41bdc2a`
- Adapter patch SHA-256: `ac906416353261ce238a6bc9354c7a73151f52c9ac3006ecc143c93b4aa7f98c`
- Fetch preload SHA-256: `6317fd87b98612d446eb02f1a69a011c63e0cddf4bb5ed36ea5bced495df8f49`
- Shim base URL: `http://127.0.0.1:18242`
- Invocation: `scripts/test-opencode-upstream.sh --base-url http://127.0.0.1:18242`

The source hashes were verified before the transport-only patch was applied.
No upstream assertion or test registration is edited by the adapter.

## Suite results

| # | Actual upstream suite | Exit | Tests | Pass | Fail | Classification | Full log |
|---:|---|---:|---:|---:|---:|---|---|
| 1 | `packages/opencode/test/server/httpapi-instance.test.ts` | 0 | 7 | 7 | 0 | PASS | [packages_opencode_test_server_httpapi-instance_test_ts.log](packages_opencode_test_server_httpapi-instance_test_ts.log) |
| 2 | `packages/opencode/test/server/httpapi-sdk.test.ts` | 0 | 18 | 18 | 0 | PASS | [packages_opencode_test_server_httpapi-sdk_test_ts.log](packages_opencode_test_server_httpapi-sdk_test_ts.log) |
| 3 | `packages/opencode/test/server/sdk-error-shape.test.ts` | 0 | 2 | 2 | 0 | PASS | [packages_opencode_test_server_sdk-error-shape_test_ts.log](packages_opencode_test_server_sdk-error-shape_test_ts.log) |
| 4 | `packages/client/test/promise.test.ts` | 0 | 7 | 7 | 0 | PASS | [packages_client_test_promise_test_ts.log](packages_client_test_promise_test_ts.log) |

## Explicit divergences

None. All named upstream suites passed against the shim.
