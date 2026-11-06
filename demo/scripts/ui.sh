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
#       이 기계에 docker · python3, 그리고 Go. Go가 없으면 linux용 pqcaton-ui를 PQCATON_BIN_DIR로 준다.
#
# 쓰는 법:
#   ./demo/scripts/ui.sh         화면을 띄우고(이미 떠 있으면 그대로 쓴다) 중계를 연다. Ctrl+C는 중계만 닫는다.
#   ./demo/scripts/ui.sh stop    컨테이너 안의 화면을 끝낸다. 다시 빌드한 화면을 띄우려면 stop 뒤에 다시 연다.
#   PQCATON_UI_PORT=8766 ...     이 기계에서 받을 포트(기본 8765). 이 기계에서 다른 화면을 이미 띄웠을 때 바꾼다.
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"   # pqcaton/demo
REPO_DIR="$(cd "$DEMO_DIR/.." && pwd)"                        # pqcaton

CTL=pqcota-ctl
# 작업 폴더(컨테이너 안). 처음 한 번만 채우고, 그 뒤로는 손대지 않는다. 화면에서 한 일이 거기 남는다.
WS=/work/ui
# 06의 승인 서명 키. 작업 폴더 밖에 0600으로 둔다(운영자 안내서 2절과 같은 자리 규칙).
KEY=/work/ui-approval.key
LOG=/work/ui.log
PORT="${PQCATON_UI_PORT:-8765}"

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
if [ $# -gt 0 ]; then
  echo "usage: $0 [stop]"; exit 2
fi

command -v python3 >/dev/null || { echo "❌ python3 not found on this machine — the relay needs it"; exit 1; }
docker exec "$CTL" test -f /work/declaration.json || { echo "❌ no /work/declaration.json — run ./demo/scripts/extend.sh first"; exit 1; }

# 컨테이너 안에서 화면이 답하는가. curl은 Host를 127.0.0.1:8765로 보내 화면의 Host 검사를 지난다.
answers() { docker exec "$CTL" curl -fsS -o /dev/null http://127.0.0.1:8765/ 2>/dev/null; }

if answers; then
  echo "▶ pqcaton-ui is already running in $CTL — reusing it (./demo/scripts/ui.sh stop to restart)"
else
  echo "▶ 1/3 build pqcaton-ui for ${CTL}…"
  TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
  if [ -n "${PQCATON_BIN_DIR:-}" ]; then
    echo "   using the prebuilt binary from $PQCATON_BIN_DIR"
    cp "$PQCATON_BIN_DIR/pqcaton-ui" "$TMP/"
  else
    # extend.sh 1/8과 같은 이유로 컨테이너용으로 빌드한다. 아키텍처는 컨테이너에 묻는다.
    case "$(docker exec "$CTL" uname -m)" in
      x86_64|amd64)  CTL_ARCH=amd64 ;;
      aarch64|arm64) CTL_ARCH=arm64 ;;
      *) echo "❌ unsupported $CTL architecture: $(docker exec "$CTL" uname -m)"; exit 1 ;;
    esac
    echo "   target: linux/$CTL_ARCH ($CTL)"
    ( cd "$REPO_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=$CTL_ARCH go build -o "$TMP/pqcaton-ui" ./inventory/cmd/pqcaton-ui )
  fi
  docker cp -q "$TMP/pqcaton-ui" "$CTL":/usr/local/bin/pqcaton-ui
  # 아래에서 셸을 python으로 바꾸므로(exec) EXIT trap이 돌지 않는다. 여기서 지운다.
  rm -rf "$TMP"; trap - EXIT
  rc=0; docker exec "$CTL" bash -c "/usr/local/bin/pqcaton-ui -h >/dev/null 2>&1" || rc=$?
  if [ "$rc" -eq 126 ] || [ "$rc" -eq 127 ] || [ "$rc" -eq 255 ]; then
    echo "❌ pqcaton-ui does not run inside $CTL (exit $rc) — it must be a linux/$(docker exec "$CTL" uname -m) binary"
    exit 1
  fi

  echo "▶ 2/3 workspace ${WS}…"
  # **처음 한 번만 채운다.** 데모가 이미 가진 것을 실제 운영자가 화면에 넣는 자리에 둔다. 선언(01) ·
  # 자산 스코프 정책(상류 적재와 같은 파일이어야 스냅샷 지문이 상류 이력과 맞는다) · 접근 정보(03) ·
  # 수집기(03, pqcota-discovery가 빌드한 모양 그대로) · 지금까지의 관측 결과(04). 03에서 다시 관측하면
  # 결과는 화면이 바꿔 넣는다.
  docker exec "$CTL" bash -c "
    set -e
    if [ -d $WS ]; then echo '   kept (already there)'; else
      mkdir -m 700 $WS
      cp /work/declaration.json /work/scope-assets.csv $WS/
      install -m 600 /work/hosts.csv $WS/hosts.csv
      cp -r /work/results $WS/results
      cp -r /work/dist $WS/collectors
      echo '   created: declaration · asset-scope policy · hosts · collectors · results'
    fi
    if [ ! -f $KEY ]; then umask 077; pqcota-keygen > $KEY; echo '   approval key for 06: $KEY'; fi
  "
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
  docker exec -d "$CTL" bash -c "exec pqcaton-ui -workspace $WS -no-browser -org demo-corp -approval-key $KEY >>$LOG 2>&1"
  for _ in $(seq 1 40); do answers && break; sleep 0.5; done
  answers || { echo "❌ pqcaton-ui did not answer — last lines of $CTL:$LOG:"; docker exec "$CTL" tail -20 "$LOG"; exit 1; }
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
