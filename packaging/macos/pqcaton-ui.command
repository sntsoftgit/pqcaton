#!/bin/sh
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#
# macOS 실행기(Intel Mac은 검증한 지원 플랫폼, Apple Silicon은 검증하지 않음). 작업 폴더 ~/pqcaton으로 화면을 띄우고
# 브라우저를 연다.
# 이미 떠 있으면 그 화면을 연다.
# 화면은 pqcota 이력 없이 시작하지 않는다. 연결 정보는 화면이 ~/Library/Application Support/pqcaton/pqcota.env(0600,
# PQCOTA_DSN=… 한 줄)에서 직접 읽는다. 이 실행기는 그 파일을 읽지도 셸로 평가하지도 않는다.
exec "$HOME/.local/bin/pqcaton-ui"
