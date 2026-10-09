# Generating Clients from Consensus with Muster (MUSTER-INT-001)

[muster](https://github.com/wojons/muster) turns any OpenAPI spec into a CLI, an
MCP server, and a Go library with no hand-written glue. Consensus serves a
muster-ready spec on every running instance — this document shows the exact
commands.

## Where the spec lives

| Surface | URL | Notes |
|---|---|---|
| JSON spec | `GET /openapi.json` | Point muster here |
| YAML spec | `GET /openapi.yaml` | Byte-equivalent bundle |
| Swagger UI | `GET /doc/api` | Interactive browser (REST API) |
| Shim contract | `GET /doc` | opencode-compatible surface, separate document |

The served contract is **embedded in the binary at build time** from
`specs/openapi/bundled.yaml` (C-GAP-039): every instance serves the same spec
from any working directory, and the Docker image needs no specs/ copy. To
publish a changed contract: edit `specs/openapi/openapi.yaml` + `paths/` +
`components/`, run `make bundle-spec`, rebuild.

## Quick start

```bash
# 1. Point muster at a running consensus instance
openapi-cli discover http://localhost:8090/openapi.json   # inspect operations
openapi-cli generate http://localhost:8090/openapi.json   # register commands
openapi-cli --help                                        # generated commands appear

# 2. Store your Consensus admin key once (cs_ak_... from server bootstrap)
openapi-cli auth add --name consensus --type bearer --value cs_ak_...

# 3. Run generated commands
openapi-cli health-check --base-url http://localhost:8090
openapi-cli list-sessions --auth consensus --base-url http://localhost:8090
openapi-cli create-session --agent_name my-agent --goal "..." \
  --auth consensus --base-url http://localhost:8090
```

The `--base-url` flag overrides the spec's default server
(`http://localhost:8090`); the spec also declares a parameterized
`https://{host}` entry for remote deployments. Both are absolute URLs — muster
DF-015 (relative servers make generated commands die with
`unsupported protocol scheme`) cannot occur.

## What muster builds from the spec

- **CLI** — one command per `operationId` (`listSessions` → `list-sessions`,
  `mcpStreamablePost` → `m-c-p` streamable post), with per-parameter flags from
  the spec's parameter/requestBody schemas.
- **MCP server** — the same operations exposed as MCP tools for AI agents.
- **Go library** — `github.com/wojons/muster/pkg/...` parses the same URL
  programmatically (`inventory`/`openapi` packages).

The opencode-shim paths (`/session`, `/find/*`, `/project/*`, `/tui/*`,
`/vcs*`, `/agent`, …) are part of the same document and generate the same way;
their error responses carry schemas (`OpencodeBadRequest`,
`NotImplementable`, the standard error envelope) so generated clients can
decode every arm the server can write.

## Compatibility guarantees (enforced by tests)

`internal/api/openapi_muster_test.go` pins the muster-facing contract on the
SERVED spec (not the on-disk file):

1. every operation has a non-empty **unique** `operationId` (command names
   never collide);
2. every operation declares a success (2xx) response, except declared stubs
   (`x-not-implemented: true`: bare-`/mcp` management, `/vcs/{vcsId}`) and
   fail-only lookups with a served sibling method (`GET /tui/{action}` — the
   real contract is the POST on the same path);
3. every response that carries a body — success and error arms — resolves to a
   schema (202/204 are honestly bodyless);
4. every `servers[]` URL is absolute (DF-015 class);
5. the surface stays at muster scale (≥ 60 paths, currently 66 / 79
   operations);
6. `x-not-implemented` markers stay honest: the marker may only appear on the
   documented stub allowlist, so a surface cannot quietly stop declaring its
   contract.

If you add a route, the reconciliation test
(`TestOpenAPIRoutesReconciledWithServedSpec`) forces the spec entry and
`TestServedSpecOperationsDeclareSuccessResponse` /
`TestServedSpecDeclaredResponsesCarryBodySchema` force the muster-facing
declarations. Regenerate the bundle and rebuild before serving.

## Verification history

- 2026-10-09, MUSTER-INT-001: audited the served spec with muster
  (`discover`, `generate`, generated-command execution against a scratch
  instance): 0 missing operationIds, 0 relative servers, 0 dangling refs;
  fixed 3 operations whose real served bodies were undeclared
  (`GET /find/symbol` 200 Symbol[], `GET /project/{projectId}` 200 Project,
  `POST /tui/{action}` 200 boolean + 400 NamedError) and 8 error/edge
  responses that were description-only (PUT /auth 405, POST /mcp 405+501,
  POST /webhooks/{source} 403/404/413/429/500).
