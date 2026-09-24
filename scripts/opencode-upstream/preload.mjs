const REDIRECT_HOSTS = new Set(["localhost", "127.0.0.1", "test"])

export function redirectURL(input, baseURL) {
  const source = input instanceof URL ? input : new URL(String(input))
  if (!REDIRECT_HOSTS.has(source.hostname)) return source
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
