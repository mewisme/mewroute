# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mewroute ./cmd/mewroute

FROM alpine:3.21

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 -g '' mewroute

COPY --from=builder /out/mewroute /usr/local/bin/mewroute

USER mewroute

EXPOSE 8080

ENV ROOT_DIR=/data \
    PORT=8080 \
    LOG_LEVEL=info

ENTRYPOINT ["/usr/local/bin/mewroute"]
