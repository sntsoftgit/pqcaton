#!/usr/bin/env bash
#  확장 — 실행 중인 pqcota 디스커버리 데모에 3-상태 대조 + 거버넌스 토폴로지를 얹는다.
# 전제: pqcota/demo/scripts/{up,demo}.sh 로 환경이 떠 있고 디스커버리(/work/results)가 끝난 상태.
#       빌드 머신에 Go + 형제 pqcota(gen 생성됨: `cd ../pqcota && make generate`).
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"   # pqcota-enterprise/demo
ENT_DIR="$(cd "$DEMO_DIR/.." && pwd)"                          # pqcota-enterprise
NODES=(node-web node-app node-db)

docker inspect pqcota-ctl >/dev/null 2>&1 || { echo "❌ pqcota-ctl 없음 — 먼저 pqcota/demo/scripts/{up,demo}.sh"; exit 1; }
docker exec pqcota-ctl bash -lc 'ls /work/results/*.json >/dev/null 2>&1' || { echo "❌ 수집 결과 없음 — 먼저 pqcota/demo/scripts/demo.sh"; exit 1; }

echo "▶ 1/4  리포트 빌드 (pqcota-report)…"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
( cd "$ENT_DIR" && CGO_ENABLED=0 go build -o "$TMP/pqcota-report" ./inventory/cmd/pqcota-report )
docker cp "$TMP/pqcota-report" pqcota-ctl:/usr/local/bin/pqcota-report

echo "▶ 2/4 스코프 선언 생성 (컨테이너 IP 주입)…"
DECL="$TMP/declaration.json"; cp "$DEMO_DIR/declaration.json" "$DECL"
for n in "${NODES[@]}"; do
  ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$n")
  sed -i "s/__IP_${n}__/${ip}/g" "$DECL"
  echo "   $n = $ip"
done
docker cp "$DECL" pqcota-ctl:/work/declaration.json

echo "▶ 3/4 인벤토리 대조 + 거버넌스 토폴로지 (pqcota-report)…"
docker exec pqcota-ctl bash -lc 'pqcota-report /work/results /work/declaration.json /work/topology-governance.dot'

echo "▶ 4/4 토폴로지 SVG 렌더 + 회수…"
if docker exec pqcota-ctl bash -lc 'command -v dot >/dev/null && dot -Tsvg /work/topology-governance.dot -o /work/topology-governance.svg'; then
  docker cp pqcota-ctl:/work/topology-governance.svg "$DEMO_DIR/topology-governance.svg"
  echo "   → $DEMO_DIR/topology-governance.svg"
fi

echo
echo "✅  확장 완료. 관측 posture(core) 위에 '선언 대비 CONFIRMED/shadow/미관측' + 리뷰 큐가 더해졌다."
echo "   정리: pqcota/demo/scripts/down.sh"
