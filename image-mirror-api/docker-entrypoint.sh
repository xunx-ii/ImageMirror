#!/bin/sh
set -eu

case "${APP_ROLE:-api}" in
  api)
    exec image-mirror-api
    ;;
  worker)
    exec image-mirror-worker
    ;;
  seed)
    exec image-mirror-seed
    ;;
  *)
    echo "unknown APP_ROLE: ${APP_ROLE}" >&2
    exit 1
    ;;
esac
