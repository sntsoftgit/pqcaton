#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#
# release-verify.sh — 발행한 뒤 「받는 사람이 정말 그 커밋을 받는가」를 잰다. 실패하면 릴리스 완료로 보지 않는다.
#
#   make release-verify VERSION=vX.Y.Z COMMIT=<40자 커밋> H1=h1:…
#
# H1 은 release-check 가 그 커밋으로 뜬 모듈 zip 해시다. 원격 태그 · Go 프록시의 .info · .mod · .zip ·
# sum.golang.org 기록을 그것과 견주고, 깨끗한 캐시에서 실제 프록시와 체크섬 DB 로 공개 명령을 설치해 본다.
# 프록시가 아직 그 판을 모르면 실패한다(잠시 뒤 다시 돌린다).
set -euo pipefail

KIND=release-verify
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION="${VERSION:-}"
COMMIT="${COMMIT:-}"
H1="${H1:-}"
# shellcheck source=lib.sh
source "$REPO/tools/releasecheck/lib.sh"
PROXY=https://proxy.golang.org
SUMDB=https://sum.golang.org
fetch() { curl -fsS --retry 3 --retry-delay 5 -o "$2" "$1"; }

st_args() {
	need_version
	[[ "$COMMIT" =~ ^[0-9a-f]{40}$ ]] || { echo "COMMIT must be a full 40-character commit hash"; return 1; }
	[[ "$H1" =~ ^h1:[A-Za-z0-9+/]{43}=$ ]] || { echo "H1 must look like h1:<base64>= (from release-check)"; return 1; }
	echo "✓ $VERSION $COMMIT $H1"
}

st_tools() {
	cd "$TOOLS"
	go build -o "$WORK/rc" .
	(cd "$REPO" && go list -m -f '{{.Path}}') > "$WORK/module"
	"$WORK/rc" escape "$(cat "$WORK/module")" > "$WORK/esc"
	echo "module $(cat "$WORK/module")"
}

st_tag() {
	local refs got
	refs="$(git -C "$REPO" ls-remote --tags origin "refs/tags/$VERSION" "refs/tags/$VERSION^{}")"
	echo "$refs"
	# 주석 태그면 ^{} 가 커밋이다. 가벼운 태그면 그 줄 자체가 커밋이다.
	got="$(awk -v t="refs/tags/$VERSION^{}" '$2==t{print $1}' <<<"$refs")"
	[ -n "$got" ] || got="$(awk -v t="refs/tags/$VERSION" '$2==t{print $1}' <<<"$refs")"
	[ -n "$got" ] || { echo "no tag $VERSION on origin"; return 1; }
	[ "$got" = "$COMMIT" ] || { echo "tag $VERSION points at $got, want $COMMIT"; return 1; }
	echo "✓ origin tag $VERSION → $COMMIT"
}

st_info() {
	fetch "$PROXY/$(cat "$WORK/esc")/@v/$VERSION.info" "$WORK/v.info"
	cat "$WORK/v.info"; echo
	"$WORK/rc" info -version "$VERSION" -commit "$COMMIT" "$WORK/v.info"
}

st_mod() {
	# 커밋만 받는다. 로컬에 태그 ref 를 만들지 않는다.
	git -C "$REPO" cat-file -e "$COMMIT^{commit}" 2>/dev/null || git -C "$REPO" fetch -q origin "$COMMIT"
	git -C "$REPO" show "$COMMIT:go.mod" > "$WORK/want.mod"
	fetch "$PROXY/$(cat "$WORK/esc")/@v/$VERSION.mod" "$WORK/v.mod"
	cmp "$WORK/want.mod" "$WORK/v.mod" || { echo "the proxy's go.mod differs from $COMMIT:go.mod"; diff -u "$WORK/want.mod" "$WORK/v.mod" || true; return 1; }
	echo "✓ proxy .mod equals $COMMIT:go.mod"
}

st_zip() {
	fetch "$PROXY/$(cat "$WORK/esc")/@v/$VERSION.zip" "$WORK/v.zip"
	local got
	got="$("$WORK/rc" hashzip "$WORK/v.zip")"
	[ "$got" = "$H1" ] || { echo "the proxy's zip hashes to $got, want $H1"; return 1; }
	echo "✓ proxy .zip $got"
}

st_sumdb() {
	local mod modh1
	mod="$(cat "$WORK/module")"
	modh1="$("$WORK/rc" hashmod "$WORK/want.mod")"
	fetch "$SUMDB/lookup/$(cat "$WORK/esc")@$VERSION" "$WORK/lookup"
	grep -x "$mod $VERSION $H1" "$WORK/lookup" || { echo "sum.golang.org has no '$mod $VERSION $H1'"; cat "$WORK/lookup"; return 1; }
	grep -x "$mod $VERSION/go.mod $modh1" "$WORK/lookup" || { echo "sum.golang.org has no '$mod $VERSION/go.mod $modh1'"; cat "$WORK/lookup"; return 1; }
	echo "✓ sum.golang.org records $H1 and the go.mod hash"
}

# 공개 명령은 그 커밋의 문서에서 뽑는다. 깨끗한 캐시 · 실제 프록시 · 체크섬 DB 로 설치한다.
st_install() {
	local mod
	mod="$(cat "$WORK/module")"
	mkdir -p "$WORK/docs/docs" "$WORK/empty" "$WORK/gmc" "$WORK/bin"
	for f in go.mod README.md docs/operator-guide.md docs/operator-guide.en.md; do
		git -C "$REPO" show "$COMMIT:$f" > "$WORK/docs/$f"
	done
	"$WORK/rc" commands -repo "$WORK/docs" -version "$VERSION" | tee "$WORK/commands"
	cd "$WORK/empty"
	local n=0
	while IFS=$'\t' read -r kind pkg where; do
		[ "$kind" = own ] || continue
		echo "== $pkg  ($where)"
		env -u GOFLAGS GOFLAGS= GOMODCACHE="$WORK/gmc" GOBIN="$WORK/bin" GOPROXY="$PROXY" GOSUMDB=sum.golang.org go install "$pkg"
		local got
		got="$(go version -m "$WORK/bin/$(binname "$pkg")" | awk -F'\t' '$2=="mod"{print $3" "$4" "$5}')"
		echo "   $got"
		[ "$got" = "$mod $VERSION $H1" ] || { echo "   want '$mod $VERSION $H1'"; return 1; }
		n=$((n + 1))
	done < "$WORK/commands"
	[ "$n" -gt 0 ] || { echo "no own install command in the docs of $COMMIT"; return 1; }
	echo "✓ installed $n command(s) from the real proxy with the checksum database"
}

note "commit: $COMMIT"
note "h1: $H1"
step args st_args
step tools st_tools
note "go: $(cd "$REPO" && go env GOVERSION) (GOTOOLCHAIN=auto)"
step remote-tag st_tag
step proxy-info st_info
step proxy-mod st_mod
step proxy-zip st_zip
step sumdb st_sumdb
step install st_install
