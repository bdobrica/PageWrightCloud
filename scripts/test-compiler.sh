#!/bin/sh
set -eu

cd "$(dirname "$0")/../pagewright/compiler"
compiler_tmp=$(mktemp -d /tmp/pagewright-compiler.XXXXXX)
trap 'rm -rf -- "$compiler_tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

go run ./cmd/pagewrightc build --theme ../themes/starter --content test-site/content --out "$compiler_tmp/public"
for page in index.html about/index.html contact/index.html; do
    test -s "$compiler_tmp/public/$page"
done
test -s "$compiler_tmp/public/assets/css/theme.css"
test -s "$compiler_tmp/public/assets/css/tokens.css"
test -s "$compiler_tmp/public/assets/js/theme.js"
echo "Compiler fixture produced all three pages and required assets."
