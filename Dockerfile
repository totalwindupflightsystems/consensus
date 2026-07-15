# Consensus — multi-stage Docker build
# Build:  docker build -t consensus .
# Run:    docker run -p 8090:8090 consensus
# Prod:   docker run -e CONSENSUS_API_KEY=cs_ak_... -v consensus-data:/data consensus

# ── Stage 1: Build ──────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/consensus ./cmd/consensus

# ── Stage 2: Runtime ────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl tzdata

# Non-root user
RUN adduser -D -h /home/consensus consensus
USER consensus
WORKDIR /home/consensus

COPY --from=builder /bin/consensus /usr/local/bin/consensus

# Consensus listens on 8090 by default (health endpoint on /api/v1/health)
EXPOSE 8090

# Data volume for SQLite and persistent state
VOLUME ["/home/consensus/data"]

ENV CONSENSUS_DB_PATH=/home/consensus/data/consensus.db

ENTRYPOINT ["consensus"]
CMD ["serve", "--db", "sqlite:///home/consensus/data/consensus.db"]
