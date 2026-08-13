#!/usr/bin/env bash
# 변이 확인 — 케이스가 실제로 무엇을 재는지 본다.
#
# 통과는 검증이 아니다. 지켜야 할 성질을 하나씩 깨 보고, 그때 **어느 케이스가 잡는지**를
# 확인한다. 잡히지 않으면 그 케이스는 그 성질을 재고 있지 않은 것이다.
#
# 코드를 고치면 변이가 겨냥하던 자리가 사라지기도 한다. 그래서 손으로 하지 않고 여기 둔다 —
# 리팩터링 뒤에 이 스크립트가 통과하지 못하면, 잃은 보장이 있다는 뜻이다.
#
#   PQCATON_TEST_DSN=postgres://... ./saas/mutation-check.sh
set -uo pipefail
cd "$(dirname "$0")/.."

if [ -n "$(git status --porcelain)" ]; then
  echo "작업 트리가 깨끗하지 않다 — 되돌리다 작업을 잃을 수 있어 멈춘다." >&2
  exit 2
fi
[ -n "${PQCATON_TEST_DSN:-}" ] || echo "! PQCATON_TEST_DSN이 없다 — Postgres 변이는 스킵된다(통과가 아니다)"

pass=0 fail=0

# mutate <이름> <파일> <파이썬 치환> <테스트 패키지> <-run 패턴>
mutate() {
  local name=$1 file=$2 py=$3 pkg=$4 pattern=$5
  python3 - "$file" <<PY || { echo "  ✗ $name — 변이를 적용하지 못했다(코드가 바뀌었나?)"; fail=$((fail+1)); return; }
import sys
p = sys.argv[1]
s = open(p).read()
$py
open(p, "w").write(s)
PY
  if go test "$pkg" -count=1 -run "$pattern" >/dev/null 2>&1; then
    echo "  ✗ $name — 깼는데 아무도 안 잡았다"
    fail=$((fail+1))
  else
    echo "  ✓ $name"
    pass=$((pass+1))
  fi
  git checkout -- "$file"
}

echo "── 조직 격리 ──"
mutate "ActiveKeys에서 org 조건 제거 → CP-ORG-1" \
  saas/internal/access/pg.go \
  's = s.replace("WHERE org=\$1 AND collector_id=\$2 AND revoked_at IS NULL", "WHERE (\$1 IS NOT NULL) AND collector_id=\$2 AND revoked_at IS NULL")' \
  ./saas/internal/access/... 'TestPgActiveKeysIsolatesOrg'

mutate "검증자가 남의 조직 키를 조회 → CP-INTAKE-3" \
  saas/internal/intake/intake.go \
  's = s.replace("o.Keys.ActiveKeys(o.Org, cid)", "o.Keys.ActiveKeys(\"beta\", cid)")' \
  ./saas/internal/intake/... 'TestRejectsResultSignedByAnotherOrgsKey'

echo "── 멱등 ──"
mutate "적재 못 한 것의 확보를 안 놓음 → CP-INTAKE-6" \
  saas/internal/intake/intake.go \
  's = s.replace("if err := o.Seen.Release(o.Org, claimed[i]); err != nil {", "if false {")' \
  ./saas/internal/intake/... 'TestRejectedResultCanBeRetriedAfterKeyIsRegistered'

mutate "확보를 적재 뒤로 미룸 → CP-INTAKE-11" \
  saas/internal/intake/intake.go \
  's = s.replace("ok, err := o.Seen.Claim(o.Org, h)", "ok, err := func() (bool, error) { seen, e := o.Seen.Claim(o.Org, h); if seen { _ = o.Seen.Release(o.Org, h) }; return seen, e }()")' \
  ./saas/internal/intake/... 'TestConcurrentResendIsCountedOnce'

mutate "Claim이 영향 행 수를 안 봄 → CP-PG-5" \
  saas/internal/intake/seen.go \
  's = s.replace("return tag.RowsAffected() == 1, nil", "_ = tag\\n\\treturn true, nil")' \
  ./saas/internal/intake/... 'TestPgClaimIsAtomicUnderConcurrency'

echo "── HTTP 경계 ──"
mutate "조직을 본문에서 읽음 → CP-HTTP-2" \
  saas/internal/api/api.go \
  's = s.replace("o := orgOf(r)", "o := orgOf(r)\\n\\tif v := r.Header.Get(\\"X-Org\\"); v != \\"\\" { o = org.ID(v) }")' \
  ./saas/internal/api/... 'TestOrgComesFromTokenNotBody'

echo
echo "잡음 $pass · 놓침 $fail"
[ "$fail" -eq 0 ] || exit 1
