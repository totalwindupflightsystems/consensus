# OpenCode CLI compatibility proof

- CLI version: `1.18.29`
- Command: `opencode run --attach <CONSENSUS_OPENCODE_BASE_URL> --pure --format json`
- Shim URL: `http://127.0.0.1:1`
- LLM endpoint: `http://127.0.0.1:1`
- Exit code: `1`
- Sanitized output: [output.log](output.log)

Result: EXPLICIT DIVERGENCE — the attached CLI session did not complete the requested proof.
No failure was converted into a pass; inspect output.log and file a follow-up board row.
