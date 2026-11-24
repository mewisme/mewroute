# syntax=docker/dockerfile:1
# Prebuilt binary (GoReleaser). Context layout: $TARGETPLATFORM/mewroute

FROM alpine:3.21

ARG TARGETPLATFORM

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 -g '' mewroute

COPY ${TARGETPLATFORM}/mewroute /usr/local/bin/mewroute

USER mewroute

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT}/healthz" || exit 1

ENV ROOT_DIR=/data \
    PORT=8080 \
    LOG_LEVEL=info

ENTRYPOINT ["/usr/local/bin/mewroute"]
