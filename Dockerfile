# ---- build stage ----
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown
RUN export GOARM="" && \
    case "${TARGETARCH}" in \
      arm) export GOARM=${TARGETVARIANT#v} ;; \
    esac && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w \
      -X 'main.version=${VERSION}' \
      -X 'main.commitSHA=${COMMIT_SHA}' \
      -X 'main.buildTime=${BUILD_TIME}'" \
    -o tideflow .

# ---- runtime stage ----
FROM scratch

# CA certificates for HTTPS downloads
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Timezone data (required by Go time package)
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Binary
COPY --from=builder /build/tideflow /tideflow

# Static assets and template
COPY app/static/ /app/static/
COPY app/templates/ /app/templates/

EXPOSE 8000

ENV DATA_DIR=/app/data

ENTRYPOINT ["/tideflow"]
