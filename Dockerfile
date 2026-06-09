FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o tideflow .

# ---- runtime ----
FROM alpine:3.21

WORKDIR /app

# Copy binary
COPY --from=builder /build/tideflow .

# Copy static assets and template
COPY app/static/ ./app/static/
COPY app/templates/ ./app/templates/

# Create data directory (SQLite persistence)
RUN mkdir -p /app/data

EXPOSE 8000

ENV DATA_DIR=/app/data
ENV ADMIN_PASSWORD=admin

CMD ["./tideflow"]
