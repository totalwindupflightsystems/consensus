const REDIRECT_HOSTS = new Set(["localhost", "127.0.0.1", "test"])

// LLM path prefix that must NOT be redirected. The pinned httpapi-sdk
// suite registers its fake LLM (TestLLMServer) on a loopback port and the
// upstream server calls it over POST /v1/chat/completions — that is the
// fixture's own transport, not the opencode protocol surface. Redirecting
// it stole the fixture's LLM calls and answered them with the shim's 404,
// which made "includes project skills in REST API prompt context" fail
// with zero captured inputs (DF-CONSENSUS-39).
const LLM_PATH_PREFIX = "/v1/"

export function redirectURL(input, baseURL) {
  const source = input instanceof URL ? input : new URL(String(input))
  if (!REDIRECT_HOSTS.has(source.hostname)) return source
  if (source.pathname.startsWith(LLM_PATH_PREFIX)) return source
  const target = new URL(baseURL)
  target.pathname = source.pathname
  target.search = source.search
  target.hash = source.hash
  return target
}

export function installFetchRedirect(baseURL, globalObject = globalThis, password = process.env.CONSENSUS_OPENCODE_PASSWORD) {
  if (!baseURL) throw new Error("CONSENSUS_OPENCODE_BASE_URL is required")
  const originalFetch = globalObject.fetch
  const original = originalFetch.bind(globalObject)
  const wrapped = async (input, init) => {
    const source = new Request(input, init)
    // The LLM fixture transport (/v1/* on loopback) is not a shim request:
    // no redirect and no synthetic shim credentials (DF-CONSENSUS-39).
    if (new URL(source.url).pathname.startsWith(LLM_PATH_PREFIX)) {
      return original(source)
    }
    const redirected = redirectURL(source.url, baseURL)
    const request = new Request(redirected, source)
    if (password && !request.headers.has("authorization")) {
      request.headers.set("authorization", `Basic ${Buffer.from(`opencode:${password}`).toString("base64")}`)
    }
    return original(request)
  }
  if (originalFetch.preconnect) wrapped.preconnect = originalFetch.preconnect
  globalObject.fetch = wrapped
  return () => {
    globalObject.fetch = originalFetch
  }
}

if (process.env.CONSENSUS_OPENCODE_BASE_URL) {
  installFetchRedirect(process.env.CONSENSUS_OPENCODE_BASE_URL)
}
