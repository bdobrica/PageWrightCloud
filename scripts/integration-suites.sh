#!/bin/sh
set -eu
for service in gateway manager storage worker; do
    echo "Integration suite: $service"
    (cd "/workspace/pagewright/$service" && go test -count=1 -v -race -tags=integration ./...)
done
