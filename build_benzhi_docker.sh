#!/bin/sh
set -eu
repo="${1:-ygw-go-49-01}"
tag="${2:-latest}"
docker build --platform linux/amd64 -t "${repo}:${tag}-amd64" -f benzhi.Dockerfile .
docker build --platform linux/arm64 -t "${repo}:${tag}-arm64" -f benzhi.Dockerfile .
echo "built ${repo}:${tag}-amd64 and ${repo}:${tag}-arm64"
