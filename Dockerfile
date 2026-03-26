FROM golang:1.24-alpine AS builder
WORKDIR /app

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -trimpath -o /flowgate ./cmd/flowgate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /flowgate /flowgate
COPY --from=builder /app/config.example.yaml /etc/flowgate/config.yaml
WORKDIR /data
EXPOSE 8080 8443
ENTRYPOINT ["/flowgate", "--config", "/etc/flowgate/config.yaml"]
