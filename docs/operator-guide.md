# pqcaton 운영자 안내서

<!-- SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd. -->
<!-- SPDX-License-Identifier: BUSL-1.1 -->

이 안내서는 pqcaton 화면(`pqcaton-ui`)을 한 기계에 설치해 혼자 운영하는 사람을 위한 것입니다. v0.23.1 기준입니다.
영어판은 [operator-guide.en.md](operator-guide.en.md)입니다. 두 판은 차례 · 명령 · 오류 코드가 같고, 시험이 그것을 잽니다.

**범위.** pqcaton 안의 절차(선언 · 스코프 · 관측 · 대조 · 판정 · 확정 · 승인 · 플레이북 생성)는 터미널 없이 화면에서
합니다. 관측은 운영자가 설치한 Ansible과 수집기로 실행합니다. 만든 플레이북을 운영 기계에 적용하는 일은 화면이 하지 않고
운영자가 Ansible로 합니다. 설치와 복구에는 터미널이나 파일 관리자가 필요합니다.

**신뢰 모델.** 화면은 한 기계에서 **한 OS 사용자**가 띄우고 **루프백**에서만 열립니다(인증이 없습니다). 그 사용자는
관측할 노드에 SSH로 접속해 **root 권한(become)으로** 수집기를 실행할 수 있고, 06에서 서명한다면 **승인 개인키를
읽습니다.** 그래서 그 사용자의 계정과 작업 폴더가 곧 이 권한들의 경계입니다. 06은 키 하나로 서명하고 확인합니다(단일
키 · 단일 승인자).

**명령은 실제로 돌려 봅니다.** 이 안내서에서 `<!-- run -->` 표시가 붙은 명령은 시험(`guide_run_test.go`)이 빈 홈 폴더에서
차례대로 실행해 결과를 확인합니다.

## 차례

1. [지원 플랫폼](#1-지원-플랫폼)
2. [설치](#2-설치)
3. [작업 폴더](#3-작업-폴더)
4. [시작과 끝내기](#4-시작과-끝내기)
5. [다른 기계에서 보기](#5-다른-기계에서-보기)
6. [하루 절차](#6-하루-절차)
7. [백업과 복원](#7-백업과-복원)
8. [복구](#8-복구)
9. [판 올리기(v0.22에서 v0.23으로)](#9-판-올리기v022에서-v023으로)
10. [오류 표](#10-오류-표)

## 1. 지원 플랫폼

| 플랫폼 | 상태 |
|---|---|
| Linux(x86-64) | **지원합니다.** 저장 · 확정 · 복구를 검증했습니다. systemd 사용자 서비스와 데스크톱 실행기를 씁니다 |
| macOS Intel(amd64) | **지원합니다.** macOS 26.6.2의 로컬 파일 시스템에서 저장 · 확정 · 복구와 실행기(`pqcaton-ui.command`)를 검증했습니다(v0.23.1) |
| macOS Apple Silicon(arm64) | 실행할 수 있지만 아직 검증하지 않았습니다. 시작할 때 경고를 남깁니다 |
| Windows | 보기 전용입니다. 화면은 뜨지만 저장하지 않습니다 |

화면은 이 기계 안(루프백 주소)에서만 열립니다. 로그인이 없기 때문입니다. 다른 기계에서는 SSH 터널로 봅니다([5절](#5-다른-기계에서-보기)).

## 2. 설치

**바이너리.** Go 1.26.10이 필요합니다. 아래 명령은 `~/.local/bin/pqcaton-ui`를 만듭니다. 이 폴더가 `PATH`에 있어야 합니다.

```bash
GOBIN="$HOME/.local/bin" go install github.com/sntsoftgit/pqcaton/inventory/cmd/pqcaton-ui@v0.23.1
```

**Linux: 서비스와 실행기.** 서비스는 로그인할 때 화면을 띄우고(브라우저는 열지 않음), 실행기는 서비스를 시작한 뒤 화면을
브라우저로 엽니다. 실행기는 데스크톱의 프로그램 목록에 「pqcaton」으로 나타납니다. 실행기는 `pqcaton-ui -open`으로 엽니다.
적힌 주소의 서버가 같은 작업 폴더의 화면이라고 답할 때까지(최대 15초) 기다린 뒤에 열기 때문에, 비정상 종료가 남긴 낡은
주소나 아직 뜨지 않은 화면을 열지 않습니다. 끝내 답이 없으면 열지 않습니다.

```bash
mkdir -p ~/.config/systemd/user ~/.local/share/applications
curl -fsSLo ~/.config/systemd/user/pqcaton-ui.service https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.23.1/packaging/linux/pqcaton-ui.service
curl -fsSLo ~/.local/share/applications/pqcaton-ui.desktop https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.23.1/packaging/linux/pqcaton-ui.desktop
systemctl --user daemon-reload
systemctl --user enable --now pqcaton-ui.service
```

서비스의 기록은 아래 명령으로 봅니다. 화면이 보이는 참조 번호(`error ref=…`)로 원인을 찾을 때 씁니다.

```bash
journalctl --user -u pqcaton-ui.service
```

**macOS: 실행기.** 내려받은 파일을 두 번 누르면 터미널 창에서 화면을 띄우고 브라우저를 엽니다. 이미 떠 있으면 그
화면만 엽니다. 끝낼 때는 그 터미널 창에서 Ctrl+C를 누릅니다.

```bash
curl -fsSLo ~/Desktop/pqcaton-ui.command https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.23.1/packaging/macos/pqcaton-ui.command
chmod +x ~/Desktop/pqcaton-ui.command
```

**관측 실행 준비.** 03 관측 화면에서 관측을 실행하려면 이 기계에 두 가지가 더 있어야 합니다. 화면은 시작하기 전에 둘을
점검하고, 빠진 것이 있으면 무엇을 하면 되는지 보입니다.

- **Ansible.** `ansible-playbook`을 `PATH`에서 찾습니다. 노드에는 SSH(Windows 노드는 SSH나 WinRM)로 접속하고, Linux
  노드에서는 root 권한(`become`)으로 실행하므로 그 계정에 sudo 권한이 있어야 합니다. 처음 접속하는 노드는 미리 `ssh`로
  한 번 접속해 호스트 키를 받아 둡니다. Windows 노드를 관측하려면 `ansible.windows` 컬렉션도 필요합니다.
- **수집기.** 작업 폴더의 `collectors` 폴더(또는 `-collectors <폴더>`)에 pqcota-discovery를 빌드한 결과를 아래 모양으로
  둡니다.

```text
collectors/
├── linux-amd64/     pqcota-nodescan · pqcota-netcap · pqcota-jvmscan
├── linux-arm64/     arm64 노드가 있을 때
├── windows-amd64/   pqcota-cngscan.exe · pqcota-jvmscan.exe (Windows 노드가 있을 때)
└── collector.jar    없으면 실행 중인 JVM의 provider 체인은 관측하지 않습니다
```

**승인 키 준비.** 06 화면에서 승인 서명을 하려면 승인 키가 필요합니다. `pqcota-keygen`의 출력을 작업 폴더 **밖**의
파일에 소유자만 읽게 저장하고, 화면을 `-approval-key <파일>`로 띄웁니다. 화면은 서명할 때마다 이 파일을 다시 읽고, 다른
사용자가 읽을 수 있거나 작업 폴더 안에 있으면 쓰지 않습니다.

```bash
GOBIN="$HOME/.local/bin" go install github.com/randyinthedev-hash/pqcota-common/cmd/pqcota-keygen@v0.10.4
mkdir -p ~/.config/pqcaton
(umask 077 && pqcota-keygen > ~/.config/pqcaton/approval.key)
```

서비스로 띄운다면 서비스에 플래그를 더합니다.

```bash
mkdir -p ~/.config/systemd/user/pqcaton-ui.service.d
printf '[Service]\nExecStart=\nExecStart=%%h/.local/bin/pqcaton-ui -no-browser -approval-key %%h/.config/pqcaton/approval.key\n' > ~/.config/systemd/user/pqcaton-ui.service.d/approval.conf
systemctl --user daemon-reload
systemctl --user restart pqcaton-ui.service
```

## 3. 작업 폴더

인자 없이 띄우면 홈의 `pqcaton` 폴더를 씁니다. 없으면 소유자만 읽을 수 있게(0700) 만듭니다. 다른 폴더를 쓰려면
`-workspace <폴더>`를 줍니다. **파일 이름은 정해져 있고 바꿀 수 없습니다.** 경로 플래그(`-decl` 같은 것)는 작업 폴더와 함께
쓸 수 없습니다. 동작 플래그(`-addr` · `-no-browser` · `-org`)는 함께 씁니다.

| 자리 | 무엇 | 누가 쓰나 |
|---|---|---|
| `declaration.json` | 선언(관리 대상 노드 · 자산 · 엣지) | 화면(01) |
| `hosts.csv` | 관측할 노드의 접근 정보. 비밀번호를 적으면 **이 파일에 남고 백업에도 들어갑니다.** 소유자만 읽습니다(0600) | 화면(03) |
| `observations/` | 관측 실행마다 하나씩 남는 자리. 기록 · 로그(비밀번호는 가림) · 바꾸기 전 결과(`previous/`) · 지금 결과가 되지 못한 결과(`partial/`) | 화면(03) |
| `collectors/` | 관측에 쓰는 수집기. 화면은 읽기만 합니다 | 운영자 |
| `results/` | 관측 결과 | 화면(03 관측) · pqcota 수집 |
| `session.json` | 판정 세션 | 화면(05) |
| `layers/*.csv` | 자산 스코프 계층. 이름 순으로 겹치고, 같은 자산에 규칙이 겹치면 뒤의 계층을 따릅니다 | 화면(02) |
| `scope-session.json` | 스코프 세션 | 화면(02) |
| `asset-scope.csv` | 확정한 자산 스코프 정책. 있으면 다음 검토의 기준입니다 | 화면(02 확정) |
| `scope-assets.csv` | pqcota 적재(`pqcota-ingest`)에 준 것과 같은 정책. 있으면 대조에 씁니다 | 운영자 |
| `plan.json` | 확정한 계획(pqcota가 읽는 계약 형식) | 화면(05 확정) |
| `plan.signed.json` | 승인 서명한 계획(FINALIZED). `plan.json`은 그대로 남습니다. 있으면 덮지 않습니다 | 화면(06 승인) |
| `plan.provision.yml` · `plan.rollback.yml` | 적용 · 되돌리기 플레이북 | 화면(06 생성) |
| `plan.signed.<시각>.json` 등 | 「지난 서명 보관」으로 이름을 바꿔 둔 지난 서명과 플레이북 | 화면(06 보관) |
| `judgments.jsonl` | 판정 원장. 덧붙이기만 합니다 | 화면(확정) |
| `.pqcaton/` | 저장 기록 · 영수증 · 잠금 · 떠 있는 화면의 주소(`screen.addr`) | 화면 |
| `.pqcaton.lock` | 이 폴더가 어느 상태 폴더에 묶였는지 적은 잠금 파일. `.pqcaton/`과 짝을 이룹니다 | 화면 |

**조직.** `-org`를 주지 않으면 판정은 선언에 적힌 조직에 묶입니다. `-org`를 주면 그 조직의 선언만 만들 수 있습니다.

## 4. 시작과 끝내기

직접 띄울 때는 터미널에서 실행합니다. 포트를 연 뒤 브라우저를 엽니다(`http://127.0.0.1:8765`). 브라우저를 열지 않으려면
`-no-browser`를 줍니다.

<!-- run: background -->
```bash
pqcaton-ui -no-browser
```

떠 있는 화면의 주소는 작업 폴더에 적혀 있습니다.

<!-- run -->
```bash
cat ~/pqcaton/.pqcaton/screen.addr
```

**이미 떠 있으면 새로 띄우지 않습니다.** 같은 작업 폴더로 다시 실행하면 떠 있는 화면을 엽니다. 적힌 주소가 루프백이고
그 주소의 서버가 같은 작업 폴더의 화면이라고 답할 때만 엽니다.

<!-- run -->
```bash
pqcaton-ui -no-browser
```

새로 띄우지 않고 떠 있는 화면만 열려면 `-open`을 씁니다(실행기가 쓰는 방식). `-no-browser`를 함께 주면 열지 않고
확인된 주소만 찍습니다.

<!-- run -->
```bash
pqcaton-ui -open -no-browser
```

**끝내기.** 띄운 터미널에서 Ctrl+C를 누르거나, 서비스면 `systemctl --user stop pqcaton-ui.service`를 실행합니다. 다른 터미널에서
끝내려면 아래처럼 합니다. 끝나면 적힌 주소를 지우고, 아래 두 번째 명령이 아무것도 찍지 않습니다.

<!-- run -->
```bash
pkill -INT -u "$USER" -x pqcaton-ui
```

<!-- run: until -->
```bash
! pgrep -u "$USER" -x pqcaton-ui
```

입력 파일이 깨져 있으면 화면은 **진단 전용**으로 뜹니다. 모든 화면이 읽지 못한 파일의 이름과 참조 번호를 보이고 저장을 멈춥니다.
파일을 고친 뒤 화면을 다시 띄웁니다.

## 5. 다른 기계에서 보기

화면은 루프백 주소에서만 엽니다. 다른 주소를 주면 시작하지 않습니다.

<!-- run: fails -->
```bash
pqcaton-ui -addr 0.0.0.0:8765 -no-browser
```

다른 기계에서 보려면 그 기계에서 SSH로 포트를 넘기고, 그 기계의 브라우저로 `http://127.0.0.1:8765`를 엽니다. `operator@screen-host`는
화면이 떠 있는 기계의 계정과 이름으로 바꿉니다.

```bash
ssh -N -L 8765:127.0.0.1:8765 operator@screen-host
```

## 6. 하루 절차

화면 위쪽의 단계 카드 여섯이 절차 순서입니다. 카드마다 지금 상태가 적혀 있고, 확정하는 두 단계(02 · 05)는 확정 상태
(미확정 · 확정됨 · 대체됨 · 복구 필요)도 적혀 있습니다.

1. **01 선언.** 처음이면 조직 이름을 넣어 빈 선언을 만듭니다. 관리 대상 노드와 IP, 노드에서 쓰는 암호 자산, 통신 엣지를
   적고 저장합니다. 앞뒤가 맞지 않는 자리(IP 형식 · 겹친 IP · 포트 0 같은 것)는 화면이 칸 단위로 짚습니다. 저장은 막지
   않지만 그대로 두면 대조 결과가 오류 없이 틀립니다.
2. **02 자산 스코프.** 처음이면 계층 이름을 넣어 빈 계층을 만듭니다. 규칙을 적고, 바뀐 규칙마다 결론을 적고, 서명해
   확정합니다. 확정하면 `asset-scope.csv`가 생깁니다.
3. **03 관측.** 관측할 노드와 접속 방법(IP · 포트 · 계정 · SSH 키 경로 · OS · SSH나 WinRM)을 적고 저장합니다. 작업 폴더의
   `hosts.csv`에 `pqcota-hosts`가 읽는 형식으로 저장됩니다. 비밀번호는 화면에서 적지 않습니다. 파일에 있으면 「있음」으로만
   보이고 저장해도 그대로 남습니다. 「관측 시작」을 누르면 표의 노드마다 수집기를 올려 실행하고, 결과를 `results/`로 받은
   뒤 노드에서 지웁니다. 실행하는 동안 로그가 보이고 「중지」로 멈출 수 있습니다. **모든 노드의 결과가 오고 성공했을 때만
   지금 결과를 한 번에 바꿉니다.** 실패하거나, 멈췄거나, 결과가 빠진 노드가 있으면 지금 결과는 그대로이고 받은 것은
   `observations/<실행>/partial/`에 진단용으로 남습니다. 받은 결과는 상류 디코더로 읽어, 깨진 파일 · 다른 노드의 결과 ·
   다른 수집기의 결과가 하나라도 있으면 지금 결과로 쓰지 않습니다. 끝까지 돌지 못한 실행 뒤에는 노드의 임시 폴더를 지우는
   정리를 한 번 더 돌립니다. 화면을 끄면 돌던 관측도 멈추고, 정리가 끝난 뒤에 화면이 끝납니다. 화면이 비정상으로 끝나면(강제
   종료 · 전원 장애) 다음 시작이 아직 살아 있는 Ansible을 끝내고, 노드 정리를 시도하고, 접근 비밀이 담긴 인벤토리를
   지웁니다. 노드에 닿지 못했으면 03 화면이 남았을 수 있는 스테이징 폴더를 실행 ID와 함께 적습니다:
   `/tmp/pqcaton-observe-<실행 ID>`(Windows는 `C:\Windows\Temp\pqcaton-observe-<실행 ID>`). 네트워크 관측은 노드의 기본 경로
   인터페이스에서 정한 구간(기본 60초) 동안 핸드셰이크를 기다립니다.
4. **04 대조.** 관측이 끝나거나 pqcota 수집이 `results/`에 결과를 쓰면 화면을 다시 불러옵니다. 결과가 없으면 화면이 그
   폴더를 가리킵니다.
5. **05 판정.** 정책마다 결론을 적고, 계획에 넣을 항목은 조치 종류 · 목표 · 수준을 고르고, 서명합니다. 「확정」을 누르면
   확인 페이지가 무엇이 쓰이는지 보이고, 확인하면 세션 · 계획 · 원장을 한 번에 씁니다. 같은 세션을 같은 내용으로 다시
   확정해도 다시 쓰지 않습니다.
6. **06 승인 · 생성.** 승인할 계획의 조치를 확인하고, 승인자 이름과 검토 확인을 넣어 「승인 서명」을 누릅니다. 확인
   페이지가 계획 · 계획 파일 해시 · 승인자 · 키 지문을 보이고, 거기서 한 번 더 눌러야 서명합니다. 확인한 뒤에 계획이나
   키가 바뀌었으면 서명하지 않습니다. 서명한 계획은 `plan.signed.json`(FINALIZED)에 따로 남고 `plan.json`은 그대로입니다.
   **이미 있는 서명 파일은 어떤 경우에도 덮지 않습니다.** 지금 계획과 맞지 않으면 「지난 서명 보관」으로 이름에 시각을 붙여
   옆에 둔 뒤 새로 서명합니다. 이어서 기본 위임 수준을 골라 「플레이북 생성」을 누르면 `plan.provision.yml`과
   `plan.rollback.yml`이 생깁니다. 생성기가 짚은 빈 곳이 있으면 그 수와 내용이 보입니다.
   운영 기계에 적용하는 일은 화면이 하지 않습니다. 06 화면의 도움말에 적힌 명령으로 운영자가 실행합니다. 05에서 다시
   확정해 계획이 바뀌면 06 카드가 「서명이 지금 계획과 맞지 않습니다」가 되고, 지난 서명을 보관하고 새 계획에 서명해야
   만들 수 있습니다.
7. **다음 회차.** 확정한 세션은 고칠 수 없어 읽기 전용으로 보입니다. 새 관측이 들어왔거나 결론을 바꾸려면 05 화면의
   「새 검토 시작」(스코프는 02 화면의 「새 스코프 검토 시작」)을 누릅니다. 새 세션 ID로 새 회차가 서고, 결론과 계획 칸은
   옮겨 오며, 승인 서명은 새로 받습니다. 지난 회차의 계획 · 원장 기록은 그대로 남습니다.

인벤토리 조회(카드 옆)는 단계가 아니라 언제든 들어오는 자리입니다. 조건을 걸어 찾고, 자산 하나의 판정 이력을 봅니다.

## 7. 백업과 복원

<!-- setup: guidetool seed "$HOME/pqcaton" -->

**백업은 화면을 끈 상태에서만 합니다.** 떠 있는 화면은 저장 도중일 수 있습니다. 실행 중 백업은 지원하지 않습니다.

1. 화면을 끕니다([4절](#4-시작과-끝내기)).
2. 끝났는지 확인합니다. `stopped`가 찍혀야 합니다.

<!-- run -->
```bash
pgrep -u "$USER" -x pqcaton-ui || echo stopped
```

3. 작업 폴더 전체를 `.pqcaton/`까지 함께 복사합니다. 저장 기록과 잠금 파일이 짝을 이루므로 일부만 복사하지 않습니다.
   `hosts.csv`에 비밀번호를 적었다면 백업에도 그대로 들어가므로, 백업을 작업 폴더와 같은 수준으로 지킵니다. 비밀번호 대신
   SSH 키를 쓰면 백업에 비밀이 남지 않습니다.

<!-- run -->
```bash
cp -a ~/pqcaton ~/pqcaton-backup-$(date +%Y%m%d)
```

**복원**도 화면을 끈 상태에서 폴더 전체를 되돌립니다. 지금 폴더는 지우지 않고 옆으로 옮겨 둡니다. 마지막 줄은 백업과
같은지 확인합니다.

<!-- run -->
```bash
mv ~/pqcaton ~/pqcaton-before-restore
cp -a ~/pqcaton-backup-$(date +%Y%m%d) ~/pqcaton
diff -r ~/pqcaton-backup-$(date +%Y%m%d) ~/pqcaton && echo restored
```

복원한 뒤 화면을 띄우고 열리는지 봅니다.

<!-- run: background -->
```bash
pqcaton-ui -no-browser
```

<!-- run -->
```bash
curl -fsS -o /dev/null "http://$(cat ~/pqcaton/.pqcaton/screen.addr)/review" && echo ok
```

<!-- run -->
```bash
pkill -INT -u "$USER" -x pqcaton-ui
```

<!-- run: until -->
```bash
! pgrep -u "$USER" -x pqcaton-ui
```

## 8. 복구

<!-- setup: guidetool stuck "$HOME/pqcaton" -->

저장하다 멈춘 기록이 남아 있고 그 기록의 파일이 pqcaton 밖에서 바뀌었으면, 화면은 **복구 필요** 상태로 뜹니다. 모든 화면이
남은 기록과 파일마다 지금 상태를 보이고, 저장을 멈춥니다. **화면은 어느 파일이 옳은지 정하지 않으므로 자동으로 고치지
않습니다.** 고치는 버튼도 없습니다. 아래 절차를 따릅니다.

1. **지금 상태를 받아 둡니다.** 화면의 「진단 파일 내려받기」를 누르거나 터미널에서 아래를 실행합니다. 화면을 끄지 않아도
   됩니다.

<!-- run -->
```bash
pqcaton-ui -recover-status -diagnostics ~/pqcaton-diagnostics.json
```

   출력의 줄마다 판정이 있습니다. `done`은 이미 적용됨, `pending`은 다시 띄우면 끝까지 적용됨, `partial`은 원장에 일부만 써져
   다시 띄우면 나머지를 씀, `mismatch`는 pqcaton 밖에서 바뀜입니다. **고쳐야 하는 것은 `mismatch`뿐입니다.**

2. **`mismatch`인 파일을 지금 상태 그대로 보존합니다.** 출력에서 `mismatch` 줄의 경로를 `TARGET`에, 그 아래
   `prepared copy at` 뒤의 경로를 `PREPARED`에 넣습니다(아래는 예시입니다).

```bash
TARGET="$HOME/pqcaton/session.json"
PREPARED="$HOME/pqcaton/.session.json.pqcaton-<txid>.tmp"
```

<!-- run -->
```bash
cp -a "$TARGET" ~/pqcaton-kept-$(date +%Y%m%d%H%M%S)-$(basename "$TARGET")
```

3. **기록이 기대하는 상태로 맞춥니다.** 준비 파일이 `present, matches`이면 그것을 대상 자리에 둡니다. 다시 확인하면 그 줄이
   `done`이 됩니다.

<!-- run -->
```bash
cp "$PREPARED" "$TARGET"
```

<!-- run -->
```bash
pqcaton-ui -recover-status
```

4. **화면을 끄고 다시 띄웁니다.** 시작할 때 남은 기록을 끝까지 적용하고 복구 필요 상태가 풀립니다.

<!-- run: background -->
```bash
pqcaton-ui -no-browser
```

<!-- run -->
```bash
curl -fsS -o /dev/null "http://$(cat ~/pqcaton/.pqcaton/screen.addr)/review" && echo ok
```

5. 보존한 파일과 견줘, 밖에서 했던 변경이 필요하면 화면에서 다시 합니다.

<!-- run -->
```bash
pkill -INT -u "$USER" -x pqcaton-ui
```

<!-- run: until -->
```bash
! pgrep -u "$USER" -x pqcaton-ui
```

**맞출 수 없을 때는 멈추고 지원을 요청합니다.** 준비 파일이 없거나 `present, differs`인 경우, 원장의 앞부분이 바뀐 경우(원장은
덧붙이기만 하므로 자르지 않습니다)가 그렇습니다. 진단 파일과 보존한 파일을 함께 보냅니다. 진단 파일에는 경로와 해시만 있고
파일 내용은 없습니다.

## 9. 판 올리기(v0.22에서 v0.23으로)

1. 화면을 끄고 [2절](#2-설치)의 `go install`로 바이너리를 바꿉니다.
2. **옛 화면은 없어졌습니다.** `/classic/…`와 `-next` 주소의 북마크는 지금 화면으로 넘어갑니다(301).
3. **명시 모드는 그대로입니다.** 세션 파일과 경로 플래그로 띄우던 방식(`pqcaton-ui session.json -decl … -results …`)은 v0.22와 같이
   동작합니다. 판정이 기본 조직(`local`)에 묶이지 않게 `-org`를 꼭 줍니다. 세션 파일은 플래그보다 앞에 둡니다. 뒤에 두면
   사용법 오류로 시작하지 않습니다.
4. **작업 폴더로 옮기려면** 먼저 `-recover-status`가 `no unfinished transactions`를 찍는지 확인합니다. 화면을 끄고, 쓰던 파일을
   [3절](#3-작업-폴더)의 이름으로 `~/pqcaton`에 복사합니다. 옛 `.pqcaton/`은 복사하지 않습니다. 계획 파일과 원장은 그대로
   이어지지만, 단계 카드의 확정 상태는 새 작업 폴더에서 다시 확정하기 전까지 「미확정」으로 보입니다.

## 10. 오류 표

오류 페이지는 무엇이 안 됐는지, 다음에 할 일, 관련 파일의 역할과 이름, **오류 코드**를 보입니다. 원인을 화면 말로 옮길 수 없는
오류는 **참조 번호**도 보입니다. 같은 번호가 서버 기록(터미널이나 `journalctl`)에 원문과 함께 남아 있습니다. 화면에 전체 경로와
오류 원문은 나오지 않습니다.

| 코드 | 무엇이 안 됐나 | 할 일 |
|---|---|---|
| `stale` | 화면을 연 뒤 파일이 바뀌어 저장하지 않았습니다 | 화면을 다시 불러와 다시 합니다 |
| `finalized` | 이미 확정한 세션이라 저장하지 않았습니다 | 새 세션에서 합니다 |
| `busy` | 같은 파일에 다른 저장이 진행 중이거나, 다른 명령이 상태 폴더를 쥐고 있습니다 | 잠시 뒤 다시 합니다. 계속되면 다른 `pqcaton-ui`나 명령이 떠 있는지 봅니다 |
| `recovery` | 복구 필요 상태라 저장을 멈췄습니다 | [8절](#8-복구)을 따릅니다 |
| `view-only` | 이 플랫폼에서는 저장하지 않습니다 | Linux에서 띄웁니다 |
| `expired` | 확인 페이지가 만료되었습니다(10분) | 확정을 처음부터 다시 합니다 |
| `confirm` | 확인 페이지를 거치지 않은 확정입니다 | 「확정」을 눌러 확인 페이지에서 확인합니다 |
| `unsafe-file` | 링크이거나 남이 쓸 수 있는 파일이라 쓰지 않았습니다 | 그 파일의 종류와 권한을 고칩니다(일반 파일, 소유자만 쓰기) |
| `state-mismatch` | 그 폴더가 다른 상태 폴더에 묶여 있습니다 | 폴더를 옮기거나 복사했는지 봅니다. [7절](#7-백업과-복원)대로 폴더 전체를 되돌립니다 |
| `damaged` | 저장 기록이나 잠금 파일이 손상되었습니다 | 화면을 끄고 진단 파일을 받아 지원을 요청합니다 |
| `diagnostic` | 입력 파일을 읽지 못해 진단 전용으로 열었습니다 | 화면에 보인 파일을 고치고 다시 띄웁니다 |
| `exists` | 만들려는 파일이 이미 있어 덮어쓰지 않았습니다 | 화면을 다시 불러옵니다 |
| `run-not-ready` | 관측을 시작하기 전에 갖출 것(접근 정보 · Ansible · 수집기)이 빠졌습니다 | 03 화면의 점검 목록에서 ✗ 표시된 것을 갖춥니다 |
| `run-busy` | 이미 관측이 돌고 있습니다 | 끝나거나 멈춘 뒤에 다시 시작합니다 |
| `sign-not-ready` | 승인 키를 쓸 수 없거나, 승인자 이름이 비었거나 콜론을 담았거나, 검토 확인을 하지 않았습니다 | 06 화면의 승인 키 점검을 보고 양식을 채웁니다 |
| `plan-not-approvable` | 계획이 검토 중(IN_REVIEW)이 아니거나, 조치가 없거나, 이미 이 키로 서명했습니다 | 05에서 다시 확정하거나 서명한 계획을 그대로 씁니다 |
| `generate-not-ready` | 지금 계획에 이 키로 확인되는 승인 서명이 없습니다 | 먼저 승인 서명을 합니다 |
| `signed-exists` | 서명한 계획 파일이 이미 있어 덮지 않았습니다 | 지금 계획과 맞지 않으면 06 화면에서 「지난 서명 보관」을 누른 뒤 서명합니다 |
| `not-found` | 이 주소에는 화면이 없습니다 | 첫 화면에서 다시 갑니다 |
| `method` | 이 방식의 요청을 받지 않습니다 | 화면의 단추로 합니다 |
| `bad-request` | 요청을 처리할 수 없습니다 | 화면을 다시 불러와 다시 합니다 |
| `input-missing` | 이 화면에 필요한 파일이 없습니다 | 앞 단계(선언 · 결과)를 먼저 합니다 |
| `input-broken` | 입력 파일을 읽지 못했습니다 | 참조 번호로 서버 기록을 찾아 그 파일을 고칩니다 |
| `forbidden` | 요청 보호에 걸렸습니다 | 화면을 띄운 기계의 브라우저에서 처음부터 다시 엽니다. 다른 기계는 [5절](#5-다른-기계에서-보기)을 따릅니다 |
| `internal` | 처리하지 못했습니다 | 참조 번호로 서버 기록을 찾아 지원을 요청합니다 |
