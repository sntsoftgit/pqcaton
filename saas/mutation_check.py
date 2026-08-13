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
        "조직 격리 · Lease 질의에서 org 조건 제거 → CP-PG-8",
        "saas/internal/jobs/pg.go",
        [("      WHERE org=$1 AND state='pending'", "      WHERE ($1 IS NOT NULL) AND state='pending'")],
        "./saas/internal/jobs/...",
        "TestPgLeaseIsolatesOrg",
    ),
    (
        "동시 점유 · 고르는 것과 표시하는 것 사이를 벌림 → CP-PG-9",
        "saas/internal/jobs/pg.go",
        # 같은 줄이 Sweep에도 있다 — LIMIT과 함께 잡아 Lease 쪽만 겨냥한다.
        [("		      FOR UPDATE SKIP LOCKED\n		      LIMIT 1\n", "		      LIMIT 1\n")],
        "./saas/internal/jobs/...",
        "TestPgLeaseIsExclusiveUnderConcurrency",
    ),
    (
        "재배포 정책 · Pg 정리가 종류를 안 가림 → CP-PG-10",
        "saas/internal/jobs/pg.go",
        [("		if k.Redeployable() {", "		if true {")],
        "./saas/internal/jobs/...",
        "TestPgSweepFollowsKindPolicy",
    ),
    (
        "재배포 정책 · 모르는 종류가 자동 재배포로 떨어짐 → CP-JOB-10",
        "saas/internal/jobs/jobs.go",
        [("func (k Kind) Redeployable() bool { return k == Enroll || k == Observe }",
          "func (k Kind) Redeployable() bool { return k != Provision }")],
        "./saas/internal/jobs/...",
        "TestRedeployablePolicy",
    ),
    (
        "완료 보고 · 닫지도 않고 닫혔다고 답함 → CP-HTTP-13",
        "saas/internal/api/api.go",
        [("		resp.Job = s.closeJob(o, req.JobID, req.RunnerID)", "		resp.Job = jobClosed")],
        "./saas/internal/api/...",
        "TestResultsCloseTheirJob",
    ),
    (
        "완료 보고 · 남의 점유를 닫고도 닫혔다고 답함 → CP-HTTP-14",
        "saas/internal/api/api.go",
        [("		return jobNotLeased", "		return jobClosed")],
        "./saas/internal/api/...",
        "TestResultsDoNotCloseAnotherRunnersJob",
    ),
    (
        "완료 보고 · 작업을 못 닫으면 결과까지 버림 → CP-HTTP-15",
        "saas/internal/api/api.go",
        [("		resp.Job = s.closeJob(o, req.JobID, req.RunnerID)\n	}",
          "		resp.Job = s.closeJob(o, req.JobID, req.RunnerID)\n"
          "		if resp.Job != jobClosed {\n"
          "			s.fail(w, http.StatusConflict, \"작업을 닫을 수 없다\")\n"
          "			return\n"
          "		}\n	}")],
        "./saas/internal/api/...",
        "TestUnknownJobDoesNotDropResults",
    ),
    (
        "롱폴 · 조직을 질의 문자열에서 읽음 → CP-HTTP-10",
        "saas/internal/api/api.go",
        [('\trunnerID := strings.TrimSpace(r.URL.Query().Get("runner_id"))',
          '\tif q := r.URL.Query().Get("org"); q != "" {\n\t\to = org.ID(q)\n\t}\n'
          '\trunnerID := strings.TrimSpace(r.URL.Query().Get("runner_id"))')],
        "./saas/internal/api/...",
        "TestJobsOrgComesFromTokenNotQuery",
    ),
    (
        "롱폴 · 기다리지 않고 곧장 없다고 답함 → CP-HTTP-9",
        "saas/internal/api/api.go",
        [("\tctx, cancel := context.WithTimeout(r.Context(), wait)",
          "\t_ = wait\n\tctx, cancel := context.WithTimeout(r.Context(), 0)")],
        "./saas/internal/api/...",
        "TestLongPollDeliversJobThatArrivesWhileWaiting",
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
