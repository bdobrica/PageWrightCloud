FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application and CLI tool
RUN CGO_ENABLED=0 GOOS=linux go build -o /gateway cmd/gateway/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /users cmd/users/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binaries from builder
COPY --from=builder /gateway .
COPY --from=builder /users .

# Expose port
EXPOSE 8085

# Run the binary
CMD ["./gateway"]
