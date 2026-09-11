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

# 기대 산출물을 쓰는 자리. **픽스처로 돌릴 때는 여기에 쓰지 않는다.** 픽스처는 선언에서 자산을
# 빼고 노드 하나를 별칭으로 바꾸므로, 그 결과를 기본 데모의 「예상 결과」로 올리면 공개 산출물에
# 토폴로지에 없는 노드 이름이 들어가고, 읽는 사람은 그것이 기본 실행의 결과라고 믿는다.
SAMPLE_DIR="$DEMO_DIR/expected-output"
if [ -n "${PQCATON_E2E_TRACE:-}" ]; then
  SAMPLE_DIR="$(mktemp -d)/expected-output"
  mkdir -p "$SAMPLE_DIR"
fi
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
# 픽스처가 고른 별칭을 bash 로 넘기는 자리. TMP 와 함께 지워진다.
export PQCATON_E2E_ALIAS_FILE="$TMP/e2e-alias"
ALIAS_FILE="$PQCATON_E2E_ALIAS_FILE"
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
import json, os, sys
decl  = json.load(open(sys.argv[1]))
nodes = json.load(open(sys.argv[2]))
decl["nodes"] = nodes
# PQCATON_E2E_TRACE — **되짚기 사슬을 재는 전용 픽스처다. 데모 서사가 아니다.**
#
# 선언에서 자산을 전부 빼면, 정책이 관리 대상으로 남긴 관측이 모두 UNDECLARED(그림자 자산)가 되어
# 리뷰 큐에 오르고 계획 칸을 든다. 그래야 **pqcaton 이 만든 계획으로** 승인 → 생성 → 레코드 →
# 실제 스냅샷 id 까지 한 바퀴를 돌 수 있다.
#
# 데모의 선언을 왜곡해 없는 자산을 지어내지 않는다. 빼기만 한다 — 빼면 관측된 것이 그대로 그림자가
# 되고, 그것은 실제로 있을 수 있는 상태다(선언을 아직 안 적은 조직).
if os.environ.get("PQCATON_E2E_TRACE"):
    decl["assets"] = []
    print("   [e2e fixture] declared assets cleared: every managed observation becomes UNDECLARED so the plan has something to carry")
    # **별칭 노드**(D9). 선언이 부르는 이름과 관측(봉투)이 부르는 이름이 다른 구성이다. 실제로
    # 흔하다 - collector 는 호스트명이나 자기가 붙인 id 로 보내고, CMDB 는 자기 이름을 쓴다.
    # 그 둘이 갈리면 조치는 선언 이름을 겨누고 스냅샷은 봉투 이름으로 저장돼 있어야 한다.
    orig = os.environ.get("PQCATON_E2E_ALIAS_NODE", "pay-db")
    if orig in decl.get("scope", []):
        alias = orig + "-cmdb"
        decl["scope"] = [alias if n == orig else n for n in decl["scope"]]
        for n in decl["nodes"]:
            if n["name"] == orig:
                n["name"] = alias
                n["observed_as"] = sorted(set(n.get("observed_as", []) + [orig]))
        for e in decl.get("edges", []):
            for k in ("src", "dst"):
                if e.get(k) == orig:
                    e[k] = alias
        open(os.environ["PQCATON_E2E_ALIAS_FILE"], "w").write("%s %s\n" % (alias, orig))
        print("   [e2e fixture] node %s is declared as %s and linked by observed_as" % (orig, alias))
known   = {n["name"] for n in decl["nodes"]}  # 픽스처가 이름을 바꿨으면 바뀐 이름으로 본다
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

# 별칭으로 적용하려면 인벤토리가 그 이름을 알아야 한다. 원래 호스트의 줄을 이름만 바꿔 더한다 -
# 현실에서 CMDB 이름을 인벤토리가 실제 주소로 잇는 것과 같은 일이다.
#
# **전달 파일은 이 실행의 임시 디렉터리에 둔다.** 고정 경로에 두면 실패한 픽스처 실행이 남긴
# 파일을 다음 기본 실행이 읽어, 픽스처를 켜지도 않았는데 별칭이 섞인다. 그리고 **픽스처를 켠
# 실행에서만 읽는다** - 조건 둘을 다 걸어야 한쪽이 틀려도 기본 실행이 오염되지 않는다.
if [ -n "${PQCATON_E2E_TRACE:-}" ] && [ -f "$ALIAS_FILE" ]; then
  read -r E2E_ALIAS E2E_ORIG < "$ALIAS_FILE"
  docker exec pqcota-ctl bash -lc "grep -q '^$E2E_ALIAS ' /work/ansible/targets.ini || sed -n 's/^$E2E_ORIG /$E2E_ALIAS /p' /work/ansible/targets.ini >> /work/ansible/targets.ini"
  docker exec pqcota-ctl bash -lc "grep -c '^$E2E_ALIAS ' /work/ansible/targets.ini" >/dev/null \
    || { echo "❌ could not add the alias $E2E_ALIAS to the inventory"; exit 1; }
  echo "   [e2e fixture] inventory now maps $E2E_ALIAS to the same host as $E2E_ORIG"
fi

echo "▶ 3/8 inventory reconciliation + governance topology (pqcaton-report)…"
# **콘솔 출력을 기대 파일로 그대로 갖고 온다.** 손으로 한 번 만들어 두면 그 순간부터
# 어긋난다 - 실제로 그렇게 낡아 있었고, 명령의 출력이 영어가 된 날에도 한국어인 채
# 남아 있었다. 스크립트가 뜨면 어긋날 수가 없다.
# **상류 적재와 같은 자산 스코프 정책을 건다.** pqcota 데모는 /work/scope-assets.csv 로 적재했다.
# 다른 정책(또는 정책 없음)으로 정규화하면 스냅샷 지문이 중앙 이력과 갈려, 아래 7/8 에서 생성기가
# 계획의 근거를 되짚지 못한다. 정책 유무를 추정하지 않는다 — 같은 파일을 준다.
SCOPE=/work/scope-assets.csv
docker exec pqcota-ctl bash -lc "test -f $SCOPE" || { echo "❌ $SCOPE not in pqcota-ctl — pqcota/demo/scripts/demo.sh writes it"; exit 1; }
docker exec -e PQCATON_SCOPE_ASSETS=$SCOPE pqcota-ctl bash -lc 'pqcaton-report /work/results /work/declaration.json /work/topology-governance.dot' \
  | tee "$SAMPLE_DIR/report.txt"

echo "▶ 4/8 judgment → judged plan, IN_REVIEW (pqcaton-decide)…"
# **대조에서 멈추지 않는다.** 여기까지만 돌리면 「관측을 판정으로 잇는다」가 데모에서
# 증명되지 않는다 - 실제로 그 구간이 v0.9.0 전까지 끊겨 있었고, 데모가 대조에서 멈춰서
# 아무도 몰랐다.
docker exec pqcota-ctl bash -lc \
  "pqcaton-decide open /work/declaration.json -results /work/results -scope-assets $SCOPE -org demo-corp > /work/session.json"
# 사람이 하는 자리를 데모에서는 정책 단위 일괄 판정으로 대신한다. **확정은 사람이 한다**는
# 원칙은 그대로다 - 여기서는 그 사람 역할을 스크립트가 맡는다.
# 화면에서 사람이 고르는 것을 여기서 대신 고른다. 결론만이 아니라 **조치 종류 · 목표 알고리즘 ·
# 위임 수준**까지다 — 비면 확정이 막힌다(v0.16.0).
#
# **실제로 관측된 자산**(CONFIRMED·UNDECLARED)만 계획에 넣는다. 조치는 있는 것을 바꾸는 일이다.
# UNOBSERVED 는 「없다」가 아니라 「못 봤다」라(§2.7) 조치 대상이 아니다 — 재수집이 먼저다. 그리고
# 그 근거를 상류 이력에서 되짚을 수 있어야 하므로, 여기 드는 자산은 정책이 관리 대상으로 남긴 것,
# 곧 중앙 이력의 스냅샷에 실제로 있는 것이다.
docker exec pqcota-ctl bash -lc 'python3 - <<PY
import json
s = json.load(open("/work/session.json"))
s["reviewer"], s["signature"] = "데모 판정자", "demo-judged"
for k in s["policy_decisions"]:
    s["policy_decisions"][k] = "PQC 라이브러리로 교체한다"
n = 0
for it in s["items"]:
    if it["state"] not in ("CONFIRMED", "UNDECLARED"):
        continue
    it["include_in_plan"] = True
    it["remediation_kind"] = "REMEDIATION_KIND_CONFIG_ONLY"
    it["target_algorithm"] = "ML-KEM (FIPS 203)"
    it["deploy_level"] = "L2"
    n += 1
json.dump(s, open("/work/session.json", "w"), ensure_ascii=False, indent=2)
print("   %d observed asset(s) go into the plan (CONFIRMED or UNDECLARED) · session %s" % (n, s.get("session_id", "?")))
PY'
docker exec pqcota-ctl bash -lc \
  'pqcaton-decide close /work/session.json -org demo-corp -judgments /work/judgments.jsonl > /work/plan.json'
docker cp pqcota-ctl:/work/plan.json "$SAMPLE_DIR/plan.json" 2>/dev/null || true
# **공백과 세션 id 를 고른다.** protojson 은 콜론 뒤 공백을 일부러 흔들고, 계획 id 에는 실행마다
# 새로 뽑는 세션 UUID 가 들어 있다. 그대로 두면 데모를 돌릴 때마다 기대 파일이 더러워져
# **진짜 달라진 날을 알아볼 수 없다.** 세션 id 는 자리만 보이면 되므로 고정값으로 바꾼다 -
# 실제 실행의 id 는 콘솔과 판정 원장에 남는다.
python3 - "$SAMPLE_DIR/plan.json" <<'PYFMT' || true
import json, re, sys
p = sys.argv[1]
d = json.load(open(p))
FIXED = "00000000-0000-4000-8000-000000000000"
uuid = r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"
if isinstance(d.get("id"), str):
    d["id"] = re.sub(uuid, FIXED, d["id"])
json.dump(d, open(p, "w"), indent=2, ensure_ascii=False, sort_keys=False)
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

# 계획에 조치가 없으면 승인부터는 돌 것이 없다. **실패가 아니다** — 자산 스코프 정책이 관리
# 대상에서 뺀 것을 빼고 나면 이 환경에는 사람이 판정해 조치할 자산이 남지 않는다는 뜻이고, 그것이
# 맞는 결과다. 억지로 조치를 만들면 관리하지 않기로 한 자산에 계획을 세우게 된다.
#
# 여기서 드러난 것: **자동통과(CONFIRMED·고신뢰)한 자산을 조치로 가져가는 길이 없다.** 자동통과는
# 리뷰 큐에 항목으로 들어가지 않아 계획 칸을 들지 못한다. 자동통과는 「사람이 볼 필요가 없다」는
# 뜻이지 「바꿀 필요가 없다」가 아닌데, 지금 구조로는 그것을 계획에 넣을 수 없다.
ACTIONS=$(docker exec pqcota-ctl bash -lc 'python3 - <<PY
import json
print(len(json.load(open("/work/plan.json")).get("actions", [])))
PY' | tr -d '[:space:]')
if [ "$ACTIONS" = "0" ]; then
  echo
  echo "ℹ  no action in the judged plan — after the asset-scope policy, nothing in the review queue is an observed asset."
  echo "   CONFIRMED assets auto-passed and cannot be taken into a plan today; UNOBSERVED is 'not seen', not 'not there' (§2.7)."
  echo "   Approval → generation → resolution is exercised by pqcota's own demo (it resolves its evidence against this same history)."
  echo "   clean up: pqcota/demo/scripts/down.sh"
  exit 0
fi

echo "▶ 6/8 execution approval (pqcota-approve, upstream) — a second approver, with their own key…"
# 판정한 사람과 실행을 승인하는 사람은 다르다. 데모 승인자(reviewer-1)는 pqcota 데모가 만들었고,
# 거버넌스 쪽 승인자는 여기서 키를 만들어 등록한다. 등록하지 않으면 생성기가 「확인할 수 없다」로
# 거절한다 — 그것이 맞는 동작이다.
GOV_KEYS=$(docker exec pqcota-ctl bash -lc 'pqcota-keygen')
GOV_PRIV=$(echo "$GOV_KEYS" | grep '^PQCOTA_SIGN_KEY=' | cut -d= -f2-)
GOV_PUB=$(echo "$GOV_KEYS" | grep '^PQCOTA_VERIFY_KEY=' | cut -d= -f2-)
# 같은 환경에서 다시 돌려도 되게, 앞서 등록한 governance-1 은 걷어내고 다시 넣는다. 한 id 에 키가
# 둘이면 상류가 「어느 것이 그 사람의 키인지 말할 수 없다」며 거절한다 — 그것이 맞는 동작이다.
docker exec pqcota-ctl bash -lc "sed -i -e 's|,governance-1=[^,]*||g' -e 's|^export PQCOTA_APPROVAL_KEYS=\(.*\)\$|export PQCOTA_APPROVAL_KEYS=\1,governance-1=$GOV_PUB|' /etc/profile.d/pqcota-approval.sh"
docker exec -e PQCOTA_APPROVAL_KEY="$GOV_PRIV" pqcota-ctl bash -lc \
  'pqcota-approve --approver governance-1 /work/plan.json > /work/plan.approved.json' 2>&1 | sed 's/^/   /'
docker exec pqcota-ctl bash -lc 'python3 - <<PY
import json, sys
p = json.load(open("/work/plan.approved.json"))
print("   status=%s approvals=%d finalizedAt=%s" % (p["status"], len(p["approvalSignatures"]), p.get("finalizedAt")))
if p["status"] != "PLAN_STATUS_FINALIZED" or not p.get("finalizedAt"):
    sys.exit("❌ approval must raise the plan to FINALIZED and stamp finalized_at")
PY'

echo "▶ 7/8 generate (pqcota-provision, §3.7 gate) — resolve the plan's evidence in the history, then actually apply…"
ANS="cd /work/ansible && ansible"
INV="-i /work/ansible/targets.ini -i /work/ansible/groups.ini"
# --dsn 을 준다: 조치의 근거(원천 노드 · v1 지문 · 규칙 판)를 중앙 이력에서 **실제로 찾아** 레코드에
# 남긴다. 못 찾으면 종료 3 이다 — 이 데모가 보이려는 것이 바로 그 고리다.
DSN="postgres://postgres:pqcota@pqcota-demo-pg:5432/pqcota"
set +e
docker exec pqcota-ctl bash -lc "pqcota-provision --level l2 --dsn '$DSN' /work/plan.approved.json > /work/ansible/provision-gov.yml" 2>&1 | sed 's/^/   /'
st=${PIPESTATUS[0]}
set -e
case "$st" in
  0) echo "   ✓ every spot filled — the evidence resolved to real snapshots";;
  3) echo "❌ exit 3: the generator names a blank above. Evidence that does not resolve is the thing this demo must catch"; exit 1;;
  *) echo "❌ pqcota-provision refused (exit $st)"; exit 1;;
esac
echo "   ── the record now points back at the snapshot (pqcota-records) ──"
NODE0=$(docker exec pqcota-ctl bash -lc 'python3 -c "import json; print(json.load(open(\"/work/plan.approved.json\"))[\"actions\"][0][\"targetNodeId\"])"' | tr -d '[:space:]')
docker exec -e PQCOTA_DSN="$DSN" pqcota-ctl bash -lc "pqcota-records $NODE0" 2>&1 | grep -E 'snapshot:|plan=' | sed 's/^/   /'
docker exec -e PQCOTA_DSN="$DSN" pqcota-ctl bash -lc "pqcota-records $NODE0" 2>&1 | grep -q 'snapshot: ingest-' \
  || { echo "❌ the record does not name a resolved snapshot"; exit 1; }
NODES=$(docker exec pqcota-ctl bash -lc 'python3 -c "import json; print(\" \".join(sorted({a[\"targetNodeId\"] for a in json.load(open(\"/work/plan.approved.json\"))[\"actions\"]})))"')
echo "   target nodes: $NODES"
docker exec pqcota-ctl bash -lc "$ANS-playbook $INV provision-gov.yml" | grep -E "ok=|changed=|failed=" | sed 's/^/   /'
# 별칭은 컨테이너 이름이 아니다. 파일을 확인할 때는 원래 이름으로 들어간다.
host_of() { if [ -n "${E2E_ALIAS:-}" ] && [ "$1" = "$E2E_ALIAS" ]; then echo "$E2E_ORIG"; else echo "$1"; fi; }
for n in $NODES; do
  docker exec "$(host_of "$n")" sh -lc 'ls -l /etc/pqcota/ 2>/dev/null' | sed "s/^/   $n │ /"
done

# ★ 별칭 사슬(D9)을 값으로 확인한다: 조치는 선언 별칭을 겨누고, 근거는 봉투 이름을 들고,
# 해결된 스냅샷은 그 봉투 이름의 것이다. 셋이 어긋나면 별칭을 쓴 정상 구성에서 되짚기가 실패한다.
if [ -n "${E2E_ALIAS:-}" ]; then
  echo "   ── the alias chain: declared name ≠ envelope name, and the snapshot resolves under the envelope name ──"
  docker exec pqcota-ctl bash -lc "ALIAS=$E2E_ALIAS ORIG=$E2E_ORIG python3 - <<PY
import json, os, sys
alias, orig = os.environ[\"ALIAS\"], os.environ[\"ORIG\"]
p = json.load(open(\"/work/plan.approved.json\"))
hit = [a for a in p[\"actions\"] if a[\"targetNodeId\"] == alias]
if not hit:
    sys.exit(\"the plan has no action targeting the declared alias %s\" % alias)
a = hit[0]
e = a[\"evidenceSources\"][0][\"snapshot\"]
print(\"   action %s targetNodeId=%s (declared alias)\" % (a[\"id\"], a[\"targetNodeId\"]))
print(\"   evidence sourceNodeId=%s (envelope name)\" % e[\"sourceNodeId\"])
if e[\"sourceNodeId\"] != orig:
    sys.exit(\"the evidence names %s, not the envelope name %s\" % (e[\"sourceNodeId\"], orig))
if a[\"targetNodeId\"] == e[\"sourceNodeId\"]:
    sys.exit(\"target and source are the same - this fixture is not exercising the alias\")
PY" || { echo "❌ the alias chain is broken"; exit 1; }
  # 레코드는 **조치의 노드**, 곧 별칭 아래 쌓인다. 그 레코드가 원천 노드의 스냅샷을 가리켜야 한다.
  docker exec -e PQCOTA_DSN="$DSN" pqcota-ctl bash -lc "pqcota-records $E2E_ALIAS" 2>&1 \
    | grep 'snapshot:' | sed 's/^/   resolved /' \
    || { echo "❌ no record under the alias names a resolved snapshot"; exit 1; }
  docker exec -e PQCOTA_DSN="$DSN" pqcota-ctl bash -lc "pqcota-records $E2E_ALIAS" 2>&1 \
    | grep -q "snapshot: .*:$E2E_ORIG\$" \
    || { echo "❌ the resolved snapshot is not the source node's"; exit 1; }
fi

echo "▶ 8/8 roll back (--rollback) — remove what this plan staged…"
docker exec pqcota-ctl bash -lc 'pqcota-provision --level l2 --rollback /work/plan.approved.json > /work/ansible/provision-gov-rollback.yml' 2>/dev/null || true
docker exec pqcota-ctl bash -lc "$ANS-playbook $INV provision-gov-rollback.yml" | grep -E "ok=|changed=|failed=" | sed 's/^/   /'
for n in $NODES; do
  docker exec "$(host_of "$n")" sh -lc 'ls /etc/pqcota/ 2>/dev/null | wc -l' | sed "s/^/   $n │ files left: /"
done

echo
echo "✅  Done. observation (pqcota) → reconciliation → judgment (IN_REVIEW) → approval (FINALIZED) → generate → apply → roll back, one full turn."
echo "   judged plan: /work/plan.json · approved: /work/plan.approved.json"
echo "   clean up: pqcota/demo/scripts/down.sh"
