# Build stage
FROM golang:1.24.10-alpine AS builder

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
FROM node:24.11.1-bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git bubblewrap \
    && rm -rf /var/lib/apt/lists/*

COPY cli/package.json cli/package-lock.json /opt/pagewright-cli/
RUN cd /opt/pagewright-cli && npm ci --omit=dev --ignore-scripts \
    && cli_native="$(find /opt/pagewright-cli/node_modules -path '*/bin/codex' -type f)" \
    && test -f "$cli_native" && ln -s "$cli_native" /usr/local/bin/codex \
    && test "$(codex --version)" = 'codex-cli 0.153.4'

WORKDIR /app

# Copy the runner binary
COPY --from=builder /build/runner .

# Copy codex instructions template
COPY .codex/instructions.md /.codex/instructions.md

# Create work directory
RUN mkdir -p /work && chown 1000:1000 /work
USER 1000:1000

# Set environment defaults
ENV PAGEWRIGHT_WORK_DIR=/work \
    PAGEWRIGHT_WORKER_PORT=8082 \
    PAGEWRIGHT_CODEX_BINARY=/usr/local/bin/codex \
    PAGEWRIGHT_INSTRUCTIONS_PATH=/.codex/instructions.md

EXPOSE 8082

CMD ["./runner"]

FROM builder AS acceptance-builder
RUN CGO_ENABLED=0 go test -tags cli_acceptance -c -o /cli-acceptance ./internal/codex

FROM runtime AS cli-acceptance
COPY --from=acceptance-builder /cli-acceptance /usr/local/bin/cli-acceptance
ENTRYPOINT ["/usr/local/bin/cli-acceptance", "-test.v", "-test.timeout", "120s", "-test.run", "TestInstalled"]

FROM runtime AS production
