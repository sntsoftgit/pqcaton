#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#
# release-check.sh full|fast — 태그를 달기 전에 「이 커밋을 그 판으로 내도 되는가」를 잰다.
#
#   make release-check      VERSION=vX.Y.Z [RELEASE_DATE=YYYY-MM-DD]   발행 근거. 종단 검증까지 한다
#   make release-check-fast VERSION=vX.Y.Z [RELEASE_DATE=YYYY-MM-DD]   발행 근거 아님. 종단 검증만 뺀다
#
# 왜 있는가: v0.26.2는 발행한 뒤에야 `go install` 로 받을 수 없다는 것이 드러났다. 빌드 · 시험 · 종단
# 검증이 모두 체크아웃에서 돌아, go 명령이 모듈 zip 을 만들지 못하는 것을 아무도 보지 않았다. 그래서 여기서는
# 프록시가 하는 그대로 커밋을 zip 으로 묶고, 그 zip 을 담은 임시 프록시에서 문서에 공개한 명령을 실제로 설치한다.
#
# 태그를 달지도, 발행하지도 않는다. 설계: pqcaton-aai plans/2026-11-11-release-check.md
#
# 시험용 스위치 하나: RELEASE_CHECK_ALLOW_EXISTING_TAG=1 이면 「태그가 이미 있음」 검사를 끈다(발행된 판으로 이 스크립트
# 자체를 시험할 때). 켜면 요약 첫 줄에 발행 근거 아님을 찍는다.
set -euo pipefail

MODE="${1:-}"
case "$MODE" in
full) KIND=release-check ;;
fast) KIND=release-check-fast ;;
*) echo "usage: release-check.sh full|fast" >&2; exit 2 ;;
esac
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION="${VERSION:-}"
# shellcheck source=lib.sh
source "$REPO/tools/releasecheck/lib.sh"

# 노트에서 작성 중인 절의 제목 표지(한국어 노트 · 영어 노트). Go 문자열에는 한국어를 두지 않으므로 여기서 넘긴다.
DRAFT_MARKER='작성 중'
DRAFT_MARKER_EN='in progress'
# 발행일은 시작할 때 한 번 정한다. 도중에 자정이 지나도 바꾸지 않는다.
RELEASE_DATE="${RELEASE_DATE:-$(TZ=Asia/Seoul date +%F)}"

if [ "$MODE" = fast ]; then
	NOTICE="NOT RELEASE EVIDENCE: release-check-fast does not run the end-to-end demo"
fi
if [ -n "${RELEASE_CHECK_ALLOW_EXISTING_TAG:-}" ]; then
	NOTICE="NOT RELEASE EVIDENCE: the existing-tag check was turned off (RELEASE_CHECK_ALLOW_EXISTING_TAG)${NOTICE:+; ${NOTICE#NOT RELEASE EVIDENCE: }}"
fi
[ -n "$NOTICE" ] && echo "$NOTICE"
echo "$KIND $VERSION  (release date $RELEASE_DATE)"
note "release date: $RELEASE_DATE (Asia/Seoul at start unless RELEASE_DATE was given)"

# ── 시작 상태: 끝날 때 견준다 ────────────────────────────────────────────────
START="$WORK/start"
mkdir -p "$START"
docker_state() {
	docker ps -aq --no-trunc | sort
	echo --networks--
	docker network ls -q --no-trunc | sort
	echo --volumes--
	docker volume ls -q | sort
}
ours() { { pgrep -x 'pqcaton-ui|pqcota-keygen' || true; pgrep -f -- "$WORK" || true; } | sort -u; }
git -C "$REPO" rev-parse HEAD > "$START/head" 2>/dev/null || true
git -C "$REPO" status --porcelain --untracked-files=all > "$START/status" 2>/dev/null || true
docker_state > "$START/docker" 2>/dev/null || echo unavailable > "$START/docker"
ours > "$START/pids"

# ── 단계 ────────────────────────────────────────────────────────────────────
st_args() {
	need_version
	[[ "$RELEASE_DATE" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "RELEASE_DATE '$RELEASE_DATE' is not YYYY-MM-DD"; return 1; }
	echo "✓ $VERSION, release date $RELEASE_DATE"
}

st_git() {
	cd "$REPO"
	local b
	b="$(git symbolic-ref --short -q HEAD || true)"
	[ "$b" = main ] || { echo "the branch is '${b:-detached}', not main"; return 1; }
	local s
	s="$(git status --porcelain --untracked-files=all)"
	[ -z "$s" ] || { echo "the working tree is not clean:"; echo "$s"; return 1; }
	# 태그도 받아 둔다. 종단 검증은 옛 판을 그 태그에서 빌드하고, 아래의 「태그가 이미 있음」도 원격 기준이라야 맞다.
	git fetch -q --tags --prune origin
	local h o
	h="$(git rev-parse HEAD)"; o="$(git rev-parse origin/main)"
	[ "$h" = "$o" ] || { echo "HEAD $h is not origin/main $o"; return 1; }
	if [ -z "${RELEASE_CHECK_ALLOW_EXISTING_TAG:-}" ]; then
		if git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null; then echo "tag $VERSION already exists locally"; return 1; fi
		if [ -n "$(git ls-remote --tags origin "refs/tags/$VERSION")" ]; then echo "tag $VERSION already exists on origin"; return 1; fi
	fi
	echo "✓ main, clean, HEAD = origin/main = $h"
}

# 사전 조건. 무엇이 없어서 멈췄는지가 그 단계 이름으로 드러나게, 쓰는 단계보다 먼저 한곳에서 본다.
# Docker 는 두 모드 모두 쓴다(REUSE 와 뒷정리 비교). 종단 검증의 조건은 demo/e2e/e2e_test.go 가 보는 것과 같고 full 에서만 본다.
st_preflight() {
	local bad=0 p
	for p in git go make bash docker; do
		command -v "$p" >/dev/null || { echo "missing: $p"; bad=1; }
	done
	docker info >/dev/null 2>&1 || { echo "docker is not usable (docker info failed)"; bad=1; }
	if [ "$MODE" != full ]; then
		[ "$bad" -eq 0 ] && echo "✓ docker usable; end-to-end prerequisites not checked (release-check-fast)"
		return "$bad"
	fi
	command -v python3 >/dev/null || { echo "missing: python3"; bad=1; }
	docker image inspect docker:dind >/dev/null 2>&1 || { echo "missing: the docker:dind image (docker pull docker:dind)"; bad=1; }
	local up
	up="$(cd "$REPO" && go list -m -f '{{.Version}}' github.com/randyinthedev-hash/pqcota-common)"
	if [ -z "${PQCOTA_DIR:-}" ]; then
		echo "missing: PQCOTA_DIR (the upstream pqcota checkout at $up)"; bad=1
	else
		[ -f "$PQCOTA_DIR/demo/scripts/up.sh" ] || { echo "missing: PQCOTA_DIR/demo/scripts/up.sh"; bad=1; }
		local r got
		for r in pqcota pqcota-common pqcota-inventory pqcota-discovery pqcota-provisioning; do
			got="$(git -C "$PQCOTA_DIR/../$r" describe --tags --exact-match HEAD 2>/dev/null || true)"
			[ "$got" = "$up" ] || { echo "the upstream repository $r is at '${got:-?}', not at $up that go.mod pins"; bad=1; }
		done
	fi
	# 종단 검증이 빌드하는 옛 판 태그. 판을 여기 박지 않고 종단 스크립트에서 읽는다(git 단계가 태그를 받아 두었다).
	local need t
	need="$(grep -oE "refs/tags/v[0-9]+\.[0-9]+\.[0-9]+\^\{commit\}" "$REPO/demo/scripts/e2e-scope.sh" | sed -E 's#refs/tags/##; s#\^\{commit\}##' | sort -u)"
	[ -n "$need" ] || { echo "found no tag the end-to-end demo builds from (demo/scripts/e2e-scope.sh)"; bad=1; }
	for t in $need; do
		git -C "$REPO" rev-parse -q --verify "refs/tags/$t^{commit}" >/dev/null || { echo "the end-to-end demo needs tag $t, which is not here after fetching"; bad=1; }
	done
	[ "$bad" -eq 0 ] && echo "✓ docker, docker:dind, upstream at $up, tags for the end-to-end demo: $(echo $need)"
	return "$bad"
}

st_tools() {
	cd "$TOOLS"
	go build -o "$WORK/rc" .
	(cd "$REPO" && go env GOVERSION) > "$WORK/goversion"
	go list -m -f '{{.Version}}' golang.org/x/vuln > "$WORK/vulnversion"
	(cd "$REPO" && go list -m -f '{{.Path}}') > "$WORK/module"
	(cd "$REPO" && go list -m -f '{{.Path}} {{.Version}}' all | grep '/randyinthedev-hash/pqcota' || true) > "$WORK/upstream"
	echo "go $(cat "$WORK/goversion"), govulncheck $(cat "$WORK/vulnversion"), module $(cat "$WORK/module")"
}

st_notes() {
	"$WORK/rc" notes -repo "$REPO" -version "$VERSION" -date "$RELEASE_DATE" -draft-marker "$DRAFT_MARKER" -draft-marker "$DRAFT_MARKER_EN"
	# 릴리스 본문: 영어 절 그대로, 링크는 태그 고정, 맨 위에 한국어 노트의 같은 판으로 가는 링크. 발행은 이 파일을 본문으로 쓴다.
	"$WORK/rc" body -repo "$REPO" -version "$VERSION" -ko-label "$(ko_label)" > "$RUN/release-body.md"
	echo "release body: $RUN/release-body.md ($(wc -l < "$RUN/release-body.md") lines)"
	head -n 1 "$RUN/release-body.md"
	cd "$REPO"
	# 판 시험 IC-U112 · IC-U144: 안내서 · 판별 표 · go.mod · README · 데모 안내의 판. 같은 규칙을 여기서 다시 짜지 않는다.
	local out
	out="$(go test -count=1 -v -run '^(TestOperatorGuideInstallsTheLatestRelease|TestCompatibilityTableIsCurrent)$' ./inventory/cmd/pqcaton-ui/)"
	echo "$out"
	for t in TestOperatorGuideInstallsTheLatestRelease TestCompatibilityTableIsCurrent; do
		grep -q -- "--- PASS: $t " <<<"$out" || { echo "$t did not pass"; return 1; }
	done
}

st_make() { make -C "$REPO"; }
st_generated() { make -C "$REPO" verify-generated; }
st_reuse() { make -C "$REPO" check-reuse; }
st_vet() {
	cd "$REPO"
	go vet ./...
	go vet -tags e2edemo ./demo/e2e/
	(cd tools/checkmodzip && go vet ./...)
	(cd "$TOOLS" && go vet ./...)
	echo "✓ vet"
}
st_race() { cd "$REPO" && go test -race -count=1 ./...; }
st_tidy() {
	for d in "$REPO" "$REPO/tools/checkmodzip" "$TOOLS"; do
		(cd "$d" && go mod tidy -diff) || { echo "go mod tidy would change $d"; return 1; }
	done
	echo "✓ tidy (3 modules)"
}
st_vuln() {
	cd "$TOOLS"
	for d in "$REPO" "$REPO/tools/checkmodzip" "$TOOLS"; do
		echo "== $d"
		go tool govulncheck -C "$d" ./...
	done
}

st_zip() {
	local mod esc at
	mod="$(cat "$WORK/module")"
	esc="$("$WORK/rc" escape "$mod")"
	at="$WORK/proxy/$esc/@v"
	mkdir -p "$at"
	"$WORK/rc" zip -repo "$REPO" -rev HEAD -version "$VERSION" -out "$at/$VERSION.zip" > "$WORK/h1"
	git -C "$REPO" show HEAD:go.mod > "$at/$VERSION.mod"
	echo "$VERSION" > "$at/list"
	printf '{"Version":"%s","Time":"%s"}\n' "$VERSION" \
		"$(TZ=UTC git -C "$REPO" log -1 --date=format-local:%Y-%m-%dT%H:%M:%SZ --format=%cd HEAD)" > "$at/$VERSION.info"
	echo "✓ module zip $(cat "$WORK/h1")"
}

st_commands() {
	"$WORK/rc" commands -repo "$REPO" -version "$VERSION" | tee "$WORK/commands"
}

# 설치는 리포 바깥 빈 폴더에서, 따로 둔 캐시 · GOBIN 으로, 빈 GOFLAGS 로 한다. 사용자가 붙여 넣는 자리와 같게.
install_env() { env -u GOFLAGS GOFLAGS= GOMODCACHE="$1" GOBIN="$2" GOPROXY="$3" "${@:4}"; }

st_install_own() {
	local mod h1
	mod="$(cat "$WORK/module")"; h1="$(cat "$WORK/h1")"
	mkdir -p "$WORK/empty" "$WORK/gmc" "$WORK/bin"
	cd "$WORK/empty"
	local n=0
	while IFS=$'\t' read -r kind pkg where; do
		[ "$kind" = own ] || continue
		echo "== $pkg  ($where)"
		install_env "$WORK/gmc" "$WORK/bin" "file://$WORK/proxy,https://proxy.golang.org" GONOSUMDB="$mod" go install "$pkg"
		local bin got
		bin="$WORK/bin/$(binname "$pkg")"
		got="$(go version -m "$bin" | awk -F'\t' '$2=="mod"{print $3" "$4" "$5}')"
		echo "   $got"
		[ "$got" = "$mod $VERSION $h1" ] || { echo "   the installed binary says '$got', want '$mod $VERSION $h1'"; return 1; }
		n=$((n + 1))
	done < "$WORK/commands"
	[ "$n" -gt 0 ] || { echo "no own install command to try"; return 1; }
	echo "✓ installed $n own command(s) from the module zip"
}

st_install_external() {
	mkdir -p "$WORK/empty" "$WORK/gmc-ext" "$WORK/bin-ext"
	cd "$WORK/empty"
	local n=0
	while IFS=$'\t' read -r kind pkg where; do
		[ "$kind" = external ] || continue
		echo "== external upstream command $pkg  ($where)"
		install_env "$WORK/gmc-ext" "$WORK/bin-ext" "https://proxy.golang.org" go install "$pkg"
		local bin got
		bin="$WORK/bin-ext/$(binname "$pkg")"
		got="$(go version -m "$bin" | awk -F'\t' '$2=="mod"{print $4}')"
		echo "   module version $got"
		[ "$got" = "${pkg##*@}" ] || { echo "   want ${pkg##*@}"; return 1; }
		n=$((n + 1))
	done < "$WORK/commands"
	echo "✓ installed $n external upstream command(s) from the real proxy"
}

# -h 는 종료코드가 아니라 사용법을 찍는지로 본다. 빈 폴더 · 빈 HOME 에서 돌리고, 아무것도 만들지 않아야 한다.
#
# 이 모듈의 명령만 돌린다. 외부 상류 명령은 설치와 판 확인까지만 하고 실행하지 않는다. pqcota-keygen 은 -h 를
# 모르고 곧바로 새 서명 키를 만들어 찍는다. 처음 시험할 때 그 개인 키가 로그에 남았다(일회용 키, 로그는 지웠다).
st_help() {
	local n=0
	for bin in "$WORK"/bin/*; do
		[ -x "$bin" ] || continue
		local box
		box="$WORK/help-$(basename "$bin")"
		mkdir -p "$box/cwd" "$box/home"
		local out
		out="$(cd "$box/cwd" && env HOME="$box/home" XDG_CONFIG_HOME="$box/home/.config" XDG_DATA_HOME="$box/home/.local/share" \
			XDG_STATE_HOME="$box/home/.local/state" XDG_CACHE_HOME="$box/home/.cache" timeout 30 "$bin" -h 2>&1 || true)"
		echo "== $(basename "$bin") -h"
		head -n 3 <<<"$out"
		grep -qi 'usage' <<<"$out" || { echo "   printed no usage"; return 1; }
		local made
		made="$(find "$box/cwd" "$box/home" -mindepth 1 | sed "s#$box/##")"
		[ -z "$made" ] || { echo "   -h created files:"; echo "$made"; return 1; }
		n=$((n + 1))
	done
	[ "$n" -gt 0 ] || { echo "no installed binary to try"; return 1; }
	echo "✓ $n own binaries print usage for -h and create nothing (external upstream commands are not run)"
}

st_e2e() { make -C "$REPO" e2e-demo; }

st_leftovers() {
	local bad=0
	[ "$(git -C "$REPO" rev-parse HEAD)" = "$(cat "$START/head")" ] || { echo "HEAD moved"; bad=1; }
	git -C "$REPO" status --porcelain --untracked-files=all | diff -u "$START/status" - || { echo "the working tree changed"; bad=1; }
	local now
	now="$(ours)"
	local extra
	extra="$(comm -13 "$START/pids" <(echo "$now" | sed '/^$/d'))"
	[ -z "$extra" ] || { echo "processes left behind:"; ps -o pid=,args= -p "$(paste -sd, <<<"$extra")" || true; bad=1; }
	if grep -qx unavailable "$START/docker"; then
		echo "docker was unavailable at start"; bad=1
	else
		docker_state | diff -u "$START/docker" - || { echo "Docker containers, networks or volumes changed"; bad=1; }
	fi
	[ "$bad" -eq 0 ] && echo "✓ working tree, processes and Docker resources are as they were"
	return "$bad"
}

step args st_args
step git st_git
step preflight st_preflight
step tools st_tools
note "commit: $(git -C "$REPO" rev-parse HEAD)"
note "go: $(cat "$WORK/goversion") (GOTOOLCHAIN=auto)"
note "govulncheck: $(cat "$WORK/vulnversion") (vulnerability DB as of the run)"
while read -r line; do note "upstream: $line"; done < "$WORK/upstream"
step notes-and-versions st_notes
note "release body: $RUN/release-body.md (gh release create --notes-file)"
step make st_make
step verify-generated st_generated
step reuse st_reuse
step vet st_vet
step race st_race
step tidy st_tidy
step govulncheck st_vuln
step module-zip st_zip
note "h1: $(cat "$WORK/h1")"
step published-commands st_commands
while IFS=$'\t' read -r kind pkg _; do note "command ($kind): go install $pkg"; done < "$WORK/commands"
step install-own st_install_own
step install-external st_install_external
step help-output st_help
if [ "$MODE" = full ]; then
	step e2e-demo st_e2e
else
	skip e2e-demo "release-check-fast"
fi
step leftovers st_leftovers
