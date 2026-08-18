#!/usr/bin/env bash
#  확장 — 실행 중인 pqcota 디스커버리 데모에 3-상태 대조 + 거버넌스 토폴로지를 얹는다.
# 전제: pqcota/demo/scripts/{up,demo}.sh 로 환경이 떠 있고 디스커버리(/work/results)가 끝난 상태.
#       빌드 머신에 Go와 python3. pqcota v0.5.0부터 모듈 경로가 리포 주소와 같아 `go build`가
#       계약을 스스로 받아온다 — 형제 체크아웃도 gen 생성도 필요 없다.
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"   # pqcaton/demo
REPO_DIR="$(cd "$DEMO_DIR/.." && pwd)"                        # pqcaton

# pqcota 리포를 찾는다 - 없으면 무엇을 어떻게 주라는지 말하고 멈춘다.
PQCOTA_DIR="${PQCOTA_DIR:-$REPO_DIR/../pqcota}"
TOPO="$PQCOTA_DIR/demo/topology/topology.yaml"
[ -f "$TOPO" ] || TOPO="$PQCOTA_DIR/demo/topology/topology.example.yaml"
if [ ! -f "$TOPO" ]; then
  echo "❌ pqcota 리포를 찾지 못했다: $PQCOTA_DIR"
  echo "   선언을 그쪽 demo/topology/topology.yaml 에서 만든다 - 환경과 갈라지지 않게."
  echo "   PQCOTA_DIR=/경로/pqcota ./demo/scripts/extend.sh"
  exit 1
fi

docker inspect pqcota-ctl >/dev/null 2>&1 || { echo "❌ pqcota-ctl 없음 — 먼저 pqcota/demo/scripts/{up,demo}.sh"; exit 1; }
docker exec pqcota-ctl bash -lc 'ls /work/results/*.json >/dev/null 2>&1' || { echo "❌ 수집 결과 없음 — 먼저 pqcota/demo/scripts/demo.sh"; exit 1; }

echo "▶ 1/4  리포트 빌드 (pqcaton-report)…"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
( cd "$REPO_DIR" && CGO_ENABLED=0 go build -o "$TMP/pqcaton-report" ./inventory/cmd/pqcaton-report )
docker cp "$TMP/pqcaton-report" pqcota-ctl:/usr/local/bin/pqcaton-report

echo "▶ 2/4 선언 생성 (topology.yaml -> declaration) + 노드↔IP 주입…"
echo "   토폴로지: $TOPO"
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

echo "▶ 3/4 인벤토리 대조 + 거버넌스 토폴로지 (pqcaton-report)…"
docker exec pqcota-ctl bash -lc 'pqcaton-report /work/results /work/declaration.json /work/topology-governance.dot'

echo "▶ 4/4 토폴로지 SVG 렌더 + 회수…"
if docker exec pqcota-ctl bash -lc 'command -v dot >/dev/null && dot -Tsvg /work/topology-governance.dot -o /work/topology-governance.svg'; then
  docker cp pqcota-ctl:/work/topology-governance.svg "$DEMO_DIR/topology-governance.svg"
  echo "   → $DEMO_DIR/topology-governance.svg"
fi

echo
echo "✅  확장 완료. 관측 등급(pqcota) 위에 '선언 대비 CONFIRMED/shadow/미관측' + 리뷰 큐가 더해졌다."
echo "   정리: pqcota/demo/scripts/down.sh"
