#!/bin/bash
set -e
apk update
apk add docker docker-cli-compose
rc-update add docker boot
service docker start
