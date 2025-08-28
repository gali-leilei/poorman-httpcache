#!/bin/bash

# Check if the search endpoint is working
echo "Checking search endpoint..."

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/../.env"
# export the env variable
set -o allexport
# shellcheck source=.env
source "$ENV_FILE"
set +o allexport


curl --location "http://127.0.0.1:${PORT}/jina/https://www.example.com" \
--header "Authorization: Bearer $INTERNAL_KEY"

curl --location "http://127.0.0.1:${PORT}/jina/https://news.ycombinator.com/news" \
--header "Authorization: Bearer $INTERNAL_KEY"

curl --location "http://127.0.0.1:${PORT}/jina/https://sans-io.readthedocs.io" \
--header "Authorization: Bearer $INTERNAL_KEY"

