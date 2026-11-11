#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#  종단 검증 — 자산 스코프 정책 활성화(IC-U152). `make e2e-demo`가 부른다.
#
# pqcota 데모를 처음부터 띄우고, 화면 작업 폴더에서 05를 옛 정책으로 확정한 뒤, 02에서 새 정책을 확정하고,
# ui.sh apply-scope(상류 재적재 → 상류 이력 확인 → 활성화)까지 간다. 실패 경로마다 정책 · 세션 · 계획 · 영수증의
# 바이트와 상류 스냅샷 수가 그대로인지 보고, 성공 뒤에는 세 지문(활성 정책 · 새 세션 · 활성화 영수증)이 같은지, 06이 옛
# 계획을 막는지, 05에 지난 결론이 참고값으로만 보이는지 본다. 마지막으로 v0.25.0 태그로 빌드한 옛 바이너리가 이 작업 폴더의
# 복사본에서 시작하지 않고 아무것도 바꾸지 않는지 본다. 끝나면(실패해도) 화면 · 데모 컨테이너를 내린다.
#
# E2E_KEEP_WS=<폴더>를 주면 정리하기 전에(실패해도) 작업 폴더를 그 폴더로, /work 전체를 <폴더>.work로 꺼내 둔다.
# 성공한 작업 폴더는 옛 판 거절 시험에 쓴다. 성공했는지는 로그의 마지막 ✅ 줄로 가린다.
#
# 전제: PQCOTA_DIR = go.mod 가 고정한 판의 상류 pqcota 체크아웃(옆에 pqcota-common · -inventory · -discovery · -provisioning). docker · go.
#
# **따로 띄운 Docker 데몬 안에서 돈다.** 상류 데모는 컨테이너 · 볼륨 · 네트워크에 고정 이름(pqcota-ctl 등)을 쓴다. 같은
# 호스트에서 pqcota 쪽이 데모를 띄워 쓰고 있으면 이름이 부딪힌다. 그래서 이 스크립트는 먼저 이번 실행만의 dind 컨테이너
# (pqcaton-e2e-<시각>-<pid>)를 띄우고, 상류 데모는 그 안의 데몬에 띄운다. 호스트의 자원은 이름이 같아도 건드리지 않는다.
# 안쪽 데몬은 TCP를 열지 않고, 실행한 사용자만 들어갈 수 있는 임시 폴더(0700)의 유닉스 소켓으로만 받는다. 끝나면(실패해도)
# 그 컨테이너와 익명 볼륨(안쪽 /var/lib/docker)을 지운다. 지울 것의 ID는 `e2e-dind:` 줄로 밝혀 두고, demo/e2e가 그것이
# 남지 않았는지 본다. 안쪽 데몬은 베이스 이미지를 처음부터 받는다(상류 Dockerfile이 다이제스트로 고정한 공식 이미지).
#
# 안쪽에서도 정리는 상류 down.sh를 쓰지 않는다. 이번 실행의 생성 파일로 만든 컨테이너(compose 라벨 config_files)와 그것이
# 쓰는 볼륨 · 네트워크의 ID만 기록해 그것만 지운다.
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(cd "$DEMO_DIR/.." && pwd)"
: "${PQCOTA_DIR:?PQCOTA_DIR must point at the upstream pqcota checkout}"
PQCOTA_DIR="$(cd "$PQCOTA_DIR" && pwd)"
export PQCOTA_DIR

if [ -z "${E2E_INNER_DOCKER:-}" ]; then
  DIND="pqcaton-e2e-$(date +%Y%m%d%H%M%S)-$$"
  SOCK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pqcaton-e2e.XXXXXX")"
  chmod 700 "$SOCK_DIR"
  dind_down() {
    local rc=$?
    if docker inspect "$DIND" >/dev/null 2>&1; then
      docker rm -f -v "$DIND" >/dev/null 2>&1 || true
    fi
    rm -rf "$SOCK_DIR" 2>/dev/null || true
    if docker inspect "$DIND" >/dev/null 2>&1; then
      echo "❌ e2e: the dind container $DIND could not be removed"
      rc=1
    fi
    exit $rc
  }
  trap dind_down EXIT
  # 상류 up.sh는 토폴로지 파일과 생성 폴더(둘 다 PQCOTA_DIR 아래)를 docker run -v로 건넨다. 안쪽 데몬이 같은 경로를
  # 보도록 PQCOTA_DIR을 같은 자리에 마운트한다.
  docker run -d --privileged --name "$DIND" --label pqcaton-e2e=1 -v "$SOCK_DIR:/run/e2e" -v "$PQCOTA_DIR:$PQCOTA_DIR" \
    docker:dind dockerd --host=unix:///run/e2e/docker.sock >/dev/null
  DIND_ID="$(docker inspect "$DIND" --format '{{.Id}}')"
  DIND_VOL="$(docker inspect "$DIND" --format '{{range .Mounts}}{{if eq .Destination "/var/lib/docker"}}{{.Name}}{{end}}{{end}}')"
  echo "e2e-dind: container $DIND_ID volume $DIND_VOL"
  for _ in $(seq 60); do
    docker exec "$DIND" test -S /run/e2e/docker.sock 2>/dev/null && break
    sleep 1
  done
  docker exec "$DIND" chown "$(id -u):$(id -g)" /run/e2e/docker.sock
  # 안쪽 데몬 주소는 안쪽 실행에만 준다. 이 셸의 docker는 끝까지 호스트를 가리켜야 dind_down이 dind를 지운다.
  INNER="unix://$SOCK_DIR/docker.sock"
  for _ in $(seq 60); do DOCKER_HOST="$INNER" docker info >/dev/null 2>&1 && break; sleep 1; done
  DOCKER_HOST="$INNER" docker info >/dev/null 2>&1 || { echo "❌ e2e: the dind daemon did not come up"; exit 1; }
  # **docker exec의 umask를 호스트와 맞춘다.** 호스트에서는 containerd가 systemd 서비스(UMask=0022)로 떠서 docker exec로
  # 띄운 프로세스가 0022를 받는다. dind의 dockerd는 containerd를 직접 데리고 umask 0000으로 띄우므로, 셸 리다이렉트로 만든
  # 파일이 0666이 되고 pqcaton의 파일 권한 검사가 그것을 막는다. 검사를 끄지 않고 기본값만 호스트와 같게 되돌리려고, 안쪽
  # 실행의 PATH 맨 앞에 docker 감싸개를 둔다. exec의 명령 앞에 umask 022만 붙이고 나머지는 그대로 넘긴다.
  # E2E_UBUNTU_MIRROR=<미러 이름>을 주면, 안쪽 빌드가 archive.ubuntu.com · security.ubuntu.com을 그 미러의 주소로 푼다.
  # 안쪽 데몬은 이미지를 처음부터 빌드하므로 기본 미러가 느린 곳에서 쓴다. 감싸개가 이번 실행의 생성 compose 파일
  # (PQCOTA_DIR/demo/.generated, 끝나면 지운다)의 build 항목에 extra_hosts만 더한다. 상류 Dockerfile은 그대로이고 apt의
  # 서명 검증도 그대로다.
  E2E_MIRROR_IP=""
  if [ -n "${E2E_UBUNTU_MIRROR:-}" ]; then
    E2E_MIRROR_IP="$(getent ahostsv4 "$E2E_UBUNTU_MIRROR" | awk 'NR==1{print $1}')"
    [ -n "$E2E_MIRROR_IP" ] || { echo "❌ e2e: cannot resolve E2E_UBUNTU_MIRROR=$E2E_UBUNTU_MIRROR"; exit 1; }
    echo "e2e-mirror: archive.ubuntu.com · security.ubuntu.com -> $E2E_UBUNTU_MIRROR ($E2E_MIRROR_IP) for builds inside dind"
  fi
  mkdir -p "$SOCK_DIR/bin"
  cat > "$SOCK_DIR/bin/docker" <<'WRAP'
#!/usr/bin/env bash
if [ "${1:-}" = exec ]; then
  shift
  opts=()
  while [ $# -gt 0 ] && [ "${1#-}" != "$1" ]; do
    case "$1" in
      -e|--env|-u|--user|-w|--workdir|--env-file|--detach-keys) opts+=("$1" "$2"); shift 2 ;;
      *) opts+=("$1"); shift ;;
    esac
  done
  c="$1"; shift
  exec "$E2E_REAL_DOCKER" exec "${opts[@]}" "$c" sh -c 'umask 022; exec "$@"' sh "$@"
fi
if [ "${1:-}" = compose ] && [ -n "${E2E_MIRROR_IP:-}" ]; then
  f="" prev=""
  for a in "$@"; do [ "$prev" = -f ] && f="$a"; prev="$a"; done
  if [ -n "$f" ] && [ -f "$f" ] && ! grep -q 'extra_hosts' "$f"; then
    sed -i.e2e-bak "s|build: { |build: { extra_hosts: [\"archive.ubuntu.com:$E2E_MIRROR_IP\", \"security.ubuntu.com:$E2E_MIRROR_IP\"], |" "$f"
    rm -f "$f.e2e-bak"
  fi
fi
exec "$E2E_REAL_DOCKER" "$@"
WRAP
  chmod 700 "$SOCK_DIR/bin/docker"
  E2E_REAL_DOCKER="$(command -v docker)" E2E_MIRROR_IP="$E2E_MIRROR_IP" \
    PATH="$SOCK_DIR/bin:$PATH" DOCKER_HOST="$INNER" E2E_INNER_DOCKER=1 bash "${BASH_SOURCE[0]}" "$@"
  exit $?
fi
CTL=pqcota-ctl
WS=/work/ui
ORG=demo-corp
DSN="postgres://postgres:pqcota@pqcota-demo-pg:5432/pqcota"
UI="$DEMO_DIR/scripts/ui.sh"

fail() { echo "❌ e2e: $*"; exit 1; }
step() { echo; echo "■ $*"; }

# 상류 데모의 고정 이름 자원(남아 있으면 이 실행은 시작하지 않는다). 이름에 pqcota만 들어간 무관한 자원은 보지 않는다.
existing() {
  {
    docker ps -a --filter name='^/pqcota-ctl$' --filter name='^/pqcota-demo-pg$' --format 'container {{.Names}}'
    docker ps -a --filter label=pqcota-demo --format 'container {{.Names}}'
    docker volume ls --filter name='^pqcota-demo-topo_' --format 'volume {{.Name}}'
    docker network ls --filter name='^pqcota-topo-' --format 'network {{.Name}}'
  } | sort -u
}
PRE="$(existing)"
if [ -n "$PRE" ]; then
  echo "❌ e2e: demo resources with the fixed names already exist (another run or session?). Nothing was started or removed:"
  echo "$PRE" | sed 's/^/   /'
  exit 1
fi

GEN_CFG="$PQCOTA_DIR/demo/.generated/docker-compose.yml"
# 실행 전의 자원 ID. 정리는 이것에 없던 것 가운데 이번 실행의 것임을 확인한 것만 지운다.
ids() { docker ps -aq --no-trunc | sort; docker volume ls -q | sed 's/^/v:/' | sort; docker network ls -q --no-trunc | sed 's/^/n:/' | sort; }
BEFORE="$(mktemp)"
ids > "$BEFORE"
# cleanup: up.sh가 끝까지 돌지 못하고 일부만 만들었어도 그것만 지운다.
#   - 컨테이너: 이번 실행의 생성 파일 라벨(config_files)이 붙고 실행 전에는 없던 것.
#   - 볼륨 · 네트워크: 실행 전에는 없었고, 위 컨테이너가 실제로 쓰는 것.
#   실행 전에 없던 자원 가운데 소유를 확인하지 못한 것은 남기고 실패로 알린다.
cleanup() {
  local rc=$? own="" vols="" nets="" c left
  for c in $(docker ps -aq --no-trunc --filter "label=com.docker.compose.project.config_files=$GEN_CFG"); do
    grep -qx "$c" "$BEFORE" && continue
    own="$own $c"
    vols="$vols $(docker inspect "$c" --format '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}} {{end}}{{end}}')"
    nets="$nets $(docker inspect "$c" --format '{{range $k, $v := .NetworkSettings.Networks}}{{$v.NetworkID}} {{end}}')"
  done
  if [ -n "$own" ]; then
    # 지우기 전에 작업 폴더를 꺼낸다(실패해도). 성공했는지는 로그의 마지막 ✅ 줄로 가린다.
    if [ -n "${E2E_KEEP_WS:-}" ]; then
      mkdir -p "$E2E_KEEP_WS"
      docker cp "$CTL:$WS/." "$E2E_KEEP_WS/" >/dev/null 2>&1 && echo "   workspace kept at $E2E_KEEP_WS (exit $rc)"
      docker cp "$CTL:/work/." "$E2E_KEEP_WS.work/" >/dev/null 2>&1 || true
    fi
    "$UI" stop >/dev/null 2>&1 || true
    docker rm -f $own >/dev/null 2>&1 || true
  fi
  for c in $vols; do grep -qx "v:$c" "$BEFORE" || docker volume rm "$c" >/dev/null 2>&1 || true; done
  for c in $nets; do grep -qx "n:$c" "$BEFORE" || docker network rm "$c" >/dev/null 2>&1 || true; done
  rm -rf "$PQCOTA_DIR/demo/.generated"
  # 실행 전에 없던 데모 이름의 자원이 아직 있으면 소유를 확인하지 못한 것이다. 지우지 않고 알린다.
  left="$(existing)"
  rm -f "$BEFORE"
  if [ -n "$left" ]; then
    echo "❌ e2e: these demo resources were not removed (their owner could not be confirmed; remove them by hand if they are yours):"
    echo "$left" | sed 's/^/   /'
    rc=1
  fi
  exit $rc
}
# **up.sh보다 먼저 건다.** up.sh가 중간에 멈춰도 만든 것만 정리한다.
trap cleanup EXIT

# 정책 · 세션 · 계획 · 후보 · 영수증의 바이트(파일 해시와 영수증 목록).
snap() {
  docker exec "$CTL" bash -c "cd $WS && sha256sum scope-assets.csv session.json plan.json asset-scope.csv 2>/dev/null;
    find .pqcaton/receipts -type f 2>/dev/null | sort | xargs -r sha256sum"
}
# 상류 스냅샷 수.
count() { docker exec pqcota-demo-pg psql -U postgres -d pqcota -tAc 'select count(*) from pqcota_snapshots' | tr -d '[:space:]'; }
# 작업 폴더의 JSON 하나를 python으로 고친다(컨테이너 안).
edit_json() { docker exec -i "$CTL" python3 - "$1"; }
# 바뀌지 않았어야 하는 실패 경로를 견준다.
unchanged() {
  [ "$(snap)" = "$1" ] || fail "$3: a file or receipt changed"
  [ "$(count)" = "$2" ] || fail "$3: the upstream history changed"
  echo "   unchanged ($3): files $(sha256sum <<<"$1" | cut -c1-16) · upstream snapshots $2"
}

step "1) pqcota demo (up · demo) and the pqcaton extension"
(cd "$PQCOTA_DIR" && ./demo/scripts/up.sh) >/dev/null
(cd "$PQCOTA_DIR" && ./demo/scripts/demo.sh) >/dev/null
"$DEMO_DIR/scripts/extend.sh" >/dev/null
"$UI" start
"$UI" stop

step "2) 05 under the old policy: judge and finalize from the command line"
edit_json "$WS/session.json" <<'PY'
import json, sys
p = sys.argv[1]; s = json.load(open(p))
for k in s.get("policy_decisions", {}):
    s["policy_decisions"][k] = "e2e: replace"
s["reviewer"], s["signature"] = "e2e", "e2e-signature"
json.dump(s, open(p, "w"), ensure_ascii=False, indent=2)
PY
docker exec "$CTL" pqcaton-decide close "$WS/session.json" -o "$WS/plan.json" -scope-assets "$WS/scope-assets.csv" -org "$ORG" >/dev/null
docker exec "$CTL" test -f "$WS/plan.json" || fail "05 did not write plan.json"

# 새 정책은 **어느 노드에서도 처음 보는 스냅샷**을 내야 한다. 두 가지를 피한다.
#   - 아무것도 바꾸지 못하는 규칙: 스냅샷 지문이 옛 정책과 같아, 다시 적재하지 않아도 확인이 통과한다(4c가 통과해 버린다).
#   - 예전 상태로 되돌리는 규칙: 상류 저장소는 같은 지문의 스냅샷이 이력에 있으면 새로 만들지 않고, 노드의 최신(seq가 가장
#     큰 것)은 옛 정책 스냅샷으로 남는다. 다시 적재해도 확인이 거절한다(이전 정책 복귀는 -confirm-ingest만 쓴다).
# 그리고 **한 노드의 관리 자산을 0개로 만든다.** 관리 자산이 없는 노드도 확인 대상인지(4c)를 보려는 것이다. 옛 정책 뒤에
# JCA 제외를 더하면 pay-app은 관리 자산 0개 · 2건 제외라는 처음 보는 상태가 되고, pay-db(05의 libssl 정책)와 web-gw는
# 그대로다.
step "3) 02: finalize a new policy (the old rules, then drop JCA: pay-app is left with no managed assets)"
TMPB="$(mktemp -d)"
( cd "$REPO_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH="$(docker exec "$CTL" uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" \
    go build -o "$TMPB/pqcaton-scope" ./inventory/cmd/pqcaton-scope )
docker cp -q "$TMPB/pqcaton-scope" "$CTL:/usr/local/bin/pqcaton-scope"
rm -rf "$TMPB"
docker exec "$CTL" bash -c "mkdir -p $WS/layers && { cat $WS/scope-assets.csv; echo 'exclude,jca,*,*,e2e: JCA is managed elsewhere'; } > $WS/layers/demo.csv"
docker exec "$CTL" bash -c "pqcaton-scope open $WS/layers/demo.csv -base $WS/scope-assets.csv -org $ORG > $WS/scope-session.json"
edit_json "$WS/scope-session.json" <<'PY'
import json, sys
p = sys.argv[1]; s = json.load(open(p))
s["layer_decisions"] = {c["layer"]: "e2e: approved" for c in s.get("changes", [])}
s["reviewer"], s["signature"] = "e2e", "e2e-signature"
json.dump(s, open(p, "w"), ensure_ascii=False, indent=2)
PY
docker exec "$CTL" pqcaton-scope close "$WS/scope-session.json" -o "$WS/asset-scope.csv" -org "$ORG" -state-dir "$WS/.pqcaton" >/dev/null
OLD_ID="$(docker exec "$CTL" pqcaton-decide scope-status "$WS/session.json" -scope-assets "$WS/scope-assets.csv" | awk '$1=="session_id"{print $2}')"
OLD_DIGEST="$(docker exec "$CTL" pqcaton-decide scope-status "$WS/session.json" -scope-assets "$WS/scope-assets.csv" | awk '$1=="active"{print $2}')"
[ -n "$OLD_ID" ] && [ -n "$OLD_DIGEST" ] || fail "no session id or active digest before activation"

step "4a) apply-scope while the screen runs: refused before the upstream ingest"
"$UI" start
B="$(snap)"; C="$(count)"
if "$UI" apply-scope; then fail "apply-scope ran while the screen was running"; fi
unchanged "$B" "$C" "screen running"
"$UI" stop

step "4b) apply-scope with a hand-edited candidate: refused before the upstream ingest"
docker exec "$CTL" cp "$WS/asset-scope.csv" /tmp/e2e-candidate.bak
docker exec "$CTL" bash -c "echo 'exclude,*,*,/opt/*,hand edit' >> $WS/asset-scope.csv"
B="$(snap)"; C="$(count)"
if "$UI" apply-scope; then fail "apply-scope activated a hand-edited candidate"; fi
unchanged "$B" "$C" "hand-edited candidate"
docker exec "$CTL" bash -c "cp /tmp/e2e-candidate.bak $WS/asset-scope.csv && rm /tmp/e2e-candidate.bak"

step "4c) verify upstream without re-ingesting: pay-app has no managed assets now, and its latest snapshot is still the old one, so refused"
B="$(snap)"; C="$(count)"
if docker exec -e PQCOTA_DSN="$DSN" "$CTL" pqcaton-decide activate-scope "$WS/session.json" -decl "$WS/declaration.json" \
    -results "$WS/results" -scope-assets "$WS/scope-assets.csv" -candidate "$WS/asset-scope.csv" \
    -scope-session "$WS/scope-session.json" -org "$ORG" -verify-upstream -upstream-org default; then
  fail "activation passed although the upstream was not re-ingested"
fi
unchanged "$B" "$C" "upstream not re-ingested"

step "5) apply-scope: re-ingest, verify, activate"
"$UI" apply-scope
STATUS="$(docker exec "$CTL" pqcaton-decide scope-status "$WS/session.json" -scope-assets "$WS/scope-assets.csv")"
ACTIVE="$(awk '$1=="active"{print $2}' <<<"$STATUS")"
SESSION="$(awk '$1=="session"{print $2}' <<<"$STATUS")"
LOCK="$(awk '$1=="lock"{print $2}' <<<"$STATUS")"
[ "$ACTIVE" != "$OLD_DIGEST" ] || fail "the active policy did not change"
[ "$ACTIVE" = "$SESSION" ] && [ "$LOCK" = "none" ] || fail "active $ACTIVE, session $SESSION, lock $LOCK"
RECEIPT="$(docker exec "$CTL" cat "$WS/.pqcaton/receipts/scope-activation/$ORG/$OLD_ID.json")"
RECEIPT_JSON="$RECEIPT" python3 - "$ACTIVE" "$OLD_DIGEST" "$SESSION" <<'PY' || fail "the activation receipt does not match"
import json, os, sys
r = json.loads(os.environ["RECEIPT_JSON"])
assert r["format"] == 2 and r["scope_policy"] == sys.argv[1], r
assert r["round"]["from_policy"] == sys.argv[2] and r["round"]["basis"] == "upstream-verified", r
print("   active policy     %s" % sys.argv[1])
print("   new session       %s" % sys.argv[3])
print("   receipt policy    %s (format %d)" % (r["scope_policy"], r["format"]))
print("   receipt from      %s" % r["round"]["from_policy"])
print("   receipt basis     %s" % r["round"]["basis"])
PY

step "6) the screen: 06 refuses the plan made under the old policy; 05 shows earlier conclusions only as references"
"$UI" start
# 데모의 수집기는 replace 빌드라 판을 확인할 수 없다. 03이 그것을 파일마다 보이고, -allow-unverified-collectors 로 허용한
# 상태가 화면과 로그에 남는지 본다. collector.jar 는 판을 싣지 않아 경고만 보인다.
OBSERVE="$(docker exec "$CTL" curl -fsS 'http://127.0.0.1:8765/observe?lang=en')"
grep -q 'data-check="collector-allowed"' <<<"$OBSERVE" || fail "03 does not show that unverified collectors are allowed"
LINES="$(grep -oE 'data-collector-state="[a-z]+"><code>[^<]*</code>[^<]*' <<<"$OBSERVE" | sed -E 's/^data-collector-state="([a-z]+)"><code>([^<]*)<\/code>/\1 \2/')"
echo "$LINES" | sed 's/^/   03 collector: /'
grep -q '^unverified ' <<<"$LINES" || fail "03 does not list the unverified collectors"
if grep -qE '^(verified|mismatch) ' <<<"$LINES"; then fail "a demo collector is not 'cannot be verified' (the demo builds them with local replaces)"; fi
if grep -q "the version this screen pins is unknown" <<<"$LINES"; then fail "03 does not know the upstream version this screen pins"; fi
if docker exec "$CTL" test -f "$WS/collectors/collector.jar"; then
  grep -q 'data-check="jar-version"' <<<"$OBSERVE" || fail "03 does not warn that collector.jar carries no version"
  JAR_NOTE="collector.jar has no version (warning only)"
else
  JAR_NOTE="no collector.jar in this demo"
fi
grep -q 'action="/observe/run"' <<<"$OBSERVE" || fail "03 does not offer to start although the allowance is given"
docker exec "$CTL" grep -q -- "-allow-unverified-collectors lets these collectors run" /work/ui.log || fail "the allowance is not in the screen log"
# 화면이 고정한 판: go.mod 가 정한 상류 판이어야 한다. 이 판은 화면이 요구하는 판일 뿐, 수집기의 실제 판을 뜻하지 않는다.
PIN="$(awk '$1=="github.com/randyinthedev-hash/pqcota-common"{print $2}' "$REPO_DIR/go.mod")"
[ -n "$PIN" ] || fail "cannot read the pinned upstream version from go.mod"
grep -o 'data-check="collector-allowed".\{0,300\}' <<<"$OBSERVE" | grep -q "<code>$PIN</code>" || fail "03 does not show the pinned version $PIN"
docker exec "$CTL" grep -q -- "(pinned $PIN)" /work/ui.log || fail "the screen log does not name the pinned version $PIN"
echo "   03: pinned $PIN; every collector cannot be verified, allowed by -allow-unverified-collectors; $JAR_NOTE"
APPROVE="$(docker exec "$CTL" curl -fsS 'http://127.0.0.1:8765/approve?lang=en')"
grep -q "was made under an asset-scope policy other than the active one" <<<"$APPROVE" || fail "06 does not say the plan is under another policy"
if grep -q 'action="/approve/sign/confirm"' <<<"$APPROVE"; then fail "06 offers to sign the old plan"; fi
REVIEW="$(docker exec "$CTL" curl -fsS 'http://127.0.0.1:8765/review?lang=en')"
grep -q "Earlier policy conclusion (from the previous round; a reference value)" <<<"$REVIEW" || fail "05 does not show the earlier conclusions as references"
grep -q "were found as the latest snapshot of every node in the upstream history" <<<"$REVIEW" || fail "05 does not show the upstream-verified basis of this round"
"$UI" stop

# 지금 활성 정책으로 만든 계획은 06에서 서명되고 플레이북이 만들어져야 한다. 상류 v0.10.7의 승인 정규형 검사
# (sign.CheckPlanText)를 지난 계획이 실제로 서명 · 생성까지 가는지를 화면 요청으로 잰다. 새 회차를 데모 확장과 같은 방식으로
# 판정해 확정하고, 06 화면의 서명 양식 → 확인 페이지 → 서명, 생성 양식 → 생성을 화면이 보내는 그대로 보낸다.
step "6b) 05 → 06 under the new policy: finalize the new round, then sign and generate through the screen"
edit_json "$WS/session.json" <<'PY'
import json, sys
p = sys.argv[1]; s = json.load(open(p))
s["reviewer"], s["signature"] = "e2e", "e2e-signature"
for k in s.get("policy_decisions", {}):
    s["policy_decisions"][k] = "e2e: replace"
def pick(it):
    it["include_in_plan"] = True
    it["remediation_kind"] = "REMEDIATION_KIND_CONFIG_ONLY"
    it["target_algorithm"] = "ML-KEM (FIPS 203)"
    it["deploy_level"] = "L2"
    it["rollback_note"] = "remove the staged config fragment"
    ins = sorted({e.get("path") or "finding:" + e["finding_id"] for e in it.get("evidence_sources") or []})
    if len(ins) > 1:
        it["installations_confirmed"] = ins
for it in s.get("items", []):
    if it["state"] in ("CONFIRMED", "UNDECLARED") and it.get("managed") != "EXCLUDED_BY_POLICY":
        pick(it)
for it in s.get("autopass_candidates") or []:
    if isinstance(it, dict):
        pick(it)
json.dump(s, open(p, "w"), ensure_ascii=False, indent=2)
PY
docker exec "$CTL" pqcaton-decide close "$WS/session.json" -o "$WS/plan.json" -scope-assets "$WS/scope-assets.csv" -org "$ORG" >/dev/null
N_ACTIONS="$(docker exec "$CTL" python3 -c "import json; print(len(json.load(open('$WS/plan.json')).get('actions', [])))")"
[ "$N_ACTIONS" -gt 0 ] || fail "the new round's plan has no actions to sign"
"$UI" start
docker exec -i "$CTL" python3 - <<'PY' || fail "06 did not sign and generate the plan made under the active policy"
import html, re, sys, urllib.parse, urllib.request, http.cookiejar
base = "http://127.0.0.1:8765"
op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
def get(path):
    return op.open(base + path).read().decode()
def post(path, data):
    req = urllib.request.Request(base + path, data=urllib.parse.urlencode(data, doseq=True).encode(),
        headers={"Origin": base, "Content-Type": "application/x-www-form-urlencoded"})
    r = op.open(req)
    return r.status, r.read().decode()
def form(page, action):
    m = re.search(r'<form[^>]*action="' + re.escape(action) + r'"[^>]*>(.*?)</form>', page, re.S)
    if not m:
        sys.exit("no form posting to " + action)
    fields = {}
    for tag in re.findall(r"<input[^>]*>", m.group(1)):
        name = re.search(r'name="([^"]*)"', tag)
        if not name:
            continue
        value = re.search(r'value="([^"]*)"', tag)
        if 'type="hidden"' in tag or 'type="text"' in tag:
            fields[name.group(1)] = html.unescape(value.group(1)) if value else ""
        elif 'type="checkbox"' in tag:
            fields[name.group(1)] = "on"
    return fields
page = get("/approve?lang=en")
f = form(page, "/approve/sign/confirm")
f["approver"] = "e2e-approver"
st, confirm = post("/approve/sign/confirm?lang=en", f)
st, _ = post("/approve/sign?lang=en", form(confirm, "/approve/sign"))
page = get("/approve?lang=en")
g = form(page, "/approve/generate")
g["level"] = "l2"
st, _ = post("/approve/generate?lang=en", g)
print("   06: signed and generated through the screen")
PY
# 데모는 -approval-key 로 키를 명시한다. 명시하면 기본 위치(사용자 설정 폴더)는 읽지 않으므로 화면 로그에 기본 위치 줄이 없어야
# 한다. 개인 키 내용은 06 화면에도, 서명한 계획에도, 화면 로그에도 없어야 한다(키 경로와 지문만 보인다).
if docker exec "$CTL" grep -q "default location" /work/ui.log; then fail "the screen looked at the default key location although -approval-key was given"; fi
PRIV="$(docker exec "$CTL" sed -n 's/^PQCOTA_SIGN_KEY=//p' /work/ui-approval.key)"
[ -n "$PRIV" ] || fail "cannot read the demo approval key"
APPROVE_SIGNED="$(docker exec "$CTL" curl -fsS 'http://127.0.0.1:8765/approve?lang=en')"
grep -q "The approval key given at start is used." <<<"$APPROVE_SIGNED" || fail "06 does not say the explicit approval key is used"
if grep -qF -- "$PRIV" <<<"$APPROVE_SIGNED"; then fail "the private key is on the 06 screen"; fi
if docker exec "$CTL" grep -qF -- "$PRIV" /work/ui.log "$WS/plan.signed.json"; then fail "the private key is in the screen log or the signed plan"; fi
echo "   06: explicit -approval-key used, no default-location lookup, the private key appears nowhere"
"$UI" stop
SIGNED_STATUS="$(docker exec "$CTL" python3 -c "import json; p=json.load(open('$WS/plan.signed.json')); print(p['status'], len(p.get('approvalSignatures', [])))")"
[ "$SIGNED_STATUS" = "PLAN_STATUS_FINALIZED 1" ] || fail "the signed plan is $SIGNED_STATUS"
docker exec "$CTL" bash -c "test -s $WS/plan.provision.yml && test -s $WS/plan.rollback.yml" || fail "06 wrote no playbooks"
echo "   signed plan: $SIGNED_STATUS · actions $N_ACTIONS · playbooks written"

# 옛 판은 형식 2 영수증을 읽기 전까지 멈추지 않아 활성화 전 계획에 서명 양식까지 낸다. 그래서 이 판은 형식 2 영수증을 쓰기
# 직전에 상태 문서를 형식 2로 올린다. 그것이 실제로 옛 바이너리를 막는지, 이 저장소의 v0.25.0 태그로 빌드한 바이너리를
# 성공한 작업 폴더의 복사본에 돌려 본다: 화면은 시작하지 않고(듣는 포트가 없으니 05 · 06의 어떤 양식도 없다), 상태를 쓰는
# 명령도 멈추며, 파일 · 영수증 · 트랜잭션 기록 · 준비 파일이 하나도 바뀌거나 늘지 않는다.
step "7) the old version (v0.25.0, built from its tag) on a copy of this workspace: does not start, changes nothing"
git -C "$REPO_DIR" rev-parse -q --verify 'refs/tags/v0.25.0^{commit}' >/dev/null || fail "the tag v0.25.0 is not in $REPO_DIR (git fetch --tags)"
OLDB="$(mktemp -d)"
git -C "$REPO_DIR" archive v0.25.0 | tar -x -C "$OLDB"
OLD_ARCH="$(docker exec "$CTL" uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
( cd "$OLDB" && CGO_ENABLED=0 GOOS=linux GOARCH="$OLD_ARCH" go build -o "$OLDB/bin/pqcaton-ui" ./inventory/cmd/pqcaton-ui \
    && CGO_ENABLED=0 GOOS=linux GOARCH="$OLD_ARCH" go build -o "$OLDB/bin/pqcaton-decide" ./inventory/cmd/pqcaton-decide )
docker cp -q "$OLDB/bin/pqcaton-ui" "$CTL:/usr/local/bin/pqcaton-ui-v025"
docker cp -q "$OLDB/bin/pqcaton-decide" "$CTL:/usr/local/bin/pqcaton-decide-v025"
rm -rf "$OLDB"
OLDWS=/work/ui-v025
docker exec "$CTL" bash -c "rm -rf $OLDWS && cp -a $WS $OLDWS"
FMT="$(docker exec "$CTL" python3 -c "import json; print(json.load(open('$OLDWS/.pqcaton/state.json'))['format'])")"
[ "$FMT" = 2 ] || fail "the state document is format $FMT after the activation, want 2"
old_measure() {
  docker exec "$CTL" bash -c "cd $OLDWS && echo tree \$({ find . -type f -print0 | sort -z | xargs -0 sha256sum; find . \( -type f -o -type d \) -printf '%p %m %s\n' | sort; } | sha256sum | cut -c1-16) \
    files \$(find . -type f | wc -l) receipts \$(find .pqcaton/receipts -type f | wc -l) txn \$(find .pqcaton/txn -type f 2>/dev/null | wc -l) \
    prepared \$(find . -name '*.pqcaton-*.tmp' | wc -l)"
}
B="$(old_measure)"
set +e
OUT="$(docker exec "$CTL" timeout 20 pqcaton-ui-v025 -workspace "$OLDWS" -no-browser -addr 127.0.0.1:8799 2>&1)"; RC=$?
set -e
[ "$RC" != 0 ] && [ "$RC" != 124 ] || fail "v0.25.0 pqcaton-ui did not refuse to start (exit $RC): $OUT"
grep -q "damaged or from another version" <<<"$OUT" || fail "v0.25.0 pqcaton-ui refused for another reason: $OUT"
if docker exec "$CTL" curl -fsS -o /dev/null http://127.0.0.1:8799/approve 2>/dev/null; then fail "something answers on the v0.25.0 screen port"; fi
echo "   v0.25.0 pqcaton-ui: exit $RC, $(grep -m1 -o 'state.json is damaged or from another version' <<<"$OUT")"
set +e
OUT="$(docker exec "$CTL" bash -c "a=\$(ls $OLDWS/session.scope-activation-*.json | grep -v report); timeout 20 pqcaton-decide-v025 close \$a -o $OLDWS/plan.json -org $ORG -state-dir $OLDWS/.pqcaton" 2>&1)"; RC=$?
set -e
[ "$RC" != 0 ] && grep -q "damaged or from another version" <<<"$OUT" || fail "v0.25.0 pqcaton-decide close did not stop at the state document (exit $RC): $OUT"
echo "   v0.25.0 pqcaton-decide close: exit $RC"
A="$(old_measure)"
[ "$A" = "$B" ] || fail "the old version changed the workspace copy: before $B, after $A"
echo "   unchanged (v0.25.0): $A"
docker exec "$CTL" rm -rf "$OLDWS" /usr/local/bin/pqcaton-ui-v025 /usr/local/bin/pqcaton-decide-v025

echo
echo "✅ e2e: policy activation passed (old $OLD_DIGEST → new $ACTIVE)"
