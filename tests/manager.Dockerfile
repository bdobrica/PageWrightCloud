FROM golang:1.24.10-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -tags=integration -o /manager-server ./cmd/server
FROM alpine:latest
WORKDIR /app
COPY --from=builder /manager-server .
CMD ["./manager-server"]
