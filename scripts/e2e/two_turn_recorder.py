#!/usr/bin/env python3
"""Hermetic LLM recorder stub for the DF-CONSENSUS-6-R2 two-turn e2e arm.

Binds 127.0.0.1 on an OS-assigned free port, appends every request body it
receives as one JSON line to the record file (argv[1]), and answers a canned
OpenAI-compatible chat.completion whose content is a valid AgentOutput plan
(a plain "respond" turn — no tools, no SQL) so the harness settles the
iteration with a reply.

Stdlib only. Prints "PORT=<n>" on stdout once listening (the driver waits for
that line before starting serve).

The canned reply text is the turn-1 assistant reply the acceptance arm asserts
on: the recorded turn-2 request must literally contain it.
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

RECORD_PATH = sys.argv[1]

REPLY_TEXT = "Ack: the assistant reply marker is E2E-REPLY-9f31."

PLAN = {
    "internal_monologue": "e2e canned reply",
    "memory_state_changes": [],
    "system_actions": ["respond"],
    "message_to_user": REPLY_TEXT,
    "tool_requests": [],
    "sub_agent_spawns": [],
}

CANNED = {
    "id": "chatcmpl-e2e-stub",
    "object": "chat.completion",
    "created": 0,
    "model": "e2e-stub",
    "choices": [
        {
            "index": 0,
            "message": {"role": "assistant", "content": json.dumps(PLAN)},
            "finish_reason": "stop",
        }
    ],
    "usage": {"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
}


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        with open(RECORD_PATH, "a", encoding="utf-8") as fh:
            fh.write(body.decode("utf-8", "replace") + "\n")
            fh.flush()
        payload = json.dumps(CANNED).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, *args):  # keep the stub log to the PORT line
        pass


def main():
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    print(f"PORT={server.server_address[1]}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
