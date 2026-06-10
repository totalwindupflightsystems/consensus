# Conscience — Multi-stage Docker build
# axiom:trace work_item=repo-bootstrap-01 spec=specs/021-repository-layout.md plan=phase-1/task-1/step-4

FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /conscience ./cmd/conscience

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /conscience /usr/local/bin/conscience
COPY conscience.yaml /etc/conscience/conscience.yaml
EXPOSE 8090
ENTRYPOINT ["conscience"]
CMD ["serve"]
