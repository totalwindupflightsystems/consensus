# Consensus — Multi-stage Docker build
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /consensus ./cmd/consensus

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /consensus /usr/local/bin/consensus
COPY consensus.yaml /etc/consensus/consensus.yaml
EXPOSE 8090
ENTRYPOINT ["consensus"]
CMD ["serve"]
