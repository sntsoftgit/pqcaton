#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Great Honor <randyinthedev@gmail.com>
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: Apache-2.0
#
# build.sh <out-dir> — 이 리포의 문서로 사이트를 만든다(영어 + /ko/). 배포하지 않는다.
#
# pqcota tools/site/build.sh(8a4e9bf0, v0.13.0)에서 가져와 pqcaton에 맞게 고쳤다. 고친 내역은 docs/site.md에 있다.
#
# **판은 입력을 믿지 않고 리포에서 구한다.** HEAD를 가리키는 vX.Y.Z 태그가 하나이고 그 태그의 커밋이 HEAD와 같으며 작업
# 트리가 깨끗해야 릴리스 판이다. 그 밖은 모두 개발판이고, 모든 쪽에 개발판 띠와 noindex가 붙는다(stamp.py).
# 도구 판(Python · pip freeze)이 고정한 것과 다르면 멈춘다. 링크 검사(check_site.py)가 0건이 아니면 멈춘다.
set -euo pipefail

OUT=${1:?usage: build.sh <out-dir>}
mkdir -p "$(dirname "$OUT")"; OUT=$(cd "$(dirname "$OUT")" && pwd)/$(basename "$OUT")
S=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$S/../.." && pwd)
PREFIX=/pqcaton
SITE_URL=https://www.sntsoft.co.kr/pqcaton/

# ── 도구 판 ────────────────────────────────────────────────────────────────
want_py=$(tr -d '[:space:]' < "$S/python-version")
got_py=$(python -c 'import platform; print(platform.python_version())')
[ "$got_py" = "$want_py" ] || { echo "site: Python $got_py, want $want_py (tools/site/python-version)"; exit 1; }
freeze=$(pip freeze --exclude pip --exclude setuptools --exclude wheel | sort -f)
diff <(grep -v '^#' "$S/freeze.txt" | sort -f) <(echo "$freeze") || { echo "site: pip freeze differs from tools/site/freeze.txt"; exit 1; }
zv=$(zensical --version)

# ── 판: 리포에서 구한다 ─────────────────────────────────────────────────────
HEAD=$(git -C "$REPO" rev-parse HEAD)
DIRTY=$(git -C "$REPO" status --porcelain --untracked-files=no | wc -l | tr -d ' ')
TAGS=()
while read -r t; do
  [ -n "$t" ] || continue
  [[ "$t" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || continue
  [ "$(git -C "$REPO" rev-parse "refs/tags/$t^{commit}")" = "$HEAD" ] && TAGS+=("$t")
done < <(git -C "$REPO" tag --points-at HEAD)
if [ "${#TAGS[@]}" -eq 1 ] && [ "$DIRTY" -eq 0 ]; then
  KIND=release; REF=${TAGS[0]}; LABEL="pqcaton $REF"
  NOTE="This documentation matches release $REF. The source documents in the repository take precedence."
  KNOTE="이 문서는 릴리스 $REF 기준입니다. 리포의 원본 문서가 우선합니다."
else
  KIND=development; REF=$HEAD; LABEL="pqcaton development ${HEAD:0:12}"
  [ "$DIRTY" -eq 0 ] || LABEL="$LABEL (uncommitted changes)"
  NOTE="Development build from ${HEAD:0:12}: not a release, not deployed."
  KNOTE="${HEAD:0:12}에서 만든 개발판입니다. 릴리스가 아니며 배포하지 않습니다."
fi
echo "site: $KIND $LABEL"

W=$(mktemp -d); WK=$(mktemp -d)
trap 'rm -rf "$W" "$WK"' EXIT

export PYTHONPATH="$S${PYTHONPATH:+:$PYTHONPATH}"   # mkdocs 설정이 slugs.github_slug를 읽는다
build() {  # build <dir> <lang> <pages> <base.yml> <label> <url> <note>
  python "$S/assemble.py" "$REPO" "$1" --lang "$2" --pages "$3" --ref "$REF" --site-prefix "$PREFIX" > "$1/assemble.txt"
  cat "$1/assemble.txt"
  ( cd "$1"
    cat "$4" nav.yml > mkdocs.yml
    sed -i "s#^site_name:.*#site_name: \"$5\"\nsite_url: $6\ncopyright: \"$7\"#" mkdocs.yml
    zensical build -f mkdocs.yml >/dev/null )
}
build "$W" en "$S/pages.txt" "$S/mkdocs.base.yml" "$LABEL" "$SITE_URL" "$NOTE"
build "$WK" ko "$S/pages.ko.txt" "$S/mkdocs.ko.base.yml" "$LABEL" "${SITE_URL}ko/" "$KNOTE"

rm -rf "$OUT"; mv "$W/site" "$OUT"
test ! -e "$OUT/ko" || { echo "site: the generated site made ko/ itself"; exit 1; }
mv "$WK/site" "$OUT/ko"
cat > "$OUT/ko/index.html" <<'KOHTML'
<!doctype html>
<html lang="ko"><head><meta charset="utf-8"><title>pqcaton</title>
<meta http-equiv="refresh" content="0; url=overview/"><link rel="canonical" href="overview/"></head>
<body><p><a href="overview/">pqcaton 개요로 이동합니다</a></p></body>
</html>
KOHTML

# ── 손으로 쓴 쪽과 원문 주소 보존 ───────────────────────────────────────────
test ! -e "$OUT/index.html" || { echo "site: the generated site made a root index.html"; exit 1; }
cp "$REPO/docs/index.html" "$OUT/index.html"
test ! -e "$OUT/ui-next.html" || { echo "site: the generated site made ui-next.html"; exit 1; }
cp "$REPO/docs/ui-next.html" "$OUT/ui-next.html"
# 손으로 쓴 쪽의 이 리포 main 링크는 지금 판으로 고정한다(원본은 그대로 둔다). 사이트는 한 판의 사본이다.
sed -i -E "s#https://github\.com/sntsoftgit/pqcaton/(blob|tree)/main/#https://github.com/sntsoftgit/pqcaton/\1/$REF/#g" "$OUT/index.html" "$OUT/ui-next.html"
N=0
while read -r f; do
  b=$(basename "$f")
  test ! -e "$OUT/$b" || { echo "site: the source $b collides with a generated file"; exit 1; }
  cp "$REPO/$f" "$OUT/$b"; cmp -s "$REPO/$f" "$OUT/$b" || { echo "site: copying $b changed it"; exit 1; }
  N=$((N + 1))
done < <(git -C "$REPO" ls-files 'docs/*.md')
[ "$N" -gt 0 ] || { echo "site: no docs/*.md to keep"; exit 1; }

python "$S/langswitch.py" "$OUT"
INFO=$(mktemp)
{ echo "tag: $([ "$KIND" = release ] && echo "$REF" || echo none)"
  echo "commit: $HEAD"
  echo "uncommitted changes: $DIRTY"
  echo "kept source documents: $N"
  echo "python: $got_py"
  echo "zensical: $zv"
  echo "requirements sha256: $(sha256sum "$S/requirements.txt" | cut -c1-64)"
  echo "pip freeze sha256: $(echo "$freeze" | sha256sum | cut -c1-64)"
  echo "tools: tools/site at $HEAD (based on pqcota tools/site 8a4e9bf060416533fab9d40dcce8427d81e10faa)"
} > "$INFO"
python "$S/stamp.py" "$OUT" --kind "$KIND" --label "$LABEL" --info "$INFO"
rm -f "$INFO"
python "$S/check_site.py" "$OUT" --prefix "$PREFIX" --repo "$REPO" --ref "$REF" | tee "$OUT.check.txt"
grep -q '^problems: 0$' "$OUT.check.txt" || { echo "site: the link check found problems"; exit 1; }
( cd "$OUT" && find . -type f -print0 | sort -z | xargs -0 sha256sum ) > "$OUT.tree.sha256"
echo "site: $KIND build in $OUT ($(find "$OUT" -name '*.html' | wc -l | tr -d ' ') html), check and tree hashes next to it"
