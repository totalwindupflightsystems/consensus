# Pinned opencode upstream compatibility evidence

- Upstream: `https://github.com/anomalyco/opencode.git`
- Version: `1.18.29`
- Revision: `16747470f976aca3d362ad730bcd3fe82ecc2c9a`
- Lock SHA-256: `e4a33f0dce76bd625ceef88b2fe44cfa75d0653a3ccba0c7066ac558896ff239`
- Adapter patch SHA-256: `ac906416353261ce238a6bc9354c7a73151f52c9ac3006ecc143c93b4aa7f98c`
- Fetch preload SHA-256: `1fcfa053aa56896b7bf1141539f690fe1cb467a798e91e7763536b60d843bfac`
- Shim base URL: `http://127.0.0.1:18232`
- Invocation: `scripts/test-opencode-upstream.sh --base-url http://127.0.0.1:18232`

The source hashes were verified before the transport-only patch was applied.
No upstream assertion or test registration is edited by the adapter.

## Suite results

| # | Actual upstream suite | Exit | Tests | Pass | Fail | Classification | Full log |
|---:|---|---:|---:|---:|---:|---|---|
| 1 | `packages/opencode/test/server/httpapi-instance.test.ts` | 1 | 7 | 0 | 7 | DIVERGENCE | [packages_opencode_test_server_httpapi-instance_test_ts.log](packages_opencode_test_server_httpapi-instance_test_ts.log) |
| 2 | `packages/opencode/test/server/httpapi-sdk.test.ts` | 1 | 18 | 17 | 1 | DIVERGENCE | [packages_opencode_test_server_httpapi-sdk_test_ts.log](packages_opencode_test_server_httpapi-sdk_test_ts.log) |
| 3 | `packages/opencode/test/server/sdk-error-shape.test.ts` | 1 | 2 | 0 | 2 | DIVERGENCE | [packages_opencode_test_server_sdk-error-shape_test_ts.log](packages_opencode_test_server_sdk-error-shape_test_ts.log) |
| 4 | `packages/client/test/promise.test.ts` | 0 | 7 | 7 | 0 | PASS | [packages_client_test_promise_test_ts.log](packages_client_test_promise_test_ts.log) |

## Explicit divergences

The following upstream-owned assertions executed and failed. These are compatibility gaps, not skips:

### `packages/opencode/test/server/httpapi-instance.test.ts`

Exit 1; tests=7, pass=0, fail=7. Full evidence: [packages_opencode_test_server_httpapi-instance_test_ts.log](packages_opencode_test_server_httpapi-instance_test_ts.log).

```text
error: expect(received).toContain(expected)
Received: "text/html; charset=utf-8"
error: expect(received).toContain(expected)
Received: "text/html; charset=utf-8"
(fail) instance HttpApi > serves the OpenAPI document [1296.29ms]
error: expect(received).toBe(expected)
Expected: 200
Received: 401
error: expect(received).toBe(expected)
Expected: 200
Received: 401
(fail) instance HttpApi > emits a sync fence header for fixed-workspace mutations [487.29ms]
error: expect(received).toBe(expected)
Expected: 200
Received: 404
error: expect(received).toBe(expected)
Expected: 200
Received: 404
(fail) instance HttpApi > does not emit sync fence headers for fixed-workspace reads or no-op mutations [531.16ms]
error: expect(received).toBe(expected)
Expected: 400
Received: 401
error: expect(received).toBe(expected)
Expected: 400
Received: 401
(fail) instance HttpApi > rejects malformed permission and question request ids [525.03ms]
error: expect(received).toBe(expected)
Expected: 404
Received: 401
error: expect(received).toBe(expected)
Expected: 404
Received: 401
(fail) instance HttpApi > returns typed not found bodies for missing permission and question requests [527.78ms]
error: expect(received).toBe(expected)
Expected: 404
Received: 501
error: expect(received).toBe(expected)
Expected: 404
Received: 501
(fail) instance HttpApi > returns typed not found bodies for missing projects [507.50ms]
error: expect(received).toBe(expected)
Expected: 200
Received: 404
error: expect(received).toBe(expected)
Expected: 200
Received: 404
(fail) instance HttpApi > serves path and VCS read endpoints [657.01ms]
```

### `packages/opencode/test/server/httpapi-sdk.test.ts`

Exit 1; tests=18, pass=17, fail=1. Full evidence: [packages_opencode_test_server_httpapi-sdk_test_ts.log](packages_opencode_test_server_httpapi-sdk_test_ts.log).

```text
error: Received value must be an array type, or both received and expected values must be strings.
error: Received value must be an array type, or both received and expected values must be strings.
(fail) HttpApi SDK > includes project skills in REST API prompt context [2471.33ms]
```

### `packages/opencode/test/server/sdk-error-shape.test.ts`

Exit 1; tests=2, pass=0, fail=2. Full evidence: [packages_opencode_test_server_sdk-error-shape_test_ts.log](packages_opencode_test_server_sdk-error-shape_test_ts.log).

```text
error: expect(received).toContain(expected)
Received: "GET http://test/session/ses_no_such?directory=%2Ftmp%2Fopencode-test-99rlzjfl97i → 401 Unauthorized"
(fail) v2 SDK error shape > 404 with NamedError body throws a real Error carrying the server message [282.39ms]
error: expect(received).toBe(expected)
Expected: 400
Received: 404
(fail) v2 SDK error shape > 400 schema rejection: SDK extracts the field-level reason from the NamedError body [11.33ms]
```

