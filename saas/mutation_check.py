#!/usr/bin/env python3
"""변이 확인 — 케이스가 실제로 무엇을 재는지 본다.

통과는 검증이 아니다. 지켜야 할 성질을 하나씩 깨 보고 **어느 케이스가 잡는지** 확인한다.
잡히지 않으면 그 케이스는 그 성질을 재고 있지 않은 것이다.

코드를 고치면 변이가 겨냥하던 자리가 사라지기도 한다. 그래서 손으로 하지 않고 여기 둔다 —
리팩터링 뒤에 이것이 통과하지 못하면 **잃은 보장이 있다**는 뜻이다.

    PQCATON_TEST_DSN=postgres://... python3 saas/mutation_check.py
"""
import os
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# (이름, 파일, [(바꿀 것, 바꿀 값), ...], 테스트 패키지, -run 패턴)
#
# 한 성질을 깨는 데 두 곳을 고쳐야 하는 변이가 있다. 한 곳만 고쳐 컴파일이 깨지면
# 테스트도 실패하는데, 그건 "케이스가 잡았다"가 아니다 — 아래에서 빌드를 먼저 본다.
MUTATIONS = [
    (
        "조직 격리 · ActiveKeys에서 org 조건 제거 → CP-ORG-1",
        "saas/internal/access/pg.go",
        [("WHERE org=$1 AND collector_id=$2 AND revoked_at IS NULL", "WHERE ($1 IS NOT NULL) AND collector_id=$2 AND revoked_at IS NULL")],
        "./saas/internal/access/...",
        "TestPgActiveKeysIsolatesOrg",
    ),
    (
        "조직 격리 · 검증자가 남의 조직 키를 조회 → CP-INTAKE-3",
        "saas/internal/intake/intake.go",
        [("o.Keys.ActiveKeys(o.Org, cid)", 'o.Keys.ActiveKeys("beta", cid)')],
        "./saas/internal/intake/...",
        "TestRejectsResultSignedByAnotherOrgsKey",
    ),
    (
        "멱등 · 적재 못 한 것의 확보를 안 놓음 → CP-INTAKE-6",
        "saas/internal/intake/intake.go",
        [("""		if err := o.Seen.Release(o.Org, claimed[i]); err != nil {
			return rep, fmt.Errorf("멱등 반환: %w", err)
		}""", "\t\t_ = claimed[i]")],
        "./saas/internal/intake/...",
        "TestRejectedResultCanBeRetriedAfterKeyIsRegistered",
    ),
    (
        "멱등 · 확보를 곧바로 되돌려 경합 창을 연다 → CP-INTAKE-11",
        "saas/internal/intake/intake.go",
        [("\t\tfresh = append(fresh, res)",
          "\t\t_ = o.Seen.Release(o.Org, h)\n\t\tfresh = append(fresh, res)")],
        "./saas/internal/intake/...",
        "TestConcurrentResendIsCountedOnce",
    ),
    (
        "멱등 · Claim이 영향 행 수를 안 봄 → CP-PG-5",
        "saas/internal/intake/seen.go",
        [("return tag.RowsAffected() == 1, nil", "_ = tag\n\treturn true, nil")],
        "./saas/internal/intake/...",
        "TestPgClaimIsAtomicUnderConcurrency",
    ),
    (
        "HTTP · 조직을 본문에서 읽음 → CP-HTTP-2",
        "saas/internal/api/api.go",
        [
            (
                '\tRunnerVersion string            `json:"runner_version"`',
                '\tOrg           string            `json:"org"`\n'
                '\tRunnerVersion string            `json:"runner_version"`',
            ),
            (
                "\tresults := make([]*discoveryv1.CollectionResult, 0, len(req.Results))",
                "\tif req.Org != \"\" {\n\t\to = org.ID(req.Org)\n\t}\n"
                "\tresults := make([]*discoveryv1.CollectionResult, 0, len(req.Results))",
            ),
        ],
        "./saas/internal/api/...",
        "TestOrgComesFromTokenNotBody",
    ),
    (
        "HTTP · 본문 상한을 안 걺 → CP-HTTP-4",
        "saas/internal/api/api.go",
        [("r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBody)", "// 상한 없음")],
        "./saas/internal/api/...",
        "TestOversizedBodyIsRejectedNotTruncated",
    ),
    (
        "HTTP · 인증 실패 사유를 응답에 담음 → CP-HTTP-3",
        "saas/internal/api/api.go",
        [('s.fail(w, http.StatusUnauthorized, "인증할 수 없다")', "s.fail(w, http.StatusUnauthorized, err.Error())")],
        "./saas/internal/api/...",
        "TestAuthFailuresAreIndistinguishable",
    ),
]


def run(cmd, **kw):
    return subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True, **kw)


def main():
    if run(["git", "status", "--porcelain"]).stdout.strip():
        print("작업 트리가 깨끗하지 않다 — 되돌리다 작업을 잃을 수 있어 멈춘다.", file=sys.stderr)
        return 2
    if not os.environ.get("PQCATON_TEST_DSN"):
        print("! PQCATON_TEST_DSN이 없다 — Postgres 변이는 스킵된다(통과가 아니다)\n")

    caught, missed = 0, 0
    for name, rel, edits, pkg, pattern in MUTATIONS:
        path = os.path.join(ROOT, rel)
        src = open(path, encoding="utf-8").read()
        bad = [old for old, _ in edits if src.count(old) != 1]
        if bad:
            # 못 찾았거나 여러 곳에 있다. 어느 쪽이든 이 변이는 더 이상 그 자리를
            # 겨냥하지 못한다 — 코드가 바뀌었다는 뜻이므로 실패로 센다.
            print(f"  ✗ {name}\n      겨냥할 자리를 찾지 못했다 — 코드가 바뀌었다")
            missed += 1
            continue
        mutated = src
        for old, new in edits:
            mutated = mutated.replace(old, new)
        try:
            open(path, "w", encoding="utf-8").write(mutated)
            built = run(["go", "build", "./..."]).returncode == 0
            ok = built and run(["go", "test", pkg, "-count=1", "-run", pattern]).returncode == 0
        finally:
            run(["git", "checkout", "--", rel])
        if not built:
            # 컴파일이 깨진 변이는 아무것도 증명하지 않는다 — 테스트는 어차피 실패한다.
            print(f"  ✗ {name}\n      변이가 컴파일되지 않는다 — 변이를 고쳐야 한다")
            missed += 1
            continue
        if ok:
            print(f"  ✗ {name}\n      깼는데 아무도 안 잡았다")
            missed += 1
        else:
            print(f"  ✓ {name}")
            caught += 1

    print(f"\n잡음 {caught} · 놓침 {missed}")
    return 0 if missed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
