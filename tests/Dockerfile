FROM golang:1.24.10-alpine
RUN apk add --no-cache gcc musl-dev nginx
WORKDIR /workspace
COPY pagewright/gateway/go.mod pagewright/gateway/go.sum pagewright/gateway/
COPY pagewright/manager/go.mod pagewright/manager/go.sum pagewright/manager/
COPY pagewright/storage/go.mod pagewright/storage/go.sum pagewright/storage/
COPY pagewright/worker/go.mod pagewright/worker/go.sum pagewright/worker/
COPY pagewright/serving/go.mod pagewright/serving/go.sum pagewright/serving/
COPY pagewright/compiler/go.mod pagewright/compiler/go.sum pagewright/compiler/
RUN for service in gateway manager storage worker serving compiler; do (cd pagewright/$service && go mod download) || exit; done
COPY pagewright/gateway pagewright/gateway
COPY pagewright/manager pagewright/manager
COPY pagewright/storage pagewright/storage
COPY pagewright/worker pagewright/worker
COPY pagewright/serving pagewright/serving
COPY pagewright/compiler pagewright/compiler
COPY pagewright/themes/starter pagewright/themes/starter
RUN cd pagewright/compiler && go build -o /usr/local/bin/pagewrightc ./cmd/pagewrightc
RUN cd pagewright/worker && go build -tags=integration -o /usr/local/bin/deterministic-executor ./test/deterministic-executor && go test -c -race -tags=integration -o /usr/local/bin/worker-contract.test ./cmd/runner
COPY tests/fixtures tests/fixtures
COPY scripts/integration-suites.sh scripts/integration-suites.sh
