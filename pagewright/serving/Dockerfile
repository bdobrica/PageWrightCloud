# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o serving-runner ./cmd/runner

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates nginx

WORKDIR /app

COPY --from=builder /build/serving-runner .

# Create necessary directories
RUN mkdir -p /var/www /etc/nginx/sites-enabled /etc/pagewright

ENV PAGEWRIGHT_SERVING_PORT=8083 \
    PAGEWRIGHT_WWW_ROOT=/var/www \
    PAGEWRIGHT_NGINX_SITES_ENABLED=/etc/nginx/sites-enabled \
    PAGEWRIGHT_NGINX_RELOAD_COMMAND="nginx -s reload" \
    PAGEWRIGHT_MAX_VERSIONS_PER_SITE=10 \
    PAGEWRIGHT_MAINTENANCE_PAGE_PATH=/etc/pagewright/503.html

EXPOSE 8083

CMD ["./serving-runner"]
