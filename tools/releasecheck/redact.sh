# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#
# redact.sh — 로그에 쓰기 전에 비밀값 꼴을 가린다. lib.sh 가 읽고, main_test.go 가 고정 픽스처로 잰다.
#
# 가리는 것: DSN 의 사용자 정보(scheme://사용자[:비밀번호]@), password= · token= · secret= 꼴(대소문자 무시,
# DB_PASSWORD= 처럼 앞에 무엇이 붙어도), SIGN_KEY= 처럼 이름 끝이 _KEY · -key 인 꼴과 KEY=, PEM 개인 키 블록 전체.
# 가리기는 마지막 방어선이다. 비밀을 찍는 명령은 애초에 돌리지 않는다(release-check.sh 의 st_help).
# GNU sed 를 쓴다(ubuntu-dev 에서 돈다).
redact() {
	sed -E \
		-e '/-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----/,/-----END [A-Z0-9 ]*PRIVATE KEY-----/c [private key block redacted]' \
		-e 's#(://)[^/@[:space:]]+@#\1***@#g' \
		-e 's#((password|passwd|pwd|token|secret|apikey|api_key)=)[^&[:space:]"]+#\1***#gI' \
		-e 's#(([A-Za-z0-9_]*[_-](KEY|Key|key)|KEY)=)[^[:space:]"]+#\1***#g'
}
