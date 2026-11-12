#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#
# local.sh <out-dir> — 고정한 Python 이미지 안에서 build.sh를 돌린다(이 기계의 Python을 쓰지 않는다). Docker가 필요하다.
# 이미지는 다이제스트로 고정한다. 판은 python-version과 같아야 하고, 도구는 requirements.txt의 해시로만 설치한다.
# 산출물은 <out-dir>에 생기고 리포에는 아무것도 쓰지 않는다. 배포하지 않는다.
set -euo pipefail
OUT=$(mkdir -p "$(dirname "${1:?usage: local.sh <out-dir>}")" && cd "$(dirname "$1")" && pwd)/$(basename "$1")
REPO=$(cd "$(dirname "$0")/../.." && pwd)
IMAGE="python:$(tr -d '[:space:]' < "$REPO/tools/site/python-version")-slim@sha256:5e2bdb1b0d542d5039cffb8181c65930093949194a65c365dfb1b0fe13a4fa5b"
docker run --rm --label pqcaton-site=1 -v "$REPO:/src:ro" -v "$(dirname "$OUT"):/out" -e OUT="/out/$(basename "$OUT")" "$IMAGE" bash -c '
  set -euo pipefail
  apt-get -qq update >/dev/null && apt-get -qq install -y --no-install-recommends git >/dev/null
  git config --global --add safe.directory /src
  pip install -q --disable-pip-version-check --no-cache-dir --root-user-action=ignore --require-hashes --only-binary :all: -r /src/tools/site/requirements.txt
  /src/tools/site/build.sh "$OUT"
  chown -R '"$(id -u):$(id -g)"' /out'
