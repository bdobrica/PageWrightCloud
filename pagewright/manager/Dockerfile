FROM golang:1.24.10-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o manager-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o recovery-audit ./cmd/recovery-audit

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/manager-server .
COPY --from=builder /build/recovery-audit .

# Expose the port
EXPOSE 8081

# Run the binary
CMD ["./manager-server"]
