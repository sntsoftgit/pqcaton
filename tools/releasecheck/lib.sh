# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#
# lib.sh — release-check.sh 와 release-verify.sh 가 함께 쓰는 단계 · 기록 · 뒷정리.
#
# 단계마다 출력 전체를 로그 파일로 남기고, 요약(summary.txt)은 단계를 끝낼 때마다 다시 쓴다.
# 그래서 어느 단계에서 멈춰도 그 단계까지의 요약이 남는다. 폴더는 0700, 파일은 0600이다(umask 077).
# 출력은 디스크에 닿기 전에 redact.sh 를 거친다. 환경 변수는 찍지 않는다.
#
# 부르는 쪽이 정해 둘 것: KIND(요약 첫 줄의 종류), REPO, VERSION.

umask 077
export GOTOOLCHAIN=auto
# 사용자 설정이 시험을 바꾸지 않게 한다. GOFLAGS 는 설치 단계에서 따로 비운다.
unset GOPRIVATE GONOPROXY GONOSUMDB GOINSECURE

# shellcheck disable=SC2034 # 부르는 스크립트가 쓴다
TOOLS="$REPO/tools/releasecheck"
STARTED="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_ROOT="${RELEASE_CHECK_OUT:-$HOME/pqcaton-sandbox/release-check}"
mkdir -p "$OUT_ROOT"
chmod 0700 "$OUT_ROOT"
# 같은 커밋으로 1초 안에 두 번 돌려도 겹치지 않게 끝에 임의 문자를 붙인다. mktemp -d 는 0700으로 만든다.
SAFE_VERSION="${VERSION//[^A-Za-z0-9.]/_}"
RUN="$(mktemp -d "$OUT_ROOT/${SAFE_VERSION:-noversion}-$(git -C "$REPO" rev-parse --short=7 HEAD 2>/dev/null || echo nohead)-$KIND-$STARTED-XXXX")"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/pqcaton-release-check.XXXXXX")"
BODY="$RUN/.body"
: > "$BODY"
HEADER=()       # 요약 머리에 둘 줄. 단계가 값을 알게 되면 덧붙인다.
NOTICE=""       # 발행 근거가 아닐 때 첫 줄 앞에 둘 경고
RESULT="RUNNING"
STEP_NO=0
CURRENT=""

# shellcheck source=redact.sh
source "$REPO/tools/releasecheck/redact.sh"

note() { HEADER+=("$1"); write_summary; }

write_summary() {
	{
		[ -n "$NOTICE" ] && echo "$NOTICE"
		echo "$KIND $VERSION: $RESULT"
		printf '%s\n' "${HEADER[@]}"
		echo
		cat "$BODY"
		echo
		echo "logs: $RUN"
	} > "$RUN/summary.txt"
}

# step <name> <function> [args…] — 단계 하나. 실패하면 요약에 남기고 멈춘다.
step() {
	local name="$1"; shift
	STEP_NO=$((STEP_NO + 1))
	CURRENT="$name"
	local log
	log="$RUN/$(printf %02d "$STEP_NO")-$name.log"
	local t0=$SECONDS
	printf '▶ %-28s' "$name"
	# if 조건 안에서 돌리면 bash 가 서브셸의 set -e 까지 끈다. 그래서 밖에서 돌리고 종료코드만 받는다.
	local rc
	set +e
	( set -euo pipefail; "$@" ) 2>&1 | redact > "$log"
	rc=${PIPESTATUS[0]}
	set -e
	if [ "$rc" -eq 0 ]; then
		local dt=$((SECONDS - t0))
		printf 'PASS  %4ss\n' "$dt"
		printf 'PASS  %-28s %5ss\n' "$name" "$dt" >> "$BODY"
		write_summary
	else
		local dt=$((SECONDS - t0))
		printf 'FAIL  %4ss\n' "$dt"
		printf 'FAIL  %-28s %5ss  (%s)\n' "$name" "$dt" "$(basename "$log")" >> "$BODY"
		echo "---- last lines of $(basename "$log") ----" >> "$BODY"
		tail -n 25 "$log" >> "$BODY"
		RESULT="FAIL at $name"
		write_summary
		exit 1
	fi
}

# skip <name> <why> — 돌리지 않은 단계도 요약에 적는다(통과로 세지 않는다).
skip() {
	STEP_NO=$((STEP_NO + 1))
	printf '▷ %-28s NOT RUN  (%s)\n' "$1" "$2"
	printf 'NOT RUN %-26s (%s)\n' "$1" "$2" >> "$BODY"
	write_summary
}

finish() {
	local rc=$?
	# 모듈 캐시는 읽기 전용이라 먼저 쓰기를 연다. 성공 · 실패 · 중단 모두 지운다.
	chmod -R u+w "$WORK" 2>/dev/null || true
	rm -rf "$WORK"
	if [ "$RESULT" = "RUNNING" ]; then
		if [ "$rc" -eq 0 ]; then RESULT="PASS"; else RESULT="FAIL at ${CURRENT:-start} (exit $rc)"; fi
	fi
	write_summary
	echo
	cat "$RUN/summary.txt"
	exit "$rc"
}
trap finish EXIT
trap 'exit 130' INT TERM

# 공개 명령을 바이너리 이름으로: 경로의 마지막 마디.
binname() { local p="${1%@*}"; echo "${p##*/}"; }

# need_version — VERSION 이 vX.Y.Z 꼴인지.
need_version() {
	if [ -z "${VERSION:-}" ]; then echo "VERSION is required, e.g. make $KIND VERSION=v1.2.3"; return 1; fi
	[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "VERSION '$VERSION' is not vX.Y.Z"; return 1; }
}
