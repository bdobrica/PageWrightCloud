FROM golang:1.24.10-alpine
RUN apk add --no-cache gcc musl-dev
WORKDIR /workspace
COPY pagewright/gateway/go.mod pagewright/gateway/go.sum pagewright/gateway/
COPY pagewright/manager/go.mod pagewright/manager/go.sum pagewright/manager/
COPY pagewright/storage/go.mod pagewright/storage/go.sum pagewright/storage/
COPY pagewright/worker/go.mod pagewright/worker/go.sum pagewright/worker/
COPY pagewright/serving/go.mod pagewright/serving/go.sum pagewright/serving/
RUN for service in gateway manager storage worker serving; do (cd pagewright/$service && go mod download) || exit; done
COPY pagewright/gateway pagewright/gateway
COPY pagewright/manager pagewright/manager
COPY pagewright/storage pagewright/storage
COPY pagewright/worker pagewright/worker
COPY pagewright/serving pagewright/serving
COPY tests/fixtures tests/fixtures
COPY scripts/integration-suites.sh scripts/integration-suites.sh
