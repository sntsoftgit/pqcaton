#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
#  화면 — 실행 중인 pqcota 데모의 pqcota-ctl 안에서 pqcaton-ui를 띄우고, 이 기계의 브라우저로 본다.
#
# **실제 배치와 같은 모양으로 띄운다.** 화면은 Ansible이 노드에 닿는 기계에서 돈다. 03 관측이
# ansible-playbook을 그 기계에서 부르기 때문이다. 데모에서 그 기계는 pqcota-ctl이다. 운영자는 자기
# 기계에서 SSH 터널(ssh -L)로 본다(운영자 안내서 5절). 데모의 pqcota-ctl에는 sshd가 없으므로 그 터널
# 자리를 docker exec 중계가 맡는다.
#
# **포트를 공개하지 않는다.** 화면에는 로그인이 없어 루프백에서만 연결을 받는다. 컨테이너의 인터페이스에
# 중계를 두면 같은 Docker 망의 다른 컨테이너(데모 노드)도 화면에 바로 닿는다. Host 검사는 브라우저를
# 거친 공격만 막는다. 이 중계는 이 기계의 127.0.0.1에서만 받고, 연결마다 docker exec로 컨테이너 안의
# 127.0.0.1:8765에 잇는다. 컨테이너 밖에서 연결을 받는 자리는 하나도 생기지 않는다.
#
# 전제: pqcota/demo/scripts/{up,demo}.sh와 ./demo/scripts/extend.sh가 끝난 상태(선언은 extend.sh 2/8이 만든다).
#       이 기계에 docker · python3, 그리고 Go. Go가 없으면 linux용 pqcaton-ui와 이 기계용 collectors를 PQCATON_BIN_DIR로 준다.
#
# **수집기는 상류 공식 릴리스 자산만 쓴다.** 먼저 준비 도구로 받아 확인해 둔다(태그는 go.mod가 고정한 판이고, 체크섬 ·
# release manifest · 출처 증명을 모두 확인한다. gh 로그인이 필요하다). 대상은 pqcota-ctl 컨테이너의 OS · 아키텍처로 명시한다
# (보통 linux-amd64, Apple Silicon의 Docker는 linux-arm64):
#   go run ./tools/collectors fetch -arch linux-amd64 -out ~/.cache/pqcaton-demo/collectors-<판>
# 이 스크립트는 아무것도 내려받지 않는다. 화면을 띄울 때마다 그 폴더를 다시 확인하고(파일마다 해시 · 판 · 커밋 · 출처 증명),
# 통과해야 작업 폴더의 collectors로 바꿔 넣는다. 확인하지 못하면 아무것도 바꾸지 않고 멈춘다. 다른 길(상류 소스 빌드 ·
# -allow-unverified-collectors)은 없다. 폴더는 PQCATON_COLLECTORS로 바꿀 수 있다.
#
# 쓰는 법:
#   ./demo/scripts/ui.sh         화면을 띄우고(이미 떠 있으면 그대로 쓴다) 중계를 연다. Ctrl+C는 중계만 닫는다.
#   ./demo/scripts/ui.sh stop    컨테이너 안의 화면을 끝낸다. 다시 빌드한 화면을 띄우려면 stop 뒤에 다시 연다.
#   ./demo/scripts/ui.sh start   화면만 띄우고 중계는 열지 않는다(종단 검증이 쓴다).
#   ./demo/scripts/ui.sh apply-scope
#                                02에서 확정한 정책으로 pqcota를 다시 적재한다. **활성화는 하지 않는다.** 활성화는 화면(05)이
#                                pqcota 이력 대조를 거쳐 한다. 화면이 떠 있어도 된다(적재는 작업 폴더를 건드리지 않는다).
#   PQCATON_UI_PORT=8766 ...     이 기계에서 받을 포트(기본 8765). 이 기계에서 다른 화면을 이미 띄웠을 때 바꾼다.
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"   # pqcaton/demo
REPO_DIR="$(cd "$DEMO_DIR/.." && pwd)"                        # pqcaton

CTL=pqcota-ctl
# 작업 폴더(컨테이너 안). 처음 한 번만 채우고, 그 뒤로는 손대지 않는다. 화면에서 한 일이 거기 남는다.
WS=/work/ui
# 06의 승인 서명 키. 작업 폴더 밖에 0600으로 둔다(운영자 안내서 2절과 같은 자리 규칙).
KEY=/work/ui-approval.key
# pqcota 이력(데모 DB). 화면은 이것 없이 시작하지 않는다. pqcota 이력의 조직은 판정 조직(demo-corp)과 같아야 하므로, 재적재도
# 그 조직으로 한다(PQCOTA_ORG). pqcota 데모의 demo.sh가 기본 조직에 적재한 이력은 이 화면이 보지 않는다.
DSN="postgres://postgres:pqcota@pqcota-demo-pg:5432/pqcota"
ORG=demo-corp
LOG=/work/ui.log
PORT="${PQCATON_UI_PORT:-8765}"
# 수집기: go.mod가 고정한 상류 판의 공식 릴리스 자산을 준비 도구(tools/collectors fetch)로 받아 둔 폴더.
PIN="$(awk '$1=="github.com/randyinthedev-hash/pqcota-discovery"{print $2}' "$REPO_DIR/go.mod")"
COLLECTORS="${PQCATON_COLLECTORS:-$HOME/.cache/pqcaton-demo/collectors-$PIN}"

docker inspect "$CTL" >/dev/null 2>&1 || { echo "❌ no $CTL — run pqcota/demo/scripts/{up,demo}.sh first"; exit 1; }

if [ "${1:-}" = "stop" ]; then
  if docker exec "$CTL" pgrep -x pqcaton-ui >/dev/null; then
    # SIGTERM은 화면이 받아 정리하고 끝낸다. 관측을 돌리는 중이면 감독이 Ansible을 끝내고 노드를 정리한다.
    docker exec "$CTL" pkill -TERM -x pqcaton-ui
    echo "✅ stopped pqcaton-ui in $CTL (workspace $WS is kept)"
  else
    echo "   pqcaton-ui is not running in $CTL"
  fi
  exit 0
fi
MODE="${1:-}"
case "$MODE" in
  ""|start|apply-scope) ;;
  *) echo "usage: $0 [stop|start|apply-scope]"; exit 2 ;;
esac
if [ "$MODE" = "" ]; then
  command -v python3 >/dev/null || { echo "❌ python3 not found on this machine — the relay needs it"; exit 1; }
fi
docker exec "$CTL" test -f /work/declaration.json || { echo "❌ no /work/declaration.json — run ./demo/scripts/extend.sh first"; exit 1; }

# install_bin NAME PKG: linux용으로 빌드해(또는 PQCATON_BIN_DIR에서 가져와) 컨테이너에 넣고, 거기서 실제로 도는지 본다.
# extend.sh 1/8과 같은 이유로 컨테이너용으로 빌드한다. 아키텍처는 컨테이너에 묻는다.
install_bin() {
  local name="$1" pkg="$2" tmp arch
  tmp="$(mktemp -d)"
  if [ -n "${PQCATON_BIN_DIR:-}" ]; then
    echo "   using the prebuilt $name from $PQCATON_BIN_DIR"
    cp "$PQCATON_BIN_DIR/$name" "$tmp/" || { rm -rf "$tmp"; return 1; }
  else
    case "$(docker exec "$CTL" uname -m)" in
      x86_64|amd64)  arch=amd64 ;;
      aarch64|arm64) arch=arm64 ;;
      *) echo "❌ unsupported $CTL architecture: $(docker exec "$CTL" uname -m)"; rm -rf "$tmp"; return 1 ;;
    esac
    echo "   target: linux/$arch ($CTL) — $name"
    ( cd "$REPO_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -o "$tmp/$name" "$pkg" ) || { rm -rf "$tmp"; return 1; }
  fi
  docker cp -q "$tmp/$name" "$CTL:/usr/local/bin/$name"
  rm -rf "$tmp"
  local rc=0
  docker exec "$CTL" bash -c "/usr/local/bin/$name -h >/dev/null 2>&1" || rc=$?
  if [ "$rc" -eq 126 ] || [ "$rc" -eq 127 ] || [ "$rc" -eq 255 ]; then
    echo "❌ $name does not run inside $CTL (exit $rc) — it must be a linux/$(docker exec "$CTL" uname -m) binary"
    return 1
  fi
}

# container_target: 화면과 Ansible이 도는 컨테이너의 OS · 아키텍처(준비 도구의 대상 이름, 예: linux-arm64).
container_target() {
  local os arch
  os="$(docker exec "$CTL" uname -s | tr '[:upper:]' '[:lower:]')"
  case "$(docker exec "$CTL" uname -m)" in
    x86_64|amd64)  arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) arch="$(docker exec "$CTL" uname -m)" ;;
  esac
  echo "$os-$arch"
}

# verify_collectors: 준비한 수집기 폴더를 쓰기 직전에 다시 확인한다(「준비 완료」 표시를 믿지 않는다). 받지 않는다.
# 먼저 준비 폴더의 대상(provenance/TARGET)이 컨테이너의 OS · 아키텍처와 같은지 본다. 다르면 「exec format error」가 나기
# 전에 거절한다(Apple Silicon의 Docker는 linux/arm64다).
verify_collectors() {
  [ -n "$PIN" ] || { echo "❌ cannot read the pinned upstream version from go.mod"; return 1; }
  TARGET="$(container_target)"
  if [ ! -d "$COLLECTORS" ]; then
    echo "❌ no prepared collectors at $COLLECTORS"
    echo "   fetch the official $PIN release for $CTL ($TARGET) first:"
    echo "   (cd $REPO_DIR && go run ./tools/collectors fetch -arch $TARGET -out $COLLECTORS)"
    return 1
  fi
  local prepared
  prepared="$(cat "$COLLECTORS/provenance/TARGET" 2>/dev/null || true)"
  echo "   collectors: prepared for ${prepared:-?} · $CTL is ${TARGET/-//}"
  if [ "$prepared" != "${TARGET/-//}" ]; then
    echo "❌ the prepared collectors are for ${prepared:-an unknown target}, but $CTL is ${TARGET/-//}; fetch them with -arch $TARGET"
    return 1
  fi
  if [ -n "${PQCATON_BIN_DIR:-}" ] && [ -x "$PQCATON_BIN_DIR/collectors" ]; then
    "$PQCATON_BIN_DIR/collectors" verify -arch "$TARGET" -dir "$COLLECTORS" -tag "$PIN"
  else
    ( cd "$REPO_DIR" && go run ./tools/collectors verify -arch "$TARGET" -dir "$COLLECTORS" -tag "$PIN" )
  fi
}

# sync_collectors: 확인한 수집기로 작업 폴더의 collectors를 바꿔 넣는다(화면이 돌지 않을 때만 부른다). 반쯤 바뀌지 않게 한다.
# 파일 연산은 컨테이너 안의 pqcaton-collectors install(tools/collectors/install.go)이 화면과 같은 작업 폴더 실행 잠금을 쥐고 한다:
# 작업 폴더 밖 source를 잠금 안에서 다시 확인하고, 지난 실행이 끊긴 자리를 정리하고, 작업 폴더 안의 고유 복사본으로 복사해 해시
# 목록과 다시 견준 뒤, 기존 것을 보관 자리로 옮기고 rename으로 들인다. 화면이 떠 있거나 다른 설치가 돌고 있으면 잠금을 얻지 못해
# 아무것도 바꾸지 않는다. 어디서 실패해도 기존 collectors는 바이트까지 그대로이거나 되돌려진다(IC-CF8 ~ IC-CF14).
# prepared_sums: 확인한 준비 폴더의 collectors 해시 목록(sha256sum 꼴).
prepared_sums() { ( cd "$COLLECTORS/collectors" && find . -type f | LC_ALL=C sort | while read -r f; do sha256sum "$f"; done ); }

sync_collectors() {
  # 확인한 수집기를 컨테이너의 작업 폴더 **밖** 임시 자리에 둔다. 작업 폴더 안의 복사 · 검증 · 설치는 도구가 실행 잠금 아래에서
  # 한 번에 한다. 이 호출이 만든 임시 자리는 성공하든 실패하든 지운다(다른 실행의 것은 건드리지 않는다).
  local src
  src="$(docker exec "$CTL" mktemp -d /tmp/pqcaton-collectors-XXXXXXXXXX)" || { echo "❌ could not make a temporary place in $CTL"; return 1; }
  local rc=0
  if ! docker cp -q "$COLLECTORS/collectors" "$CTL:$src/"; then
    echo "❌ could not copy the collectors into $CTL; the workspace's collectors are unchanged"
    rc=1
  else
    prepared_sums | docker exec -i "$CTL" bash -c "umask 077 && cat > $src/collectors.sha256"
    docker exec "$CTL" pqcaton-collectors install -ws "$WS" -from "$src/collectors" || rc=1
  fi
  docker exec "$CTL" rm -rf -- "$src"
  [ "$rc" -eq 0 ] || return 1
  echo "   collectors: the verified $PIN release assets for ${TARGET/-//}"
}

# 컨테이너 안에서 화면이 답하는가. curl은 Host를 127.0.0.1:8765로 보내 화면의 Host 검사를 지난다.
answers() { docker exec "$CTL" curl -fsS -o /dev/null http://127.0.0.1:8765/ 2>/dev/null; }

if [ "$MODE" = "apply-scope" ]; then
  # 1) 적재 전에 점검한다: 작업 폴더의 파일 · 결과 · 필요한 프로그램 · DB.
  echo "▶ 1/2 check ${WS}, the programs and the pqcota DB…"
  docker exec "$CTL" bash -c "
    set -e
    for f in $WS/asset-scope.csv $WS/declaration.json; do
      [ -f \$f ] || { echo \"❌ missing \$f — finalize the policy in 02 first\"; exit 1; }
    done
    ls $WS/results/*.json* >/dev/null 2>&1 || { echo '❌ no results in $WS/results'; exit 1; }
    command -v pqcota-ingest >/dev/null || { echo '❌ pqcota-ingest not found in $CTL'; exit 1; }
  "
  docker exec pqcota-demo-pg pg_isready -q || { echo "❌ the pqcota DB (pqcota-demo-pg) is not ready"; exit 1; }
  # 2) pqcota 적재. 화면이 실제로 쓰는 결과 폴더와 02에서 확정한 정책으로 한다.
  echo "▶ 2/2 re-ingest pqcota with the finalized policy ($WS/asset-scope.csv, $WS/results)…"
  docker exec -e PQCOTA_DSN="$DSN" -e PQCOTA_ORG="$ORG" "$CTL" pqcota-ingest -scope-assets "$WS/asset-scope.csv" "$WS/results" | sed 's/^/   /'
  echo "✅ re-ingested. Activate it in 05 (the screen reconciles the pqcota history first): ./demo/scripts/ui.sh"
  exit 0
fi

# 수집기 확인이 먼저다(화면이 이미 떠 있어도). 통과하지 못하면 화면도 작업 폴더도 건드리지 않는다.
echo "▶ 0/3 verify the prepared collectors (${COLLECTORS})…"
verify_collectors || { echo "❌ the prepared collectors did not verify; nothing was started or changed"; exit 1; }

if answers; then
  # 화면이 떠 있는 동안에는 collectors를 바꾸지 않는다. 지금 것이 확인한 준비 폴더와 같으면 그대로 쓰고, 다르면 거절한다.
  if [ "$(prepared_sums)" = "$(docker exec "$CTL" bash -c "cd $WS/collectors 2>/dev/null && find . -type f | LC_ALL=C sort | while read -r f; do sha256sum \"\$f\"; done")" ]; then
    echo "▶ pqcaton-ui is already running in $CTL with these collectors — reusing it (./demo/scripts/ui.sh stop to restart)"
  else
    echo "❌ pqcaton-ui is running in $CTL with other collectors; stop the screen first (./demo/scripts/ui.sh stop). Nothing was changed"
    exit 1
  fi
else

  echo "▶ 1/3 build pqcaton-ui and pqcaton-collectors for ${CTL}…"
  install_bin pqcaton-ui ./inventory/cmd/pqcaton-ui
  install_bin pqcaton-collectors ./tools/collectors

  echo "▶ 2/3 workspace ${WS}…"
  # **처음 한 번만 채운다.** 데모가 이미 가진 것을 실제 운영자가 화면에 넣는 자리에 둔다. 선언(01) ·
  # 자산 스코프 정책(상류 적재와 같은 파일이어야 스냅샷 지문이 상류 이력과 맞는다) · 접근 정보(03) ·
  # 지금까지의 관측 결과(04). 03에서 다시 관측하면 결과는 화면이 바꿔 넣는다. 수집기(03)는 띄울 때마다 위에서 확인한
  # 공식 릴리스 자산으로 바꿔 넣는다(상류 데모가 소스로 빌드한 것은 쓰지 않는다).
  docker exec "$CTL" bash -c "
    set -e
    if [ -d $WS ]; then echo '   kept (already there)'; else
      mkdir -m 700 $WS
      cp /work/declaration.json /work/scope-assets.csv $WS/
      install -m 600 /work/hosts.csv $WS/hosts.csv
      cp -r /work/results $WS/results
      echo '   created: declaration · asset-scope policy · hosts · results'
    fi
    if [ ! -f $KEY ]; then umask 077; pqcota-keygen > $KEY; echo '   approval key for 06: $KEY'; fi
  "
  sync_collectors
  # **노드의 호스트 키를 미리 받아 둔다.** 운영자 안내서 2절의 준비(처음 접속하는 노드는 ssh로 한 번
  # 접속해 호스트 키를 받아 둔다)를 데모에서 대신한다. 화면은 운영 환경처럼 호스트 키를 검사하므로, 이것이
  # 없으면 03 관측이 「Host key verification failed」로 멈춘다(상류 데모의 Ansible 설정은 검사를 끈다).
  # 접근 정보는 03에서 바뀔 수 있으므로 띄울 때마다 작업 폴더의 hosts.csv를 읽고, 이미 있는 키는 두고 없는 것만 더한다.
  docker exec "$CTL" bash -c '
    mkdir -p -m 700 ~/.ssh; touch ~/.ssh/known_hosts
    tail -n +2 '"$WS"'/hosts.csv | while IFS=, read -r _ _ ip port _; do
      [ -n "$ip" ] || continue; port=${port:-22}; h=$ip; [ "$port" = 22 ] || h="[$ip]:$port"
      ssh-keygen -F "$h" >/dev/null 2>&1 || ssh-keyscan -p "$port" "$ip" 2>/dev/null >> ~/.ssh/known_hosts
    done
    echo "   node host keys: $(wc -l < ~/.ssh/known_hosts) in ~/.ssh/known_hosts"
  '

  echo "▶ 3/3 start pqcaton-ui in $CTL (127.0.0.1:8765 inside the container)…"
  # 수집기는 확인한 공식 자산이므로 03에서 「확인됨」이고, -allow-unverified-collectors 없이 띄운다.
  # 화면은 pqcota 이력 없이 시작하지 않는다. 연결 정보는 환경 변수로만 넘긴다(명령줄 인자가 아니다).
  docker exec -d -e PQCOTA_DSN="$DSN" "$CTL" bash -c "exec pqcaton-ui -workspace $WS -no-browser -org $ORG -approval-key $KEY >>$LOG 2>&1"
  for _ in $(seq 1 40); do answers && break; sleep 0.5; done
  answers || { echo "❌ pqcaton-ui did not answer — last lines of $CTL:$LOG:"; docker exec "$CTL" tail -20 "$LOG"; exit 1; }
fi

if [ "$MODE" = "start" ]; then
  echo "✅ pqcaton-ui is running in $CTL (127.0.0.1:8765 inside the container); no relay was opened"
  exit 0
fi

# 중계. 이 기계의 127.0.0.1:$PORT에서 받고, 연결마다 docker exec로 컨테이너 안의 127.0.0.1:8765에 잇는다.
# 바이트를 그대로 옮기므로 브라우저가 보낸 Host(127.0.0.1:$PORT)와 Origin이 화면까지 그대로 간다.
exec python3 -I - "$PORT" "$CTL" <<'RELAY'
import shutil, socket, subprocess, sys, threading

port, ctl = int(sys.argv[1]), sys.argv[2]
INNER = r"""
import socket, sys, threading
s = socket.create_connection(("127.0.0.1", 8765))
def up():
    try:
        while True:
            b = sys.stdin.buffer.read1(65536)
            if not b:
                break
            s.sendall(b)
    finally:
        try:
            s.shutdown(socket.SHUT_WR)
        except OSError:
            pass
threading.Thread(target=up, daemon=True).start()
while True:
    b = s.recv(65536)
    if not b:
        break
    sys.stdout.buffer.write(b)
    sys.stdout.buffer.flush()
"""

def serve(c):
    p = subprocess.Popen(["docker", "exec", "-i", ctl, "python3", "-I", "-c", INNER],
                         stdin=subprocess.PIPE, stdout=subprocess.PIPE, bufsize=0)
    def up():
        try:
            while True:
                b = c.recv(65536)
                if not b:
                    break
                p.stdin.write(b)
        except OSError:
            pass
        finally:
            try:
                p.stdin.close()
            except OSError:
                pass
    threading.Thread(target=up, daemon=True).start()
    try:
        while True:
            b = p.stdout.read(65536)
            if not b:
                break
            c.sendall(b)
    except OSError:
        pass
    finally:
        c.close()
        p.wait()

ls = socket.socket()
ls.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
try:
    ls.bind(("127.0.0.1", port))
except OSError as e:
    sys.exit(f"❌ cannot listen on 127.0.0.1:{port} ({e.strerror}) — another screen may be using it; set PQCATON_UI_PORT")
ls.listen(64)
url = f"http://127.0.0.1:{port}"
print(f"✅ open {url} in this machine's browser (Ctrl+C closes the relay; the screen keeps running in {ctl})", flush=True)
opener = shutil.which("open") if sys.platform == "darwin" else None
if opener:
    subprocess.Popen([opener, url], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
try:
    while True:
        c, _ = ls.accept()
        threading.Thread(target=serve, args=(c,), daemon=True).start()
except KeyboardInterrupt:
    print("\n   relay closed", flush=True)
RELAY
