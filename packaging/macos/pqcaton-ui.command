#!/bin/sh
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#
# macOS 편의 실행기(검증한 지원 플랫폼은 Linux다). 작업 폴더 ~/pqcaton으로 화면을 띄우고 브라우저를 연다.
# 이미 떠 있으면 그 화면을 연다.
exec "$HOME/.local/bin/pqcaton-ui"
