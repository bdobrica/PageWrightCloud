FROM golang:1.24.10-alpine AS builder
WORKDIR /build
COPY tests/spawn-worker/main.go .
RUN CGO_ENABLED=0 go build -o /fixture main.go
FROM alpine:latest
WORKDIR /app
COPY --from=builder /fixture .
CMD ["./fixture"]
