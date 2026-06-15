// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.Version=$(git describe --tags 2>/dev/null || echo dev) -X main.BuildAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /build/download-manager .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata ffmpeg wget
COPY --from=builder /build/download-manager /usr/local/bin/download-manager
EXPOSE 8080
ENTRYPOINT ["download-manager"]
CMD ["--config", "/etc/download-manager/config.yaml"]
