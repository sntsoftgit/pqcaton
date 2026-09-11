#!/usr/bin/env bash
#  확장 — 실행 중인 pqcota 디스커버리 데모에 3-상태 대조 + 거버넌스 토폴로지를 얹고,
#  판정한 계획을 **상류의 승인 → 생성 → 적용 → 되돌림까지** 실제로 돌린다.
# 전제: pqcota/demo/scripts/{up,demo}.sh 로 환경이 떠 있고 디스커버리(/work/results)가 끝난 상태.
#       빌드 머신에 Go와 python3. pqcota v0.5.0부터 모듈 경로가 리포 주소와 같아 `go build`가
#       계약을 스스로 받아온다 — 형제 체크아웃도 gen 생성도 필요 없다.
#       Go 가 없는 호스트에서는 미리 빌드한 둘을 PQCATON_BIN_DIR 로 준다(pqcaton-report · pqcaton-decide).
#
# 판정과 실행 승인은 다른 단계다. pqcaton-decide close 가 내는 계획은 IN_REVIEW 이고 승인 칸이
# 비어 있다. 상류의 pqcota-approve 가 FINALIZED 로 올리며 서명해야 생성기가 받는다. 그 사이가
# 끊겨 있으면 여기서 드러나야 한다 — v0.16.0 은 close 에서 이미 끊겨 있었는데 아무도 돌리지 않아
# 몰랐다(조치 종류를 고르지 않은 항목이 확정을 막게 됐는데, 이 스크립트는 고르지 않았다).
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"   # pqcaton/demo
REPO_DIR="$(cd "$DEMO_DIR/.." && pwd)"                        # pqcaton

# pqcota 리포를 찾는다 - 없으면 무엇을 어떻게 주라는지 말하고 멈춘다.
PQCOTA_DIR="${PQCOTA_DIR:-$REPO_DIR/../pqcota}"
TOPO="$PQCOTA_DIR/demo/topology/topology.yaml"
[ -f "$TOPO" ] || TOPO="$PQCOTA_DIR/demo/topology/topology.example.yaml"
if [ ! -f "$TOPO" ]; then
  echo "❌ pqcota repo not found: $PQCOTA_DIR"
  echo "   The declaration is generated from its demo/topology/topology.yaml, so it cannot drift from the environment."
  echo "   PQCOTA_DIR=/path/to/pqcota ./demo/scripts/extend.sh"
  exit 1
fi

docker inspect pqcota-ctl >/dev/null 2>&1 || { echo "❌ no pqcota-ctl — run pqcota/demo/scripts/{up,demo}.sh first"; exit 1; }
docker exec pqcota-ctl bash -lc 'ls /work/results/*.json >/dev/null 2>&1' || { echo "❌ no collected results — run pqcota/demo/scripts/demo.sh first"; exit 1; }

echo "▶ 1/8  build (pqcaton-report · pqcaton-decide)…"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
if [ -n "${PQCATON_BIN_DIR:-}" ]; then
  echo "   using prebuilt binaries from $PQCATON_BIN_DIR"
  cp "$PQCATON_BIN_DIR/pqcaton-report" "$PQCATON_BIN_DIR/pqcaton-decide" "$TMP/"
else
  ( cd "$REPO_DIR" && CGO_ENABLED=0 go build -o "$TMP/pqcaton-report" ./inventory/cmd/pqcaton-report )
  ( cd "$REPO_DIR" && CGO_ENABLED=0 go build -o "$TMP/pqcaton-decide" ./inventory/cmd/pqcaton-decide )
fi
docker cp "$TMP/pqcaton-report" pqcota-ctl:/usr/local/bin/pqcaton-report
docker cp "$TMP/pqcaton-decide" pqcota-ctl:/usr/local/bin/pqcaton-decide

echo "▶ 2/8 generate the declaration (topology.yaml -> declaration) and inject node↔IP…"
echo "   topology: $TOPO"
# **선언을 환경에서 끌어온다.** 노드 이름을 우리 파일에 박아 두면 상류가 토폴로지를 고칠 때
# 조용히 어긋난다. 규칙(무엇을 선언하고 무엇을 일부러 뺄지)은 declare.py 에 있다.
DECL="$TMP/declaration.json"
python3 "$DEMO_DIR/scripts/declare.py" "$TOPO" > "$TMP/declaration.gen.json"
docker exec pqcota-ctl bash -lc 'cat /work/nodes.json' > "$TMP/nodes.json"
python3 - "$TMP/declaration.gen.json" "$TMP/nodes.json" "$DECL" <<'PYIN'
import json, sys
decl  = json.load(open(sys.argv[1]))
nodes = json.load(open(sys.argv[2]))
decl["nodes"] = nodes
known   = {n["name"] for n in nodes}
missing = [n for n in decl.get("scope", []) if n not in known]
if missing:
    # 토폴로지에서 만든 선언인데 실행 중 환경과 다르다 - 파서가 어긋났거나, up.sh 를 돌린
    # 뒤에 topology.yaml 을 고쳤다는 뜻이다. 어느 쪽이든 그대로 두면 결과가 거짓이 된다.
    sys.exit("❌ 선언의 노드가 이 환경에 없다: %s\n   환경의 노드: %s\n"
             "   topology.yaml 을 고쳤다면 up.sh 부터 다시 돌리라." % (missing, sorted(known)))
json.dump(decl, open(sys.argv[3], "w"), ensure_ascii=False, indent=2)
print("   " + " · ".join("%s=%s" % (n["name"], ",".join(n["ips"])) for n in nodes))
PYIN
docker cp "$DECL" pqcota-ctl:/work/declaration.json

echo "▶ 3/8 inventory reconciliation + governance topology (pqcaton-report)…"
# **콘솔 출력을 기대 파일로 그대로 갖고 온다.** 손으로 한 번 만들어 두면 그 순간부터
# 어긋난다 - 실제로 그렇게 낡아 있었고, 명령의 출력이 영어가 된 날에도 한국어인 채
# 남아 있었다. 스크립트가 뜨면 어긋날 수가 없다.
docker exec pqcota-ctl bash -lc 'pqcaton-report /work/results /work/declaration.json /work/topology-governance.dot' \
  | tee "$DEMO_DIR/expected-output/report.txt"

echo "▶ 4/8 judgment → judged plan, IN_REVIEW (pqcaton-decide)…"
# **대조에서 멈추지 않는다.** 여기까지만 돌리면 「관측을 판정으로 잇는다」가 데모에서
# 증명되지 않는다 - 실제로 그 구간이 v0.9.0 전까지 끊겨 있었고, 데모가 대조에서 멈춰서
# 아무도 몰랐다.
docker exec pqcota-ctl bash -lc \
  'pqcaton-decide open /work/declaration.json -results /work/results -org demo-corp > /work/session.json'
# 사람이 하는 자리를 데모에서는 정책 단위 일괄 판정으로 대신한다. **확정은 사람이 한다**는
# 원칙은 그대로다 - 여기서는 그 사람 역할을 스크립트가 맡는다.
# 화면에서 사람이 고르는 것을 여기서 대신 고른다. 결론만이 아니라 **조치 종류 · 목표 알고리즘 ·
# 위임 수준**까지다 — 비면 확정이 막힌다(v0.16.0). 관측된 것(UNDECLARED)만 계획에 넣는다.
# 관측되지 않은 것(UNOBSERVED)은 「없다」가 아니라 「못 봤다」라 조치 대상이 아니다.
docker exec pqcota-ctl bash -lc 'python3 - <<PY
import json
s = json.load(open("/work/session.json"))
s["reviewer"], s["signature"] = "데모 판정자", "demo-judged"
for k in s["policy_decisions"]:
    s["policy_decisions"][k] = "PQC 라이브러리로 교체한다"
n = 0
for it in s["items"]:
    if it["state"] != "UNDECLARED":
        continue
    it["include_in_plan"] = True
    it["remediation_kind"] = "REMEDIATION_KIND_CONFIG_ONLY"
    it["target_algorithm"] = "ML-KEM (FIPS 203)"
    it["deploy_level"] = "L2"
    n += 1
json.dump(s, open("/work/session.json", "w"), ensure_ascii=False, indent=2)
print("   %d item(s) go into the plan (UNDECLARED only) · session %s" % (n, s.get("session_id", "?")))
PY'
docker exec pqcota-ctl bash -lc \
  'pqcaton-decide close /work/session.json -org demo-corp -judgments /work/judgments.jsonl > /work/plan.json'
docker cp pqcota-ctl:/work/plan.json "$DEMO_DIR/expected-output/plan.json" 2>/dev/null || true
# **공백을 고른다.** protojson 은 콜론 뒤 공백을 일부러 흔들어, 같은 계획을 두 번 내도
# 파일이 달라진다. 그대로 두면 데모를 돌릴 때마다 기대 파일이 더러워져 **진짜 달라진
# 날을 알아볼 수 없다.**
python3 - "$DEMO_DIR/expected-output/plan.json" <<'PYFMT' || true
import json, sys
p = sys.argv[1]
json.dump(json.load(open(p)), open(p, "w"), indent=2, ensure_ascii=False, sort_keys=False)
open(p, "a").write("\n")
PYFMT

echo "▶ 5/8 render the topology SVG and collect it…"
if docker exec pqcota-ctl bash -lc 'command -v dot >/dev/null && dot -Tsvg /work/topology-governance.dot -o /work/topology-governance.svg'; then
  docker cp pqcota-ctl:/work/topology-governance.svg "$DEMO_DIR/topology-governance.svg"
  echo "   → $DEMO_DIR/topology-governance.svg"
fi

# 계약으로 나간 계획은 IN_REVIEW 이고 승인 칸이 비어 있어야 한다. 그렇지 않으면 이 리포가
# 실행 승인의 자리에 무언가를 넣은 것이다.
docker exec pqcota-ctl bash -lc 'python3 - <<PY
import json, sys
p = json.load(open("/work/plan.json"))
st, sigs, at = p.get("status"), p.get("approvalSignatures", []), p.get("finalizedAt")
print("   status=%s approvals=%d finalizedAt=%s id=%s" % (st, len(sigs), at, p.get("id")))
if st != "PLAN_STATUS_IN_REVIEW" or sigs or at:
    sys.exit("❌ a judged plan must leave IN_REVIEW with the approval slot and finalized_at empty")
PY'

echo "▶ 6/8 execution approval (pqcota-approve, upstream) — a second approver, with their own key…"
# 판정한 사람과 실행을 승인하는 사람은 다르다. 데모 승인자(reviewer-1)는 pqcota 데모가 만들었고,
# 거버넌스 쪽 승인자는 여기서 키를 만들어 등록한다. 등록하지 않으면 생성기가 「확인할 수 없다」로
# 거절한다 — 그것이 맞는 동작이다.
GOV_KEYS=$(docker exec pqcota-ctl bash -lc 'pqcota-keygen')
GOV_PRIV=$(echo "$GOV_KEYS" | grep '^PQCOTA_SIGN_KEY=' | cut -d= -f2-)
GOV_PUB=$(echo "$GOV_KEYS" | grep '^PQCOTA_VERIFY_KEY=' | cut -d= -f2-)
docker exec pqcota-ctl bash -lc "sed -i 's|^export PQCOTA_APPROVAL_KEYS=\(.*\)\$|export PQCOTA_APPROVAL_KEYS=\1,governance-1=$GOV_PUB|' /etc/profile.d/pqcota-approval.sh"
docker exec -e PQCOTA_APPROVAL_KEY="$GOV_PRIV" pqcota-ctl bash -lc \
  'pqcota-approve --approver governance-1 /work/plan.json > /work/plan.approved.json' 2>&1 | sed 's/^/   /'
docker exec pqcota-ctl bash -lc 'python3 - <<PY
import json, sys
p = json.load(open("/work/plan.approved.json"))
print("   status=%s approvals=%d finalizedAt=%s" % (p["status"], len(p["approvalSignatures"]), p.get("finalizedAt")))
if p["status"] != "PLAN_STATUS_FINALIZED" or not p.get("finalizedAt"):
    sys.exit("❌ approval must raise the plan to FINALIZED and stamp finalized_at")
PY'

echo "▶ 7/8 generate (pqcota-provision, §3.7 gate) and actually apply the playbook…"
ANS="cd /work/ansible && ansible"
INV="-i /work/ansible/targets.ini -i /work/ansible/groups.ini"
set +e
docker exec pqcota-ctl bash -lc 'pqcota-provision --level l2 /work/plan.approved.json > /work/ansible/provision-gov.yml' 2>&1 | sed 's/^/   /'
st=${PIPESTATUS[0]}
set -e
case "$st" in
  0) ;;
  3) echo "   ↑ exit 3: the playbook is out but the plan has blanks the generator names above";;
  *) echo "❌ pqcota-provision refused (exit $st)"; exit 1;;
esac
NODES=$(docker exec pqcota-ctl bash -lc 'python3 -c "import json; print(\" \".join(sorted({a[\"targetNodeId\"] for a in json.load(open(\"/work/plan.approved.json\"))[\"actions\"]})))"')
echo "   target nodes: $NODES"
docker exec pqcota-ctl bash -lc "$ANS-playbook $INV provision-gov.yml" | grep -E "ok=|changed=|failed=" | sed 's/^/   /'
for n in $NODES; do
  docker exec "$n" sh -lc 'ls -l /etc/pqcota/*.cnf /etc/pqcota/*.properties 2>/dev/null' | sed "s/^/   $n │ /"
done

echo "▶ 8/8 roll back (--rollback) — remove what this plan staged…"
docker exec pqcota-ctl bash -lc 'pqcota-provision --level l2 --rollback /work/plan.approved.json > /work/ansible/provision-gov-rollback.yml' 2>/dev/null || true
docker exec pqcota-ctl bash -lc "$ANS-playbook $INV provision-gov-rollback.yml" | grep -E "ok=|changed=|failed=" | sed 's/^/   /'
for n in $NODES; do
  docker exec "$n" sh -lc 'ls /etc/pqcota/ 2>&1 | grep -c "cnf\|properties" || true' | sed "s/^/   $n │ files left: /"
done

echo
echo "✅  Done. observation (pqcota) → reconciliation → judgment (IN_REVIEW) → approval (FINALIZED) → generate → apply → roll back, one full turn."
echo "   judged plan: /work/plan.json · approved: /work/plan.approved.json"
echo "   clean up: pqcota/demo/scripts/down.sh"
