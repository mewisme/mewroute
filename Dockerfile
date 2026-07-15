# syntax=docker/dockerfile:1
# Prebuilt binary (GoReleaser). Context layout: $TARGETPLATFORM/mewroute

FROM alpine:3.21

ARG TARGETPLATFORM

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 -g '' mewroute

COPY ${TARGETPLATFORM}/mewroute /usr/local/bin/mewroute

USER mewroute

EXPOSE 8080

ENV ROOT_DIR=/data \
    PORT=8080 \
    LOG_LEVEL=info

ENTRYPOINT ["/usr/local/bin/mewroute"]
