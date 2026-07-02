# ═══════════════════════════════════════════════════════════════
# Stage 1 – Build
# ═══════════════════════════════════════════════════════════════
FROM golang:1.26-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN go build \
    -trimpath \
    -ldflags="-s -w -extldflags=-static" \
    -o /server \
    ./cmd/server/main.go && \
    go build \
    -trimpath \
    -ldflags="-s -w -extldflags=-static" \
    -o /healthcheck \
    ./cmd/healthcheck/main.go

# ═══════════════════════════════════════════════════════════════
# Stage 2 – Certs & timezone
# ═══════════════════════════════════════════════════════════════
FROM alpine:3.21 AS certs
RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates tzdata

# ═══════════════════════════════════════════════════════════════
# Stage 3 – Runtime (scratch)
# ═══════════════════════════════════════════════════════════════
FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /usr/share/zoneinfo/Europe/Moscow   /usr/share/zoneinfo/Europe/Moscow
COPY --from=certs /etc/passwd  /etc/passwd
COPY --from=certs /etc/group   /etc/group

COPY --from=builder /server      /server
COPY --from=builder /healthcheck /healthcheck

ENV TZ=Europe/Moscow

USER nobody

EXPOSE 8081

HEALTHCHECK --interval=10s --timeout=5s --start-period=25s --retries=5 \
    CMD ["/healthcheck"]

ENTRYPOINT ["/server"]
