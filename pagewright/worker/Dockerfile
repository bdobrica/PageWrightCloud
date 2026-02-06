# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the runner
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o runner ./cmd/runner

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl

WORKDIR /app

# Copy the runner binary
COPY --from=builder /build/runner .

# Copy codex instructions template
COPY .codex/instructions.md /.codex/instructions.md

# Install codex CLI
# For now, we'll create a placeholder script that can be replaced with the real binary
RUN mkdir -p /usr/local/bin && \
    echo '#!/bin/sh' > /usr/local/bin/codex && \
    echo 'echo "CODEX MOCK: Would execute: $@"' >> /usr/local/bin/codex && \
    echo 'echo "SUMMARY: Mock execution completed"' >> /usr/local/bin/codex && \
    echo 'echo "FILES_CHANGED:"' >> /usr/local/bin/codex && \
    echo 'echo "- modified: content/index.md"' >> /usr/local/bin/codex && \
    chmod +x /usr/local/bin/codex

# Create work directory
RUN mkdir -p /work

# Set environment defaults
ENV PAGEWRIGHT_WORK_DIR=/work \
    PAGEWRIGHT_WORKER_PORT=8082 \
    PAGEWRIGHT_CODEX_BINARY=/usr/local/bin/codex \
    PAGEWRIGHT_INSTRUCTIONS_PATH=/.codex/instructions.md

EXPOSE 8082

CMD ["./runner"]
