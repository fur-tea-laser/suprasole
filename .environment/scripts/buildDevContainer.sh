#/bin/sh

docker build --platform linux/arm64 --no-cache -t suprasole-dev -f ./.environment/Dockerfile .
