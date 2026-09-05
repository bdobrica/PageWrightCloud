FROM golang:1.24.10-alpine
RUN apk add --no-cache gcc musl-dev
WORKDIR /workspace
COPY pagewright/gateway/go.mod pagewright/gateway/go.sum pagewright/gateway/
COPY pagewright/manager/go.mod pagewright/manager/go.sum pagewright/manager/
COPY pagewright/storage/go.mod pagewright/storage/go.sum pagewright/storage/
RUN for service in gateway manager storage; do (cd pagewright/$service && go mod download) || exit; done
COPY pagewright/gateway pagewright/gateway
COPY pagewright/manager pagewright/manager
COPY pagewright/storage pagewright/storage
COPY scripts/integration-suites.sh scripts/integration-suites.sh
