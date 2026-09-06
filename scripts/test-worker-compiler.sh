#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
compiler_image="pagewright-compiler-test:$(date +%s)-$$"
docker build --target compiler-acceptance -f pagewright/worker/Dockerfile -t "$compiler_image" .
docker run --rm --user 1000:1000 --network none --cap-drop ALL --security-opt no-new-privileges:true --tmpfs /work:rw,nosuid,nodev,size=268435456,mode=0700,uid=1000,gid=1000 -e TMPDIR=/work "$compiler_image"
