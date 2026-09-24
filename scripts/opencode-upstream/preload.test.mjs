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
