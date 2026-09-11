FROM golang:1.24.10-alpine AS builder
WORKDIR /build
COPY pagewright/gateway/go.mod pagewright/gateway/go.sum ./
RUN go mod download
COPY pagewright/gateway .
RUN CGO_ENABLED=0 go build -o /fixture ./test/backup-fixture
FROM alpine:latest
COPY --from=builder /fixture /fixture
ENTRYPOINT ["/fixture"]
