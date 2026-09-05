#!/bin/sh
set -eu
artifact_tmp=$(mktemp -d /tmp/pagewright-transport.XXXXXX)
trap 'rm -rf -- "$artifact_tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export TEST_ARTIFACT_PATH="$artifact_tmp/reference.tar.gz"
export TEST_ARTIFACT_FIXTURE=/workspace/tests/fixtures/artifact-transport.json
export TEST_ARTIFACT_SITE_ID="transport-$(basename "$artifact_tmp")"
export TEST_ARTIFACT_VERSION_ID=transport-version
# Worker uploads one real packed fixture; later clients fetch those same bytes.
for service in worker gateway manager storage serving; do
    echo "Integration suite: $service"
    (cd "/workspace/pagewright/$service" && go test -count=1 -v -race -tags=integration ./...)
done
