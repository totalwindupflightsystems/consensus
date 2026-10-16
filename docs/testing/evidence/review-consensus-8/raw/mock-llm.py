#!/usr/bin/env python3
"""REVIEW-CONSENSUS-8 scratch LLM stub.

Serves the OpenAI /v1/chat/completions contract well enough for the Consensus
harness to run one real iteration: turn 1 asks the harness to execute the
approval-gated tool `r8_approval_gated_echo`; later turns answer with a plain
`respond` action. Every request is appended verbatim to REQLOG.
"""
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

REQLOG = os.environ.get("R8_REQLOG", "/tmp/r8/mock-llm-requests.jsonl")
GATED_TOOL = os.environ.get("R8_GATED_TOOL", "r8_approval_gated_echo")
PORT = int(os.environ.get("R8_MOCK_PORT", "18123"))
TURN = {"n": 0}

TOOL_ACTION = {
    "internal_monologue": "Operator asked for the production cleanup. This tool is approval-gated, so I request it and wait.",
    "memory_state_changes": [],
    "system_actions": [],
    "message_to_user": "Requesting approval to run the gated cleanup tool.",
    "tool_requests": [
        {"tool_name": GATED_TOOL, "parameters": {"target": "production"}}
    ],
    "sub_agent_spawns": [],
}

IDLE_ACTION = {
    "internal_monologue": "Nothing else to do; answering the operator.",
    "memory_state_changes": [],
    "system_actions": ["respond"],
    "message_to_user": "Done. Waiting for further instructions.",
    "tool_requests": [],
    "sub_agent_spawns": [],
}


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):  # keep stderr quiet, we log ourselves
        pass

    def _send(self, code, payload):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b""
        TURN["n"] += 1
        with open(REQLOG, "a") as fh:
            fh.write(json.dumps({
                "turn": TURN["n"],
                "path": self.path,
                "request": json.loads(raw or b"{}"),
            }) + "\n")

        if not self.path.endswith("/chat/completions"):
            self._send(404, {"error": {"message": "stub: only /chat/completions"}})
            return

        action = TOOL_ACTION if TURN["n"] == 1 else IDLE_ACTION
        self._send(200, {
            "id": "chatcmpl-r8-stub",
            "object": "chat.completion",
            "created": 0,
            "model": "r8-stub-model",
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": json.dumps(action)},
                "finish_reason": "stop",
            }],
            "usage": {"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
        })

    def do_GET(self):
        # key-probe style call
        self._send(200, {"data": [{"id": "r8-stub-model", "object": "model"}]})


if __name__ == "__main__":
    print(f"r8 mock llm listening on 127.0.0.1:{PORT}", flush=True)
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
