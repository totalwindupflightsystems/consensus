# Contributing

## Development Setup

```bash
git clone https://github.com/wojons/consensus.git
cd consensus
go mod download
CGO_ENABLED=0 go build ./cmd/consensus
go test ./... -count=1 -short
```

## Commit Convention

- Use conventional commits: `feat:`, `fix:`, `chore:`, `docs:`, `test:`
- Every commit must include `Co-authored-by: Alexis Okuwa <wojonstech@gmail.com>`

## Pull Requests

1. Fork the repository
2. Create a feature branch
3. Ensure tests pass: `go test ./... -count=1 -short`
4. Open a PR against `main`
