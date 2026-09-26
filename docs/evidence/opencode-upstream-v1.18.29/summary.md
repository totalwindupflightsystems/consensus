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

| # | Actual upstream suite | Exit | Tests | Pass | Fail | Full log |
|---:|---|---:|---:|---:|---:|---|
| 1 | `packages/opencode/test/server/httpapi-instance.test.ts` | 1 | unknown | 0 | 0 | [packages_opencode_test_server_httpapi-instance_test_ts.log](packages_opencode_test_server_httpapi-instance_test_ts.log) |
| 2 | `packages/opencode/test/server/httpapi-sdk.test.ts` | 1 | unknown | 0 | 0 | [packages_opencode_test_server_httpapi-sdk_test_ts.log](packages_opencode_test_server_httpapi-sdk_test_ts.log) |
| 3 | `packages/opencode/test/server/sdk-error-shape.test.ts` | 1 | unknown | 0 | 0 | [packages_opencode_test_server_sdk-error-shape_test_ts.log](packages_opencode_test_server_sdk-error-shape_test_ts.log) |
| 4 | `packages/client/test/promise.test.ts` | 1 | unknown | 0 | 0 | [packages_client_test_promise_test_ts.log](packages_client_test_promise_test_ts.log) |

## Explicit divergences

The following upstream-owned assertions failed. They are compatibility gaps, not skips:

### `packages/opencode/test/server/httpapi-instance.test.ts`

Exit 1; tests=unknown, pass=0, fail=0. Full evidence: [packages_opencode_test_server_httpapi-instance_test_ts.log](packages_opencode_test_server_httpapi-instance_test_ts.log).

```text
```

### `packages/opencode/test/server/httpapi-sdk.test.ts`

Exit 1; tests=unknown, pass=0, fail=0. Full evidence: [packages_opencode_test_server_httpapi-sdk_test_ts.log](packages_opencode_test_server_httpapi-sdk_test_ts.log).

```text
```

### `packages/opencode/test/server/sdk-error-shape.test.ts`

Exit 1; tests=unknown, pass=0, fail=0. Full evidence: [packages_opencode_test_server_sdk-error-shape_test_ts.log](packages_opencode_test_server_sdk-error-shape_test_ts.log).

```text
```

### `packages/client/test/promise.test.ts`

Exit 1; tests=unknown, pass=0, fail=0. Full evidence: [packages_client_test_promise_test_ts.log](packages_client_test_promise_test_ts.log).

```text
```

