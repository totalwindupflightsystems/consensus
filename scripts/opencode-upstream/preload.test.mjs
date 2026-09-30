import assert from "node:assert/strict"
import test from "node:test"

import { installFetchRedirect, redirectURL } from "./preload.mjs"

test("redirectURL preserves the upstream path and query on the shim origin", () => {
  assert.equal(
    redirectURL("http://localhost:4321/session?limit=10", "http://127.0.0.1:8090/root").href,
    "http://127.0.0.1:8090/session?limit=10",
  )
})

test("redirectURL leaves non-local URLs untouched", () => {
  assert.equal(
    redirectURL("https://example.com/session", "http://127.0.0.1:8090").href,
    "https://example.com/session",
  )
})

// DF-CONSENSUS-39: /v1/* is the LLM fixture's transport (the upstream
// server's own calls to its in-process fake LLM), not the opencode
// protocol surface. Redirecting it stole the fake LLM's requests and made
// the skills-in-prompt-context assertion observe zero inputs.
test("redirectURL leaves the LLM transport path (/v1/*) untouched", () => {
  assert.equal(
    redirectURL("http://127.0.0.1:39305/v1/chat/completions", "http://127.0.0.1:8090").href,
    "http://127.0.0.1:39305/v1/chat/completions",
  )
  assert.equal(
    redirectURL("http://localhost:4321/v1/responses", "http://127.0.0.1:8090").href,
    "http://localhost:4321/v1/responses",
  )
})

test("redirectURL still redirects protocol paths on the same host/port", () => {
  // Same loopback host, protocol path → shim.
  assert.equal(
    redirectURL("http://127.0.0.1:39305/session/ses_1/message", "http://127.0.0.1:8090").href,
    "http://127.0.0.1:8090/session/ses_1/message",
  )
})

test("installFetchRedirect preserves method, headers, and body", async () => {
  let captured
  const fakeGlobal = {
    fetch: async (input, init) => {
      captured = { request: input instanceof Request ? input : new Request(input, init), init }
      return new Response("ok")
    },
  }
  const restore = installFetchRedirect("http://127.0.0.1:8090", fakeGlobal, "test-password")
  await fakeGlobal.fetch(
    new Request("http://test/session", {
      method: "POST",
      headers: { authorization: "Basic sanitized", "content-type": "application/json" },
      body: JSON.stringify({ title: "adapter" }),
    }),
  )
  restore()

  assert.equal(captured.request.url, "http://127.0.0.1:8090/session")
  assert.equal(captured.request.method, "POST")
  assert.equal(captured.request.headers.get("authorization"), "Basic sanitized")
  assert.deepEqual(await captured.request.json(), { title: "adapter" })
  assert.equal(fakeGlobal.fetch.name, "fetch")
})

test("installFetchRedirect adds test auth only when the request has none", async () => {
  let captured
  const fakeGlobal = {
    fetch: async (input) => {
      captured = input
      return new Response("ok")
    },
  }
  const restore = installFetchRedirect("http://127.0.0.1:8090", fakeGlobal, "test-password")
  await fakeGlobal.fetch("http://localhost/session")
  restore()

  assert.equal(
    captured.headers.get("authorization"),
    `Basic ${Buffer.from("opencode:test-password").toString("base64")}`,
  )
})

test("installFetchRedirect does not add auth to the LLM transport path", async () => {
  let captured
  const fakeGlobal = {
    fetch: async (input, init) => {
      captured = input instanceof Request ? input : new Request(input, init)
      return new Response("ok")
    },
  }
  const restore = installFetchRedirect("http://127.0.0.1:8090", fakeGlobal, "test-password")
  // Request goes to a NON-redirected URL (LLM transport on loopback), so
  // the wrapped fetch must hand it through with no synthetic auth.
  await fakeGlobal.fetch("http://127.0.0.1:39305/v1/chat/completions", { method: "POST" })
  restore()

  assert.equal(captured.url, "http://127.0.0.1:39305/v1/chat/completions")
  assert.equal(captured.headers.get("authorization"), null)
})
