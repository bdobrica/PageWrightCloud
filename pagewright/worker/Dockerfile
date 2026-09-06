# Build stage
FROM golang:1.24.10-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY pagewright/worker/go.mod pagewright/worker/go.sum ./
RUN go mod download

# Copy source code
COPY pagewright/worker/ .

# Build the runner
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o runner ./cmd/runner
RUN CGO_ENABLED=0 GOOS=linux go build -o sandbox-bwrap ./cmd/sandbox-bwrap

FROM golang:1.24.10-alpine AS compiler-builder
WORKDIR /compiler
COPY pagewright/compiler/go.mod pagewright/compiler/go.sum ./
RUN go mod download
COPY pagewright/compiler/ .
RUN CGO_ENABLED=0 go build -o /pagewrightc ./cmd/pagewrightc

# Runtime stage
FROM node:24.11.1-bookworm-slim AS runtime

LABEL io.pagewright.sandbox-policy="proc-v1"

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git bubblewrap \
    && rm -rf /var/lib/apt/lists/*

COPY pagewright/worker/cli/package.json pagewright/worker/cli/package-lock.json /opt/pagewright-cli/
RUN cd /opt/pagewright-cli && npm ci --omit=dev --ignore-scripts \
    && cli_native="$(find /opt/pagewright-cli/node_modules -path '*/bin/codex' -type f)" \
    && test -f "$cli_native" && ln -s "$cli_native" /usr/local/bin/codex \
    && test "$(codex --version)" = 'codex-cli 0.153.4'

WORKDIR /app

# Copy the runner binary
COPY --from=builder /build/runner .
COPY --from=builder /build/sandbox-bwrap /usr/local/bin/bwrap
COPY --from=compiler-builder /pagewrightc /usr/local/bin/pagewrightc
COPY pagewright/themes/starter/ /opt/pagewright/themes/starter-1.0.0/
RUN test "$(pagewrightc version)" = 'pagewrightc version 0.1.0' \
    && node -e 'if(require("/opt/pagewright/themes/starter-1.0.0/tokens.json").theme_version!=="1.0.0")process.exit(1)' \
    && chown -R 0:0 /opt/pagewright/themes /usr/local/bin/pagewrightc \
    && find /opt/pagewright/themes -type d -exec chmod 0555 {} + \
    && find /opt/pagewright/themes -type f -exec chmod 0444 {} + \
    && chmod 0555 /usr/local/bin/pagewrightc

# Copy codex instructions template
COPY pagewright/worker/.codex/instructions.md /.codex/instructions.md

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
RUN CGO_ENABLED=0 go test -tags compiler_acceptance -c -o /compiler-acceptance ./internal/build

FROM runtime AS cli-acceptance
COPY --from=acceptance-builder /cli-acceptance /usr/local/bin/cli-acceptance
ENTRYPOINT ["/usr/local/bin/cli-acceptance", "-test.v", "-test.timeout", "120s", "-test.run", "TestInstalled"]

FROM runtime AS compiler-acceptance
COPY --from=acceptance-builder /compiler-acceptance /usr/local/bin/compiler-acceptance
ENTRYPOINT ["/usr/local/bin/compiler-acceptance", "-test.v", "-test.timeout", "120s"]

FROM runtime AS production
