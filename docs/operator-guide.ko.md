[English](operator-guide.md) · 한국어

# pqcaton 운영자 안내서

<!-- SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd. -->
<!-- SPDX-License-Identifier: BUSL-1.1 -->

이 안내서는 pqcaton 화면(`pqcaton-ui`)을 한 기계에 설치해 혼자 운영하는 사람을 위한 것입니다. v0.33.0 기준입니다.
영어판은 [operator-guide.md](operator-guide.md)입니다. 두 판은 차례 · 명령 · 오류 코드가 같고, 시험이 그것을 잽니다.

**범위.** pqcaton 안의 절차(선언 · 스코프 · 관측 · 대조 · 판정 · 확정 · 승인 · 플레이북 생성)는 터미널 없이 화면에서
합니다. 관측은 운영자가 설치한 Ansible과 수집기로 실행합니다. 만든 플레이북을 운영 기계에 적용하는 일은 화면이 하지 않고
운영자가 Ansible로 합니다. 설치와 복구에는 터미널이나 파일 관리자가 필요합니다.

**신뢰 모델.** 화면은 한 기계에서 **한 OS 사용자**가 띄우고 **루프백**에서만 열립니다(인증이 없습니다). 그 사용자는
관측할 노드에 SSH로 접속해 **root 권한(become)으로** 수집기를 실행할 수 있고, 06에서 서명한다면 **승인 개인키를
읽습니다.** 그래서 그 사용자의 계정과 작업 폴더가 곧 이 권한들의 경계입니다. 06은 키 하나로 서명하고 확인합니다(단일
키 · 단일 승인자).

**수집기 서명 키의 신뢰 전제.** 03은 수집기 ID별 서명 키(2절 「수집기 서명 키 준비」)를 읽어 관측마다 노드에 넘깁니다. 키는 노드
수집기의 표준 입력으로만 가고 파일 · 명령줄 · Ansible 설정에 남기지 않지만, **관측하는 동안에는 노드 수집기 프로세스의 환경에 있어
그 노드의 root가 읽을 수 있습니다.** 컨트롤러에서는 화면을 띄운 계정과 root가 키 파일과 임시 키를 읽을 수 있습니다. 노드 점검은
sudo의 입출력 기록만 봅니다. **sudo 밖의 감사 도구(auditd · tlog · pam_tty_audit 같은 것), sudo의 다른 정책 플러그인, 노드 root가 바꾼
실행 파일이 무엇을 기록하는지는 운영 환경이 책임질 몫입니다.** 한 ID의 키가 새면 그 ID로 모든 노드의 결과를 위조할 수 있으므로 그때는
키를 바꿉니다.

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
9. [판 올리기](#9-판-올리기)
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

이 절을 마치면 이 기계에서 화면이 뜹니다. 1~3은 처음 한 번 하고, 4 · 5는 그 단계를 쓸 때 합니다.

1. [화면 설치](#화면-설치): `pqcaton-ui` 바이너리
2. [pqcota 준비](#pqcota-준비): PostgreSQL 16 서버 · psql(먼저 준비) · 역할 · 상류 명령 · 스키마 · 권한 · 연결 정보
3. 화면 띄우기: [4절](#4-시작과-끝내기). 로그인할 때 띄우려면 [서비스와 실행기](#서비스와-실행기)
4. [관측 실행 준비](#관측-실행-준비): 03에서 관측할 때
5. [승인 키 준비](#승인-키-준비): 06에서 서명할 때

<a id="화면-설치"></a>

**화면 설치.** Go 1.26.11이 필요합니다. 아래 명령은 `~/.local/bin/pqcaton-ui`를 만듭니다. 이 폴더가 `PATH`에 있어야 합니다.

```bash
GOBIN="$HOME/.local/bin" go install github.com/sntsoftgit/pqcaton/inventory/cmd/pqcaton-ui@v0.33.0
```

`pqcaton-ui -h`가 플래그 목록을 찍으면 설치된 것입니다. 도움말을 찍은 뒤 종료 코드 2로 끝나는 것은 정상입니다.
`set -e`를 쓰는 설치 스크립트에서는 이 줄에서 멈추므로, 스크립트에서는 `command -v pqcaton-ui`로 확인합니다.
이것은 `PATH`에서 명령을 찾는지만 보는 검사이고, 명령을 실행해 보지는 않습니다. 화면은 다음 「pqcota 준비」를 마쳐야 뜹니다.

<a id="pqcota-준비"></a>

**pqcota 준비.** 화면은 pqcota 이력(PostgreSQL)을 읽으므로 화면보다 먼저 준비합니다. 아래 1~5단계를 차례로 하면 화면이 뜹니다.
이미 쓰던 pqcota 이력이 있으면 [9절 「v0.30.0에서 v0.31.0으로」](#v0300에서-v0310으로)의 「기존 이력을 세 역할로 옮기기」를 따릅니다.

1. **PostgreSQL과 세 역할.** **접속할 PostgreSQL 16 서버와, 아래 명령을 실행할 기계의 `psql`이 먼저 준비돼 있어야 합니다.**
   서버는 이 기계나 다른 기계에 둘 수 있습니다. 필요한 구성 요소가 없으면 PostgreSQL 공식
   [내려받기 안내](https://www.postgresql.org/download/)에서 해당 운영체제의 설치 방법을 따릅니다
   ([Linux](https://www.postgresql.org/download/linux/) · [macOS](https://www.postgresql.org/download/macosx/)). 준비가 끝나면 아래
   명령으로 역할 셋을 만듭니다. 역할별로 권한을 나누며, 이름은 예입니다.

   | 역할 | 하는 일 |
   |---|---|
   | `pqcota_schema` | 스키마를 만들고 표를 소유합니다. `pqcota-inventory -init-schema`만 이 역할로 돌립니다 |
   | `pqcota_ingest` | 적재합니다(`pqcota-ingest` 등 쓰는 명령). 표의 읽기 · 쓰기 권한만 있고 `PQCOTA_AUTO_DDL=0`으로 돌립니다 |
   | `pqcaton_ui` | 화면이 이력을 읽습니다. 읽기 권한만 있습니다 |

   관리자 계정으로 `psql`에 들어가 실행합니다. 비밀번호는 `\password`가 묻는 자리에만 입력하므로 명령줄과 셸 기록에 남지 않습니다.

   ```sql
   CREATE ROLE pqcota_schema LOGIN;
   \password pqcota_schema
   CREATE ROLE pqcota_ingest LOGIN;
   \password pqcota_ingest
   CREATE ROLE pqcaton_ui LOGIN;
   \password pqcaton_ui
   CREATE DATABASE pqcota OWNER pqcota_schema;
   REVOKE ALL ON DATABASE pqcota FROM PUBLIC;
   GRANT CONNECT ON DATABASE pqcota TO pqcota_ingest, pqcaton_ui;
   ```

   비밀번호는 연결 정보에 넣지 않고 `~/.pgpass`(0600)에 둡니다. 이 파일은 DB 서버가 있는 기계가 아니라 명령과 화면을 돌리는
   기계에, 그 명령을 실행하는 사용자의 홈에 둡니다. 한 줄에 `호스트:포트:데이터베이스:계정:비밀번호`이고, 편집기로 채웁니다.
   다른 자리에 두려면 `PGPASSFILE`로 알립니다. 그러면 연결 정보는 `postgres://pqcaton_ui@<호스트>:5432/pqcota`처럼
   비밀 없이 적을 수 있습니다.

   파일이 없을 때만 빈 파일을 만듭니다. 이미 있는 `~/.pgpass`를 `install`로 다시 만들면 내용이 지워지므로, 있으면 그대로 둡니다.

   ```bash
   [ -e ~/.pgpass ] || install -m 600 /dev/null ~/.pgpass
   ```

   계정마다 한 줄씩 적습니다. 이 기계에서 쓰는 역할의 줄만 있으면 됩니다. 예(같은 기계의 서버, `<…>`는 실제 비밀번호):

   ```text
   127.0.0.1:5432:pqcota:pqcota_schema:<스키마 역할 비밀번호>
   127.0.0.1:5432:pqcota:pqcota_ingest:<적재 역할 비밀번호>
   127.0.0.1:5432:pqcota:pqcaton_ui:<조회 역할 비밀번호>
   ```

   위에서부터 처음 맞는 줄을 씁니다. 호스트 칸은 연결 정보에 적은 값과 글자까지 같아야 합니다(`localhost`와 `127.0.0.1`은
   다른 값으로 봅니다).

2. **상류 중앙 명령.** 적재 · 조회 · 승인 · 생성 · 키 만들기 명령입니다. 상류 릴리스 묶음(`pqcota-ctl`)은 **Linux용만** 있으므로,
   이 기계에 맞는 길을 고릅니다. 어느 길이든 명령은 `~/.local/bin`에 들어가고, 이 폴더가 `PATH`에 있어야 합니다.

   **Linux(amd64 · arm64): 릴리스 묶음.** 체크섬과 출처 증명(상류 리포 · 릴리스 워크플로 · 태그에 고정)을 확인한 뒤 풉니다.
   `gh`(2.49 이상, 로그인)가 필요합니다. arm64 기계면 `linux-amd64`를 `linux-arm64`로 바꿉니다.

   ```bash
   mkdir -p ~/pqcaton/pqcota-ctl-v0.20.0 && cd ~/pqcaton/pqcota-ctl-v0.20.0
   gh release download v0.20.0 -R randyinthedev-hash/pqcota -p 'pqcota-ctl-linux-amd64.tar.gz' -p SHA256SUMS
   sha256sum -c --ignore-missing SHA256SUMS
   gh attestation verify pqcota-ctl-linux-amd64.tar.gz --repo randyinthedev-hash/pqcota \
     --signer-workflow randyinthedev-hash/pqcota/.github/workflows/release.yml --source-ref refs/tags/v0.20.0 --deny-self-hosted-runners
   tar -xzf pqcota-ctl-linux-amd64.tar.gz
   install -m 755 pqcota-ctl-linux-amd64/pqcota-* ~/.local/bin/
   ```

   **macOS(Intel · Apple Silicon): 태그에 고정한 소스 빌드.** 상류 리포 다섯 개를 v0.20.0 태그로 나란히 받아 빌드합니다. Git과
   Go가 필요합니다. 상류 명령 대부분은 `go.mod`의 `replace` 때문에 `go install …@v0.20.0`으로 설치되지 않습니다.

   ```bash
   mkdir -p ~/pqcaton/pqcota-src-v0.20.0 && cd ~/pqcaton/pqcota-src-v0.20.0
   for r in pqcota pqcota-common pqcota-inventory pqcota-discovery pqcota-provisioning; do git clone -q --branch v0.20.0 https://github.com/randyinthedev-hash/$r; done
   cd pqcota
   D=github.com/randyinthedev-hash
   go build -o ~/.local/bin/ $D/pqcota-common/cmd/... $D/pqcota-discovery/cmd/... $D/pqcota-inventory/cmd/... $D/pqcota-provisioning/cmd/...
   ```

   `pqcota-inventory -h`가 플래그 목록을 찍으면 됩니다. macOS의 Git은 클론할 때 「refs/tags/v0.20.0 … is not a commit!」 경고를
   찍을 수 있습니다. 경고가 나면 클론이 실패 없이 끝났는지(종료 상태 0) 보고, `pqcota` 폴더에서
   `for r in pqcota pqcota-common pqcota-inventory pqcota-discovery pqcota-provisioning; do git -C ../$r describe --tags --exact-match; done`이
   다섯 줄 모두 `v0.20.0`을 찍는지 확인합니다. 하나라도 다르거나 오류가 나면 설치를 멈추고 해당 리포의 상태를 확인한 뒤, 새 빈 폴더에 다시 받으십시오. 빌드 방법은 상류의
   [빌드 안내(v0.20.0)](https://github.com/randyinthedev-hash/pqcota/blob/v0.20.0/docs/build.ko.md) 「빌드」 절과 같고, Linux에서도
   이 길을 쓸 수 있습니다.

3. **이력 스키마 만들기.** 스키마는 `pqcota_schema`로 `pqcota-inventory -init-schema`를 한 번 돌려 만듭니다. 한 트랜잭션이고, 끝나면
   `schema ready: 11 created, 0 already present`처럼 알립니다. 다시 돌려도 있는 표와 데이터는 그대로 두고, 판을 올린 뒤에는 새 판이
   더한 것만 만듭니다. 그러므로 **상류 판을 올릴 때마다 한 번씩 돌립니다.** `pqcota-provision`으로 화면 밖에서 생성 기록을 남긴다면
   `pqcota-records -init-schema`도 같은 역할로 돌립니다. 상류 v0.16.0부터 조회 명령은 표를 만들지 않고, 화면도 이력을 조회용으로만
   엽니다. 스키마가 없으면 화면은 「the pqcota history schema is missing or incomplete; run `pqcota-inventory -init-schema` as the role
   that owns the schema」로 시작하지 않습니다. `<…>`를 실제 값으로 바꾼 뒤 실행하십시오.

   ```bash
   PQCOTA_DSN='postgres://pqcota_schema@<호스트>:5432/pqcota' pqcota-inventory -init-schema
   ```

4. **적재 · 화면 계정의 권한.** 스키마를 만든 뒤 관리자 계정으로 `pqcota` 데이터베이스에서 실행합니다. 앞의 세 줄은 지금 있는 표에,
   뒤의 세 줄(`ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema`)은 `pqcota_schema`가 앞으로 만드는 표에 권한을 줍니다. 뒤의 것만으로는
   이미 있는 표에 아무 권한도 생기지 않으므로 둘 다 실행합니다.

   ```sql
   GRANT USAGE ON SCHEMA public TO pqcota_ingest, pqcaton_ui;
   GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO pqcota_ingest;
   GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO pqcota_ingest;
   GRANT SELECT ON ALL TABLES IN SCHEMA public TO pqcaton_ui;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pqcota_ingest;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT USAGE ON SEQUENCES TO pqcota_ingest;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT SELECT ON TABLES TO pqcaton_ui;
   ```

   - **적재 계정은 `PQCOTA_AUTO_DDL=0`으로 돌립니다.** 이 값이 없으면 쓰는 명령이 저장소를 열 때마다 스키마 문장을 돌리는데, 그것은
     표의 소유자만 할 수 있습니다. 표를 소유하지 않은 적재 계정은 「permission denied for schema public」으로 실패합니다.
   - **화면 계정은 이력 표 전부의 `SELECT`와 스키마의 `USAGE`만 있으면 됩니다.** 화면은 시작할 때 이력 저장소의 모든 표와 열을
     확인합니다. 권한이 빠지면 「this account lacks privileges to read the pqcota history (SELECT on its tables, USAGE on their
     schema); initializing the schema does not change them」로 시작하지 않습니다. 초기화로는 고쳐지지 않으므로 권한을 줍니다.
   - 역할과 권한의 자세한 표는 상류의 [setting up the database(v0.20.0)](https://github.com/randyinthedev-hash/pqcota-inventory/blob/v0.20.0/cmd/README.md#setting-up-the-database)에 있습니다.

5. **화면 연결 정보.** 화면 계정의 연결 정보를 `pqcota.env`에 한 줄로 둡니다. 비밀번호가 셸 기록에 남지 않도록 명령줄에 값을
   쓰지 않습니다. 아래 명령으로 빈 파일(0600)을 만든 뒤, 편집기로 `PQCOTA_DSN=postgres://pqcaton_ui@<호스트>:5432/pqcota` 한 줄을
   넣습니다. 파일 규칙은 아래 「pqcota 연결」에 있습니다.

   ```bash
   mkdir -p ~/.config/pqcaton && chmod 700 ~/.config/pqcaton
   install -m 600 /dev/null ~/.config/pqcaton/pqcota.env
   ```

   macOS에서는 자리만 다릅니다.

   ```bash
   mkdir -p "$HOME/Library/Application Support/pqcaton" && chmod 700 "$HOME/Library/Application Support/pqcaton"
   install -m 600 /dev/null "$HOME/Library/Application Support/pqcaton/pqcota.env"
   ```

이제 [4절](#4-시작과-끝내기)대로 `pqcaton-ui -no-browser`를 실행합니다. `screen: http://127.0.0.1:8765/  (workspace …)` 줄이
나오면 화면이 뜬 것입니다. 시작하지 않으면 `❌` 뒤의 까닭을 아래 「pqcota 연결」과 [10절 오류 표](#10-오류-표)에서 찾습니다.

**pqcota 연결.** pqcaton은 pqcota가 설치 · 연결되어 동작하는 환경에서만 실행합니다. 화면은 시작할 때 pqcota 이력에
연결하고, 이력을 한 번 읽어(스키마와 읽기 권한) 통과해야 뜹니다. 연결 정보가 없거나 연결 · 조회가 10초 안에 끝나지 않으면
포트를 열거나 주소 파일을 쓰기 전에 끝납니다. 조회만 하므로 읽기 권한만 있는 계정이면 됩니다.

- **연결 정보는 `PQCOTA_DSN`입니다.** 환경 변수에 있으면 그것을 쓰고, 비었으면 승인 키와 같은 사용자 설정 폴더의
  `pqcota.env`를 화면이 직접 읽습니다(Linux `~/.config/pqcaton/pqcota.env`, macOS `~/Library/Application Support/pqcaton/pqcota.env`).
  서비스와 실행기도 이 파일을 그대로 씁니다.
- **파일 규칙.** 링크가 아닌 보통 파일이고, 이 사용자의 것이며, 그룹 · 다른 사용자에게 아무 권한이 없고(0600), 하드 링크가
  하나뿐이며, 작업 폴더 밖에 있어야 합니다. 하나라도 어기면 화면이 시작하지 않고 까닭만 알립니다.
- **내용은 데이터로만 읽습니다.** 빈 줄과 `#` 주석 말고는 `PQCOTA_DSN=<연결 정보>` 한 줄만 받습니다. 다른 변수 · `export` ·
  따옴표 · 앞뒤 공백 · 두 번 적기는 거절합니다. 셸로 평가하지 않으므로 값 안의 `$(…)` 같은 글자도 그대로 값입니다.
- **값은 어디에도 남기지 않습니다.** 작업 폴더 · 처리 기록 · 로그 · 오류 문구 · 화면에 없고, 화면이 띄우는 관측 프로세스에도
  넘기지 않습니다. 화면에서 입력받는 자리도 없습니다.
- **pqcota 조직은 판정 조직입니다.** 화면은 판정 조직(`-org`, 주지 않으면 선언의 조직)의 pqcota 이력만 엽니다.
  `-upstream-org`는 같은 값만 받고, 다른 조직을 주면 시작하지 않습니다. 판정 세션의 조직이 다르면 05가 조회하지 않고
  `upstream-org`로 거절합니다. 그러므로 pqcota 적재도 그 조직으로 합니다(`PQCOTA_ORG='<조직>' pqcota-ingest …`).

화면을 띄운 뒤에는 일마다 아래가 더 필요합니다. 화면이 직접 실행하는 것은 관측의 `ansible-playbook`뿐이고, 승인 서명과 플레이북
생성은 화면 안에서 합니다.

| 일 | 필요한 것 |
|---|---|
| 03 관측 | Ansible과 노드 접속 정보, 공식 수집기, 수집기 서명 키와 신뢰할 공개 키(아래 「관측 실행 준비」 · 「수집기 서명 키 준비」) |
| 관측 결과를 이력에 적재 | `pqcota-ingest`, 적재용 계정. 03의 결과는 서명을 확인(`pqcota-ingest -verify-keys`)한 뒤 판정 조직에 적재합니다(6절). 04 · 05의 대조와 05의 정책 활성화가 이 적재를 봅니다 |
| 06 프로비저닝(승인 서명 · 플레이북 생성) | 승인 키(아래 「승인 키 준비」의 `pqcota-keygen`). 화면 밖에서 서명 · 생성할 때만 `pqcota-approve` · `pqcota-provision` |
| 노드에 적용 · 되돌리기 | `pqcota-hosts`(Ansible 인벤토리 만들기)와 `ansible-playbook`(06의 안내) |
| 오래된 이력 정리 | `pqcota-prune`(골라서) |

<a id="서비스와-실행기"></a>

**Linux: 서비스와 실행기.** 서비스는 로그인할 때 화면을 띄우고(브라우저는 열지 않음), 실행기는 서비스를 시작한 뒤 화면을
브라우저로 엽니다. 서비스를 켜기 전에 위 「pqcota 준비」를 마칩니다(서비스도 그 `pqcota.env`를 읽습니다). 실행기는 데스크톱의 프로그램 목록에 「pqcaton」으로 나타납니다. 실행기는 `pqcaton-ui -open`으로 엽니다.
적힌 주소의 서버가 같은 작업 폴더의 화면이라고 답할 때까지(최대 15초) 기다린 뒤에 열기 때문에, 비정상 종료가 남긴 낡은
주소나 아직 뜨지 않은 화면을 열지 않습니다. 끝내 답이 없으면 열지 않습니다.

```bash
mkdir -p ~/.config/systemd/user ~/.local/share/applications
curl -fsSLo ~/.config/systemd/user/pqcaton-ui.service https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.33.0/packaging/linux/pqcaton-ui.service
curl -fsSLo ~/.local/share/applications/pqcaton-ui.desktop https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.33.0/packaging/linux/pqcaton-ui.desktop
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
curl -fsSLo ~/Desktop/pqcaton-ui.command https://raw.githubusercontent.com/sntsoftgit/pqcaton/v0.33.0/packaging/macos/pqcaton-ui.command
chmod +x ~/Desktop/pqcaton-ui.command
```

<a id="관측-실행-준비"></a>

**관측 실행 준비.** 03 관측 화면에서 관측을 실행하려면 이 기계에 Ansible과 수집기가, 노드에는 접속 계정이 있어야 합니다. 화면은
시작하기 전에 이것들과 서명 키(아래 「수집기 서명 키 준비」)를 점검하고, 빠진 것이 있으면 무엇을 하면 되는지 보입니다.

아래 순서로 준비합니다. 자세한 조건은 그 뒤의 항목에 있습니다.

1. **Ansible.** ansible-core(2.16.3 · 2.21.6으로 검증)를 설치해 `ansible-playbook`이 `PATH`에 있게 합니다.
2. **노드 접속.** Linux 노드에 root로, 또는 비밀번호 없는 sudo(`NOPASSWD`) 계정으로 SSH 접속이 되게 하고, 노드마다 `ssh`로 한 번
   접속해 호스트 키를 받아 둡니다.
3. **수집기.** 아래 「받은 수집기를 확인하기」대로 받고 확인합니다.
4. **서명 키.** 아래 「수집기 서명 키 준비」대로 만듭니다.

그다음 화면을 다시 띄워, 03의 점검 줄에 빠진 것이 없는지 봅니다.

- **Ansible.** `ansible-playbook`을 `PATH`에서 찾습니다. ansible-core 2.16.3과 2.21.6으로 검증했습니다. 03은 **운영자의 Ansible
  설정을 읽지 않습니다**(아래 「03이 쓰는 Ansible 설정」).
- **노드와 접속 계정.** 이번 판의 03은 **Linux 노드만** 관측합니다. 노드에는 SSH로 접속하고, 수집기는 root로 실행합니다. 접속
  계정은 둘 중 하나입니다.
  - **root로 접속.** Ansible은 접속 계정과 권한 상승 대상이 같으면 권한 상승을 건너뛰므로 sudo를 쓰지 않습니다.
  - **일반 계정으로 접속해 sudo로 root.** 그 계정은 sudo를 **비밀번호 없이**(`NOPASSWD`) 쓸 수 있어야 하고, sudo가 명령의
    입출력을 기록하지 않아야 합니다(아래 「노드 점검」).

  처음 접속하는 노드는 미리 `ssh`로 한 번 접속해 호스트 키를 받아 둡니다. 화면은 호스트 키를 검사합니다.
- **Windows 노드는 관측하지 않습니다.** 이번 판의 03은 Windows 노드에 서명 키를 넘기지 않으므로, 관측해도 그 결과가 서명 검증을
  지나지 못합니다. 그래서 접근 정보에 Windows 노드가 있으면 03은 그 노드를 보이며 관측을 시작하지 않습니다. Windows 노드는
  접근 정보에서 빼고, 그 노드는 `pqcota-cngscan` · `pqcota-jvmscan`을 서명 키와 함께 직접 돌려 관측합니다.
- **수집기.** 작업 폴더의 `collectors` 폴더(또는 `-collectors <폴더>`)에 상류 릴리스의 수집기를 아래 모양으로 둡니다. 판은
  pqcota v0.20.0으로 두고(pqcaton 판마다의 조합은 [판별 표](compatibility.ko.md)), 노드에 올리는 `collector.jar`도 같은 판으로 둡니다.
  이 화면은 읽은 수집 결과를 v0.20.0 규칙 묶음(`pqcota-enrich/v4`)으로 정규화합니다. v0.12.0부터 `pqcota-jvmscan`은 같은
  애플리케이션을 돌리는 JVM을 배포본 하나로 묶어 내고, v0.13.0부터 `pqcota-nodescan` · `pqcota-netcap`은 사용자 서비스의 앱 키를
  `user@<uid>.service/<유닛>`으로, 엣지를 연결마다 그 연결을 연 애플리케이션으로 잡습니다. v0.15.0부터 모든 수집기가
  `PQCOTA_SIGN_KEY`로 결과에 서명하고, 키를 받았는데 서명하지 못하면 결과를 내지 않고 멈춥니다. v0.10.7 전의 `pqcota-jvmscan`은
  JVM이 둘 이상인 노드에서 결과를 여러 줄로 써서, 그 노드가 든 적재 요청 전체가 거절됩니다. 그 전 판의 `pqcota-jvmscan` ·
  `collector.jar`는 provider별 PQC 서비스 목록을 싣지 않아 JCA 준비도가 `unknown`이 되고, 03 · 04 화면이 그 노드를 「v0.10.6 JCA
  수집 결과임을 확인할 수 없습니다」로 알립니다(README의 「맞는 조합」).

```text
collectors/
├── linux-amd64/     pqcota-nodescan · pqcota-netcap · pqcota-jvmscan
├── linux-arm64/     arm64 노드가 있을 때
└── collector.jar    없으면 실행 중인 JVM의 provider 체인은 관측하지 않습니다
```

**받은 수집기를 확인하기.** 상류 수집기는 릴리스 자산으로 받고, 받기와 확인은 준비 도구 `pqcaton-collectors`로 합니다. 이 도구는
go.mod가 고정한 상류 판(이 판은 v0.20.0)의 자산만 받고, 받은 파일을 실행하지 않습니다.

```bash
GOBIN="$HOME/.local/bin" go install github.com/sntsoftgit/pqcaton/inventory/cmd/pqcaton-collectors@v0.33.0
pqcaton-collectors fetch -arch linux-amd64 -out ~/pqcaton/collectors-v0.20.0
pqcaton-collectors verify -arch linux-amd64 -dir ~/pqcaton/collectors-v0.20.0
```

- **조건.** `gh`(GitHub CLI 2.49 이상)가 있고 로그인되어 있어야 합니다(`gh auth login`). 출처 증명을 확인하고 신뢰 루트를 받느라
  GitHub(`github.com`, 자산 내려받기 호스트)에 닿아야 합니다. `verify`도 신뢰 루트를 새로 받으므로 망이 필요합니다. 망이 없는
  기계에서 쓰려면 망이 있는 기계에서 준비 · 확인한 폴더를 옮깁니다.
- **대상(`-arch`).** 수집기를 돌릴 노드의 아키텍처를 늘 명시합니다(`linux-amd64` 또는 `linux-arm64`). 준비 폴더의
  `provenance/TARGET`에 적히고, `verify`가 다시 견줍니다. 실행 파일의 Go 빌드 정보에 적힌 OS · 아키텍처도 대상과 같아야 합니다.
  두 아키텍처의 노드가 함께 있으면 대상마다 따로 받습니다.
- **`fetch`가 하는 일.** 대상 옆의 새 임시 폴더에 자산을 받고, 자산의 출처 증명(상류 저장소 · 릴리스 워크플로 · 그 태그에 고정)을
  확인하고, `SHA256SUMS`와 견주고, 압축을 안전하게 풀고(링크 · 장치 파일 · 경로 탈출 거절), release manifest와 파일마다 해시 ·
  판 · 커밋 · 내용 해시(h1)를 견주고, 꺼낸 실행 파일의 출처 증명을 확인한 뒤, 정해진 파일만 있을 때 한 번에 옮깁니다. 하나라도
  실패하면 대상 폴더를 만들지 않습니다. 대상이 이미 있으면 덮어쓰지 않습니다.
- **쓰는 법.** 화면을 띄우기 전에 `verify`로 다시 확인하고, 화면에는 `-collectors ~/pqcaton/collectors-v0.20.0/collectors`로
  줍니다.

<a id="수집기-서명-키-준비"></a>

**수집기 서명 키 준비.** 03은 수집기 결과에 서명하게 하고, 서명이 확인된 결과만 지금 결과로 씁니다. 상류 적재
(`pqcota-ingest`)도 같은 키로 확인합니다. 그래서 관측은 **수집기 ID마다 서명 키가 있어야 시작합니다.**

`pqcota-keygen`은 「pqcota 준비」의 `pqcota-ctl` 묶음에 있습니다. 셋째 줄은 세 키의 공개 키를 `verify.pub` 한 줄로 모읍니다. 이 줄도
umask 077 안에서 실행하므로 파일은 만들어지는 순간부터 그룹과 다른 사용자가 고칠 수 없습니다. 기본 umask가 002인 배포판에서
그냥 만들면 그룹이 고칠 수 있는 파일이 되고, 03은 그것을 받지 않습니다. 마지막 `chmod`는 다른 사용자에게 읽기만 엽니다. 0600으로 두어도
03은 받습니다.

```bash
mkdir -p ~/.config/pqcaton && mkdir -m 700 ~/.config/pqcaton/collector-keys
for id in openssl-collector network-collector jvm-collector; do (umask 077 && pqcota-keygen -collector "$id" > ~/.config/pqcaton/collector-keys/"$id".key); done
(umask 077 && for id in openssl-collector network-collector jvm-collector; do sed -n 's/^PQCOTA_VERIFY_KEY=//p' ~/.config/pqcaton/collector-keys/"$id".key; done | paste -sd, - | sed 's/^/PQCOTA_VERIFY_KEY=/' > ~/.config/pqcaton/collector-keys/verify.pub)
chmod 644 ~/.config/pqcaton/collector-keys/verify.pub
```

macOS에서는 자리가 `~/Library/Application Support/pqcaton/collector-keys`입니다. 위 명령의 `~/.config/pqcaton`을 그 자리로 바꿉니다.
03 밖에서 운영자가 직접 돌리는 수집기도 같은 ID의 키로 서명해야 적재에서 거절되지 않습니다. 키를 바꿀 때는 새 공개 키를
`verify.pub`과 적재 쪽 값에 먼저 더하고(한 ID 아래 둘을 둘 수 있습니다), 서명 키 파일을 바꾸고, 관측과 `pqcota-ingest -verify-keys`로 확인한
뒤 옛 공개 키를 뺍니다.

- **수집기 ID와 키.** 결과에 적히는 ID는 명령마다 정해져 있습니다. `pqcota-nodescan`은 `openssl-collector`, `pqcota-netcap`은
  `network-collector`, `pqcota-jvmscan`은 `jvm-collector`입니다. `collector.jar`가 없으면 `jvm-collector` 키는 없어도 됩니다.
  키는 ID마다 하나이고, **같은 ID의 수집기는 모든 노드가 그 키를 함께 씁니다.** 상류는 아직 키를 노드에 묶지 않으므로, 한 ID의 키를
  가진 쪽은 그 ID로 어느 노드의 결과든 서명할 수 있습니다. 노드를 가리는 것은 등록 목록과 머신 지문 검사입니다.
- **자리.** 서명 키는 사용자 설정 폴더의 `pqcaton/collector-keys/<ID>.key`이고, 신뢰할 공개 키는 같은 폴더의 `verify.pub`입니다.
  폴더는 작업 폴더 밖에 두고 소유자만 들어가게(0700) 하며, 키 파일은 소유자만 읽게(0600) 합니다. 승인 키(06)와 자리 · 용도가
  다릅니다. 다른 자리에 두려면 `-collector-keys <폴더>`와 `-collector-verify-keys <파일>`로 줍니다.
- **신뢰할 공개 키(`verify.pub`).** `PQCOTA_VERIFY_KEY=<ID>:<공개 키>,…` 한 줄이고, **적재에 주는 `PQCOTA_VERIFY_KEY`와 같은
  값입니다.** 03은 결과를 이 값으로만 검증합니다(개인 키에서 뽑아 쓰지 않습니다). 다른 사용자가 고칠 수 있거나 개인 키가 든 파일,
  ID에 묶이지 않은 옛 목록 꼴은 받지 않습니다.
- **03이 막는 것.** 빠진 ID, 파일 이름과 다른 ID의 키, 수집기 ID가 없는 키(플래그 없이 만든 승인 키 꼴), 넓은 권한, `verify.pub`에
  없는 서명 키, 한 공개 키를 두 ID에 둔 것, 승인 키를 서명 키로 쓴 것. 03의 「수집기 서명 키」 · 「신뢰할 수집기 공개 키」 줄이
  빠진 ID와 까닭을 보입니다.

**두 확인이 보는 것.** 배포물 검증(`pqcaton-collectors`)은 **이 파일이 상류의 그 공식 릴리스가 낸 그 파일인가**를 봅니다: 누가 어느
태그에서 만들었는지(출처 증명)와 받은 바이트가 게시된 해시와 같은지입니다. 03 화면의 판 확인은 관측 직전에 **이 파일이 이 판이
요구하는 판의 그 빌드인가**를 봅니다: Go 빌드 정보의 판과 내용 해시(h1), JAR의 판 · 커밋 · 수정 여부입니다. 화면은 망을 쓰지 않고
출처를 보지 않으므로, 배포물 검증을 대신하지 못합니다. 둘을 함께 씁니다.

**수집기의 판.** 03은 관측을 시작하기 전에 수집기마다 판을 봅니다. 수집기를 실행하지 않고, Go 빌드 정보만 읽습니다.
「확인됨」은 수집기 자신이 고정 판이고, 빌드 정보에 실린 주 모듈의 내용 해시(h1)가 이 판이 고정한 값(리포의 `go.sum`)과
같으며, 의존 모듈에도 replace가 없는 빌드뿐입니다.

**상류 v0.20.0의 릴리스 자산은 「확인됨」입니다.** v0.14.0부터 상류 릴리스는 형제 모듈을 옆 폴더가 아니라 같은 태그의 모듈로
받아 빌드하므로, 빌드 정보에 수집기 자신과 의존 모듈이 그 판으로 남고 replace가 없습니다. 옆 폴더를 두고 직접 빌드한 것(상류
데모 방식)은 지금도 「확인 불가」입니다. 수집기 자신을 replace로 빌드했으면 「실제 소스: 로컬 replace(devel)」로, 태그
체크아웃에서 빌드했으면 「수집기 소스 v0.20.0 확인 · replace 의존 소스 확인 불가」로 보입니다. 직접 빌드한 것은 빌드 정보에
주 모듈의 h1이 없어, replace가 없어도 「모듈 내용 해시(h1) 없음(직접 빌드)」으로 확인 불가입니다.

- **맞지 않음.** 다른 판이 보이거나, 수집기끼리 판이 섞였거나, **판은 같아도 내용 해시(h1)가 고정한 값과 다르면**(같은 판
  이름의 다른 내용) 관측을 시작하지 않습니다. 이것은 어떤 옵션으로도 넘길 수 없습니다. 그 판의 상류 릴리스 자산으로 바꿉니다.
- **확인 불가.** 판을 믿을 수 없는 수집기도 기본으로는 시작하지 않습니다. 상류 데모처럼 다른 리포에서 replace로 빌드한
  것, `go build -buildvcs=false`로 빌드한 것, 고친 트리에서 빌드한 것, 빌드 정보가 없는 파일이 그렇습니다. 알고 쓰려면 화면을
  `-allow-unverified-collectors`로 띄웁니다. 허용한 사실과 파일 목록은 03 화면과 표준 오류에 남고, 작업 폴더에는 남지
  않습니다.
- **`collector.jar`.** 없어도 됩니다(선택 항목이라 이 점검이 없습니다). 두면 **v0.30.0부터 수집기와 같은 기준으로 엄격하게
  봅니다.** v0.14.0부터 공식 JAR에는 manifest에 판(`Implementation-Version`) · 커밋(`X-Pqcota-Commit`) · 수정 여부
  (`X-Pqcota-Dirty`)가 실려 있습니다.
  - **확인됨**: 정식 판이 고정 판이고, 커밋이 40자 · 64자 16진수이며, 수정 여부가 `false`입니다. 03의 collector.jar 줄에 판과
    커밋 앞 12자가 보이고, 전체 커밋은 그 줄의 설명(마우스를 올리면 보이는 `title`)에 있습니다.
  - **맞지 않음**: 정식 판이 다릅니다. 어떤 옵션으로도 넘길 수 없습니다.
  - **확인 불가**: 판 정보가 없음, 상류를 직접 빌드한 판(`devel`, `git describe` 꼴), 고친 트리(`X-Pqcota-Dirty: true`), 커밋 ·
    수정 여부가 없거나 꼴이 틀림. 기본으로 시작하지 않고, `-allow-unverified-collectors`로 허용하면 경고로 바뀝니다. 확인 불가인
    JAR의 판은 수집기끼리 판이 섞였는지 셀 때 넣지 않습니다.

  **v0.29.x까지는 판 정보가 없는 JAR에 경고만 했습니다.** 직접 빌드한 JAR을 계속 쓰려면 이제 override가 필요합니다. 기본
  해결책은 상류 릴리스의 `collector.jar`로 바꾸는 것입니다. manifest의 값은 화면에 옮길 때 64자에서 자릅니다.

<a id="03이-쓰는-ansible-설정"></a>

**03이 쓰는 Ansible 설정.** 03은 서명 키를 노드 표준 입력으로 넘깁니다. 그 길에 다른 설정이 끼면 키가 샐 수 있어서(운영자 폴더의
콜백 플러그인이 키를 받거나, 다른 sudo 실행 파일이 키를 받는 것을 실험으로 확인했습니다), 검증한 조건만 씁니다.

- 실행마다 제품이 만든 설정 파일 하나만 읽습니다. **운영자의 `ansible.cfg`(`./ansible.cfg` · `~/.ansible.cfg` ·
  `/etc/ansible/ansible.cfg`)와 `ANSIBLE_*` 환경 변수는 03 관측에 쓰이지 않습니다.** 운영자 폴더의 플러그인 · 컬렉션도 읽지 않습니다.
- 연결은 내장 ssh, 권한 상승은 내장 sudo(`sudo -H -S -n`, root), pipelining은 켜고, 원격 임시 파일은 남기지 않습니다.
- ssh 클라이언트 설정(`~/.ssh/config`의 `ProxyJump` · `IdentityFile` · `Port` 등)과 `~/.ssh/known_hosts`는 ssh가 읽으므로 그대로
  씁니다. ssh-agent(`SSH_AUTH_SOCK`)도 그대로 씁니다.
- 기존 `ansible.cfg`에 두던 값을 옮기는 방법은 [9절 「v0.30.0에서 v0.31.0으로」](#v0300에서-v0310으로)에 있습니다.

<a id="노드-점검"></a>

**노드 점검.** 관측을 시작하면 03은 먼저 **키 없이** 모든 노드를 점검합니다. 노드마다 접속 계정의 uid와, root가 아니면
`LC_ALL=C sudo -n -l`을 읽습니다. 모든 노드가 통과해야 키를 만들고 관측을 시작합니다. 한 노드라도 통과하지 못하면 아무 노드도
관측하지 않고, 그 실행은 「실패」이며, 노드 정리는 「불필요」로 끝납니다(관측이 노드에 닿지 않았고 점검은 노드에 아무것도 만들지
않습니다). 실행 로그에 노드 · 까닭 · 그 정책 줄 · 노드에서 확인할 명령이 남습니다.

| 판정 | 언제 |
|---|---|
| 통과 | root로 접속함, 또는 `sudo -n -l`의 지원하는 출력 꼴에서 입출력 기록 설정을 **찾지 못함**. 이 점검이 보지 못하는 기록은 아래 「신뢰 전제」 |
| 거절 | 입출력 기록이 켜짐(`log_input` · `log_output` · `log_stdin` · `log_stdout` · `log_stderr` · `log_ttyin` · `log_ttyout`), 규칙의 `LOG_INPUT:` · `LOG_OUTPUT:`, `requiretty`. 키가 그 기록에 남기 때문입니다 |
| 확인 불가 | sudo가 비밀번호를 물음(`NOPASSWD`가 아님), `sudo -n -l`이 실패함, 출력 꼴이 다름, 켜는 설정과 끄는 설정(`!log_input` · `NOLOG_INPUT:`)이 섞임, 명령별 · runas별 설정이 있음, root로 모든 명령을 실행하는 규칙 말고 다른 규칙이 있음. 실제로 어느 설정이 `/bin/sh`에 적용되는지 이 목록만으로 가릴 수 없어 멈춥니다 |

확인 불가는 노드의 sudoers를 단순하게 바꾸면 풀립니다. 그 접속 계정에 `<계정> ALL=(ALL) NOPASSWD: ALL` 규칙만 두고, 입출력 기록을
그 계정에서 끕니다. 노드에서 그 계정으로 `LC_ALL=C sudo -n -l`을 실행해, 03의 로그에 보인 정책 줄을 직접 확인합니다.

<a id="승인-키-준비"></a>

**승인 키 준비.** 06 화면에서 새 승인에 서명하려면 승인 키가 필요합니다. 아래 명령은 기본 자리(Linux
`~/.config/pqcaton/`, macOS `~/Library/Application Support/pqcaton/`)에 승인 키 `approval.key`와 공개 키 `approval.pub`을 만듭니다.
그 자리에 두면 플래그 없이 씁니다. 두 파일 모두 umask 077 안에서 만들므로, 만들어지는 순간부터 그룹과 다른 사용자가 고칠 수
없습니다(06은 그룹이 고칠 수 있는 공개 키 파일을 받지 않습니다). 마지막 `chmod`는 다른 사용자에게 읽기만 엽니다. 0600으로 두어도
06은 받습니다.

```bash
GOBIN="$HOME/.local/bin" go install github.com/randyinthedev-hash/pqcota-common/cmd/pqcota-keygen@v0.20.0
mkdir -p ~/.config/pqcaton
(umask 077 && pqcota-keygen > ~/.config/pqcaton/approval.key)
(umask 077 && grep '^PQCOTA_VERIFY_KEY=' ~/.config/pqcaton/approval.key > ~/.config/pqcaton/approval.pub)
chmod 644 ~/.config/pqcaton/approval.pub
```

macOS에서는 자리만 다릅니다.

```bash
mkdir -p "$HOME/Library/Application Support/pqcaton"
(umask 077 && pqcota-keygen > "$HOME/Library/Application Support/pqcaton/approval.key")
(umask 077 && grep '^PQCOTA_VERIFY_KEY=' "$HOME/Library/Application Support/pqcaton/approval.key" > "$HOME/Library/Application Support/pqcaton/approval.pub")
chmod 644 "$HOME/Library/Application Support/pqcaton/approval.pub"
```

화면을 다시 띄우면 시작 줄에 `approval key: <경로> (default location)`이 나옵니다. `warning:`으로 시작하는 줄이 나오면 아래 「잘못된 키」를 봅니다.

- **키 파일.** `pqcota-keygen`을 **플래그 없이** 실행한 출력입니다. `pqcota-keygen -collector`로 만든 수집기 서명 키는 승인 키로
  받지 않습니다(「collector」 상태). `pqcota-keygen`은 `pqcota-ctl` 묶음에도 있습니다. 화면은 이 파일을 만들거나 권한을 고치지
  않고, 서명할 때마다 다시 읽습니다.
- **확인용 공개 키(`approval.pub`).** 개인 키 없이도 이미 서명된 계획을 확인하고 플레이북을 만들 수 있게 합니다. 개인 키 줄이
  든 파일이나 다른 사용자가 고칠 수 있는 파일은 받지 않습니다. 두 파일이 모두 있으면 같은 키여야 하고, 다르면 서명과 확인을
  모두 막습니다.
- **잘못된 키.** 기본 위치의 파일이 없으면 화면은 정상으로 뜨고 새 서명만 하지 않습니다. 있는데 권한 · 소유자 · 링크 · 형식이
  잘못됐으면 화면은 뜨지만 06과 표준 오류에 까닭을 보이고 새 서명을 막습니다. 06에는 사용자 설정 폴더 기준의 위치와 키
  지문만 보이고, 개인 키는 어디에도 보이지 않습니다.
- **다른 자리에 둘 때.** `-approval-key <파일>`과 `-approval-verify-key <파일>`로 줍니다. 명시한 파일이 잘못됐거나, 둘 다
  명시했는데 서로 다른 키면 화면을 시작하지 않습니다. 기본 위치를 찾지 않게 하려면 `-no-default-approval-key`로 띄웁니다. 이
  옵션은 기본 위치의 두 파일을 모두 끄고, 명시한 파일은 그래도 씁니다.
- **고르는 순서.** 명시한 개인 키, 명시한 공개 키, 기본 위치의 개인 키, 기본 위치의 공개 키 순서입니다. **하나라도 명시하면
  기본 위치는 둘 다 읽지 않습니다.** 개인 키만 있으면 그 키에서 공개 키를 뽑아 서명 · 확인 · 생성을 모두 하고, 공개 키만
  있으면 확인 · 생성만 합니다. 개인 키가 잘못됐고 공개 키가 정상이면 서명만 막고 확인 · 생성은 합니다. 개인 키는 정상인데
  함께 고른 공개 키를 쓸 수 없거나 다른 키면 구성 오류로 서명과 생성을 모두 막습니다.

서비스로 띄우면서 기본 위치가 아닌 키를 쓴다면 서비스에 플래그를 더합니다. 기본 위치에 두었다면 필요 없습니다.

```bash
mkdir -p ~/.config/systemd/user/pqcaton-ui.service.d
printf '[Service]\nExecStart=\nExecStart=%%h/.local/bin/pqcaton-ui -no-browser -approval-key %%h/secrets/pqcaton-approval.key\n' > ~/.config/systemd/user/pqcaton-ui.service.d/approval.conf
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
| `asset-scope.csv` | 02에서 확정한 자산 스코프 정책. 다음 검토의 기준이고, 활성화하기 전에는 대조에 쓰지 않습니다 | 화면(02 확정) |
| `scope-assets.csv` | 활성 정책. 04 · 05가 대조에 쓰며, pqcota 적재(`pqcota-ingest -scope-assets`)에 준 것과 같아야 합니다 | 운영자 · 화면(새 정책 활성화) |
| `scope-assets.activation-<지문>-<세션 ID>.csv` | 새 정책을 활성화할 때 보관한 옛 활성 정책 | 화면(새 정책 활성화) |
| `session.scope-activation-<세션 ID>.json` · `session.scope-baseline-<세션 ID>.json` | 정책 회차를 열 때 보관한 옛 판정 세션. 옆의 `…-report-…json`이 변환 보고서입니다 | 화면(정책 회차) |
| `plan.json` | 확정한 계획(pqcota가 읽는 계약 형식) | 화면(05 확정) |
| `plan.signed.json` | 승인 서명한 계획(FINALIZED). `plan.json`은 그대로 남습니다. 있으면 덮지 않습니다 | 화면(06 승인) |
| `plan.provision.yml` · `plan.rollback.yml` | 적용 · 되돌리기 플레이북 | 화면(06 생성) |
| `plan.signed.<시각>.json` 등 | 「지난 서명 보관」으로 이름을 바꿔 둔 지난 서명과 플레이북 | 화면(06 보관) |
| `judgments.jsonl` | 판정 원장. 덧붙이기만 합니다 | 화면(확정) |
| `.pqcaton/` | 저장 기록 · 처리 기록 · 잠금 · 떠 있는 화면의 주소(`screen.addr`) | 화면 |
| `.pqcaton.lock` | 이 폴더가 어느 상태 폴더에 묶였는지 적은 잠금 파일. `.pqcaton/`과 짝을 이룹니다 | 화면 |

**조직.** `-org`를 주지 않으면 판정은 선언에 적힌 조직에 묶입니다. `-org`를 주면 그 조직의 선언만 만들 수 있습니다.

## 4. 시작과 끝내기

직접 띄울 때는 터미널에서 실행합니다. 연결 정보는 2절 「pqcota 연결」의 `pqcota.env`에서 읽으므로 셸에 따로 넣지 않아도
됩니다. 포트를 연 뒤 브라우저를 엽니다(`http://127.0.0.1:8765`). 브라우저를 열지 않으려면 `-no-browser`를 줍니다.

<!-- run: background -->
```bash
pqcaton-ui -no-browser
```

뜨면 표준 오류에 `screen: http://127.0.0.1:8765/  (workspace <작업 폴더>)` 줄이 나옵니다. 시작하지 않으면 `❌` 뒤의 까닭을
[10절 오류 표](#10-오류-표)에서 찾습니다.

지금 조직, 파일 경로, 연결 정보의 출처, 키의 상태를 확인하려면 화면 상단의 「설정」을 여십시오.

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
   적고 저장합니다. 앞뒤가 맞지 않는 자리(겹친 IP · 포트 0 같은 것)는 화면이 칸 단위로 짚습니다. 저장은 막지
   않지만 그대로 두면 대조 결과가 오류 없이 틀립니다. 이름이나 IP를 비운 줄, 주소가 아닌 IP, 숫자가 아닌 포트는 저장하지 않고 그
   칸을 짚습니다. 적은 값은 화면에 남습니다. 노드나 자산을 지울 때는 「제거」를 누릅니다.
2. **02 자산 스코프.** 처음이면 계층 이름을 넣어 빈 계층을 만듭니다. 규칙을 적고, 바뀐 규칙마다 결론을 적고, 서명해
   확정합니다. 확정하면 `asset-scope.csv`가 생깁니다.

   **02에서 확정한 정책은 바로 대조에 쓰이지 않습니다.** 대조(04 · 05)는 활성 정책 `scope-assets.csv`를 씁니다. 대조
   정책이 상류 적재에 쓴 정책과 다르면 05에서 만든 계획의 근거를 pqcota가 찾지 못하기 때문입니다. 확정한 정책을 쓰려면
   먼저 그 정책으로 상류 적재를 다시 합니다. 결과 폴더는 작업 폴더의 `results/`를 줍니다. 순서는 셋입니다.

   1. **03이 서명을 확인합니다.** 03이 받은 결과는 수집기 서명이 있고, 03은 서명이 확인된 결과만 `results/`에 둡니다(3번).
   2. **`pqcota-ingest -verify-keys`로 다시 확인합니다.** 적재와 같은 판정으로 확인하고 아무것도 저장하지 않습니다. 결과마다 수집기 ID · 서명 여부 ·
      맞은 키의 지문 · 판정을 찍고, 거절될 결과가 있으면 종료 코드 1입니다. 그때는 적재하지 않습니다.
   3. **판정 조직에 적재합니다.** 같은 `PQCOTA_VERIFY_KEY`로 적재하므로 서명이 맞지 않는 결과는 저장 전에 거절됩니다.

   `PQCOTA_VERIFY_KEY`는 03이 신뢰하는 `verify.pub`의 값입니다(공개 키라 비밀이 아닙니다). 연결 정보는 적재용 계정의 것을
   `PQCOTA_DSN`으로 주고, 비밀번호는 「pqcota 준비」의 `~/.pgpass`에서 읽습니다. 적재 계정은 `PQCOTA_AUTO_DDL=0`으로 돌립니다.
   `<…>`를 실제 값으로 바꾼 뒤 실행하십시오.

   ```bash
   export PQCOTA_VERIFY_KEY="$(sed -n 's/^PQCOTA_VERIFY_KEY=//p' ~/.config/pqcaton/collector-keys/verify.pub)"
   pqcota-ingest -verify-keys '<작업 폴더>/results'
   PQCOTA_DSN='postgres://pqcota_ingest@<호스트>:5432/pqcota' PQCOTA_ORG='<판정 조직>' PQCOTA_AUTO_DDL=0 pqcota-ingest -scope-assets '<작업 폴더>/asset-scope.csv' '<작업 폴더>/results'
   ```

   05의 「pqcota 재적재」 단계도 같은 두 명령(작업 폴더 기준 상대 경로)을 보입니다.

   02를 확정하면 확정 알림 아래 「다음 단계」가 「04에서 현재 대조 보기」와 「05에서 활성화 절차 보기」로 잇고, 활성화하기
   전까지는 04 위쪽에도 「02에서 확정한 정책이 아직 활성화되지 않았습니다」가 떠서 그 화면의 대조가 활성 정책으로 한 것임을
   알립니다. 05 위쪽의 「02에서 확정한 정책을 활성화할 수 있습니다」는 pqcota 재적재 → pqcota 재적재 결과 대조 → 정책
   활성화의 세 단계로 보입니다. 위 명령으로 재적재한 뒤 「활성화 전 확인으로」를 누르고, 확인 페이지를 거쳐 활성화합니다.

   **활성화는 언제나 pqcota 이력 대조를 거칩니다.** 운영자 진술로 대신하는 길은 없습니다. pqcaton은 활성화 요청 하나에서
   다음 순서로만 진행합니다.

   1. 활성화할 정책과 작업 폴더 입력(판정 세션 · 선언 · 결과 폴더 · 활성 정책)의 스냅샷을 읽습니다.
   2. pqcota 이력에서 노드마다 최신 적재 결과가 1의 입력과 새 정책으로 만든 결과와 같은지 대조합니다. 결과 폴더에 있는
      노드는 새 정책에서 관리 자산이 0개가 되어도 모두 대조합니다. 정책이 노드의 자산을 모두 빼는 것도 정책 변경의
      결과이기 때문입니다.
   3. 하나라도 없거나 다르거나 조회할 수 없으면 활성화하지 않습니다. 대조 전체에 10초 제한 하나가 있고, 넘으면 대조
      실패입니다.
   4. 대조가 성공하면 곧바로 작업 폴더의 실행 잠금을 잡습니다.
   5. 1의 입력 스냅샷이 그대로인지 다시 확인합니다.
   6. 그대로일 때만 활성 정책 · 새 회차 · 변환 보고서 · 회차 표지 · 활성화 처리 기록을 한 트랜잭션으로 씁니다.

   조회는 작업 폴더 잠금 밖에서 합니다. pqcota가 선택 사항이라는 뜻이 아니라, 네트워크 조회가 느릴 때 잠금을 오래 쥐지
   않으려는 순서입니다. 확인 페이지도 미리 대조해 「확인 시점에 pqcota 이력과 일치했습니다」와 노드 수를 보이지만, 활성화를
   누르면 1부터 다시 합니다. **대조가 끝난 뒤부터 활성화가 끝날 때까지의 pqcota 변경은 감지하지 못합니다.** 5의 재확인은
   작업 폴더 입력만 봅니다. 그래서 화면과 처리 기록은 「확인 시점에 pqcota 이력과 일치했다」고만 말하며, 지금도 일치한다거나
   적재를 증명한다고 하지 않습니다. pqcota가 안정된 현재 위치 표시나 적재 기록을 주기 전에는 이보다 강하게 보장할 수
   없습니다.

   활성화 처리 기록(`.pqcaton/`의 `scope-activation`)은 근거가 `upstream-verified`로 남고, 확인 시각 · pqcota 조직 · 대조한
   노드별 지문은 변환 보고서의 `upstream_check`에 남습니다. 둘 다 비밀값이 없습니다. 명령으로도 같은 대조로 활성화할 수
   있습니다(화면을 끄지 않아도 되지만, 같은 작업 폴더를 쓰므로 화면이 다른 저장을 하는 동안에는 기다립니다). 이력은 읽기만
   하므로 화면용 계정의 연결 정보를 주고, 비밀번호는 `~/.pgpass`에서 읽습니다. `<…>`를 실제 값으로 바꾼 뒤 실행하십시오.

   ```bash
   PQCOTA_DSN='postgres://pqcaton_ui@<호스트>:5432/pqcota' pqcaton-decide activate-scope '<작업 폴더>/session.json' \
     -decl '<작업 폴더>/declaration.json' -results '<작업 폴더>/results' -scope-assets '<작업 폴더>/scope-assets.csv' \
     -candidate '<작업 폴더>/asset-scope.csv' -scope-session '<작업 폴더>/scope-session.json' -org '<조직>'
   ```

   `pqcaton-decide activate-scope -verify-upstream`의 플래그는 호환을 위해 받지만 아무것도 바꾸지 않습니다.
   `pqcaton-decide activate-scope -confirm-ingest`는 지원을 끝냈고, 주면 아무것도 쓰지 않고 멈춥니다.

   활성화하면 옛 정책과 옛 판정 세션이 보관되고, 새 정책으로 다시 세운 새 회차가 열립니다. 지난 결론은 「지난 판정」으로만
   옮겨지고, 새 정책에서 빠진 항목은 변환 보고서에만 남습니다. 이미 확정한 계획은 06에서 서명 · 생성되지 않으므로 새
   회차에서 다시 확정합니다. **활성화 기록 없이 `scope-assets.csv`를 손으로 바꾸면** 05가 잠기고 저장 · 확정이 막힙니다.
   그때는 이전 정책 파일을 복원하거나 위 절차를 밟습니다.

   **이전 정책으로 되돌리는 활성화는 그 정책으로 결과를 다시 적재한 뒤에 합니다.** 이전 정책으로 만든 결과가 pqcota 이력에
   있어도 노드의 최신 적재 결과가 다른 정책의 것이면 대조가 통과하지 않고, 05는 `upstream-revert`로 거절합니다. 그 정책으로
   결과를 다시 적재하면 마지막에 적재한 상태가 노드의 최신이 되어 활성화할 수 있습니다(pqcota v0.13.1 이상). v0.27.x 이하가 운영자 진술(`operator-statement`)로 연
   회차는 과거 기록으로 계속 읽히며, 05의 「이 회차를 연 정책 변경」에 「pqcota 이력과 대조하지 않았습니다」로 보입니다.
3. **03 관측.** 관측할 노드와 접속 방법(IP · 포트 · 계정 · SSH 키 경로 · OS)을 적고 저장합니다. Windows 노드가 있으면 관측을
   시작하지 않습니다(2절 「관측 실행 준비」). 작업 폴더의
   `hosts.csv`에 `pqcota-hosts`가 읽는 형식으로 저장됩니다. 비밀번호는 화면에서 적지 않습니다. 파일에 있으면 「있음」으로만
   보이고 저장해도 그대로 남습니다. 「관측 시작」을 누르면 다음 순서로 돕니다. 실행하는 동안 로그가 보이고 「중지」로 멈출 수
   있습니다.

   1. **노드 점검.** 키 없이 모든 노드를 점검합니다(2절 「노드 점검」). 하나라도 통과하지 못하면 여기서 끝나고, 아무 노드도
      관측하지 않으며, 노드 정리는 「불필요」입니다.
   2. **임시 서명 키.** 원래 서명 키(`collector-keys/`)를 다시 점검하고, 이 실행에만 쓸 임시 키를 **작업 폴더 밖**에 만듭니다.
      자리는 `XDG_RUNTIME_DIR`(이 사용자만 들어갈 수 있을 때) 아니면 시스템 임시 폴더 아래의 `pqcaton-keys-<실행 ID>`이고, 폴더는
      0700, 키 파일은 만들 때부터 0600입니다. 실행 로그가 그 자리를 알립니다. 자리는 실행 자리의 `run-keys.json`에 먼저 적어 두므로
      비정상 종료 뒤에도 찾을 수 있습니다. 키 값은 Ansible · ssh의 환경과 명령줄에 넣지 않고, 노드 수집기의 표준 입력으로만 넘깁니다.
   3. **관측.** 표의 노드마다 수집기를 올려 실행하고, 결과를 받은 뒤 노드에서 지웁니다. 수집기가 표준 오류로 남긴 것은 실행
      로그에 옮깁니다.
   4. **서명 확인.** 받은 결과마다 `verify.pub`으로 서명을 검증합니다(적재와 같은 판정). 통과하면 실행 로그에 「signature checked
      (key <지문>)」가 남습니다. 서명이 없거나(not signed), 다른 ID의 키로 서명했거나(그 키로 검증될 때만 그렇게 알립니다),
      등록된 키와 맞지 않으면(서명 뒤 바뀌었거나 등록되지 않은 키) 그 까닭이 「결과가 오지 않았거나 맞지 않는 자리」에 보입니다.
   5. **결과 교체.** **모든 노드의 결과가 오고, 상류 디코더로 읽히고, 노드 · 수집기가 맞고, 서명이 확인됐을 때만** 지금 결과를
      한 번에 바꿉니다. 하나라도 어긋나면 지금 결과는 그대로이고 받은 것은 `observations/<실행>/partial/`에 진단용으로 남습니다.
   6. **회수.** 임시 키를 지웁니다. 끝까지 돌지 못한 실행(실패 · 중지) 뒤에는 노드 정리를 한 번 더 돌립니다. 정리는 이 실행의
      스테이징 자리에서 아직 도는 수집기를 먼저 끝내고(sudo로 띄운 원격 수집기는 컨트롤러가 ssh를 끝내도 제 관측 구간까지 돌기
      때문입니다) 스테이징 폴더를 지웁니다.

   **임시 키를 지우지 못하면 「회수 필요」입니다.** 실행 결과(끝남 · 실패 …)와 따로 남고, 03이 그 자리와 까닭을 보이며 새 관측을
   시작하지 않습니다. 까닭을 고친 뒤(예: 그 폴더의 권한) 화면을 다시 시작하면 시작 때의 복구가 다시 지우고 「회수 필요」를 풉니다.
   지울 수 없으면 그 폴더(`pqcaton-keys-<실행 ID>`)를 직접 지운 뒤 화면을 다시 시작합니다. **화면과 감독이 함께 비정상으로 끝나면
   (전원 장애 같은 경우) 다음 시작까지 임시 키가 그 자리에 남을 수 있습니다.** 소유자만 들어갈 수 있는 폴더이고, 다음 시작의 복구가
   실행 기록으로 찾아 지웁니다. 관측 중에는 노드 수집기 프로세스의 환경에 키가 있고, 그 노드의 root가 읽을 수 있습니다. 화면을 끄면 돌던 관측도 멈추고, 정리가 끝난 뒤에 화면이 끝납니다. 관측은 화면이 띄운 감독
   프로세스(`pqcaton-ui` 자신)가 실행하므로, 화면이 강제로 끝나도 감독이 Ansible을 끝내고 노드를 정리하고 인벤토리를
   지웁니다. 감독마저 끝났으면(전원 장애 같은 경우) 다음 시작이 이 실행의 프로세스를 찾아 끝내고 같은 정리를 합니다.
   프로세스를 끝내지 못하면 아무것도 지우거나 옮기지 않고 「회수 필요」로 남기며, 그 PID를 03 화면에 보이고 새 관측을
   시작하지 않습니다. pqcaton은 실행 표지(`PQCATON_RUN`)나 살아 있는 프로세스의 계보로 이 관측의 것임을 확인한
   프로세스만 끝냅니다. 소속을 확인할 수 없는 프로세스는 관계없는 프로세스일 수 있으므로 끝내지 않고, 관측을 「회수
   필요」로 남깁니다. macOS에서는 일부 Apple 기본 프로그램(`/bin/sh` · `/usr/bin/ssh` 같은 것)의 환경을 읽을 수 없으므로,
   그런 프로세스는 실행 표지 대신 부모나 같은 프로세스 묶음에 살아 있는 이 관측의 프로세스로 확인합니다. 「회수 필요」가
   보이면 03 화면의 「남은 프로세스」를 봅니다. 화면은 남길 때 찾은 PID와 시작 시각을 지금 프로세스와 견주어, 같은 프로세스일
   때만 PID · 부모 PID · 프로세스 묶음 · 사용자 · 실행 파일 · 시작 시각을 보입니다. 인자는 접혀 있고, 펼쳐도 비밀값(비밀번호 ·
   토큰 · DSN의 사용자 정보 · 키)은 가려져 있습니다. 환경 변수는 보이지 않습니다. 「다시 확인」은 다시 읽기만 하고 아무것도
   바꾸지 않으며, 화면에는 프로세스를 끝내는 단추가 없습니다. 상태는 다섯 가지입니다.

   - **이 관측이 남긴 프로세스**: 확인한 뒤 끝낼 대상입니다.
   - **이미 종료됨**: 화면을 다시 시작하면 남은 정리를 이어서 합니다.
   - **다른 프로세스로 바뀜**: 같은 PID를 다른 프로세스가 씁니다. 끝내지 않습니다.
   - **조회 권한 없음 · 조회 실패**: 소속을 확인할 수 없으니 끝내지 않습니다. 조회 실패는 잠시 뒤 다시 확인합니다.
   - **같은 프로세스인지 확인할 수 없음**: 시작 시각을 남기지 않은 이전 판의 기록입니다. 이때만 터미널에서 직접 봅니다.
     `<…>`를 실제 값으로 바꾼 뒤 실행하십시오.

   ```bash
   ps -p '<PID>' -o pid=,ppid=,pgid=,user=,lstart=,command=
   ```

   이 관측이 띄운 프로세스임이 확인될 때만 정상 종료한 뒤 화면을 다시 시작합니다. 관계없는 프로세스이거나 소속을 확인할
   수 없으면 끝내지 말고, 그 프로세스가 스스로 끝난 뒤 화면을 다시 시작합니다. 다시 시작한 화면이 남은 정리를 이어서
   합니다. 화면이 띄운 관측은 SSH 연결 공유를 쓰지 않습니다(`ControlMaster=no`). 정리 때 노드에 닿지 못했으면 03
   화면이 남았을 수 있는 스테이징 폴더를 실행 ID와 함께 적습니다: `/tmp/pqcaton-observe-<실행 ID>`. 그 노드에서는 명령줄이 그
   자리로 시작하는 프로세스가 남지 않았는지도 봅니다. 네트워크 관측은 노드의 기본 경로 인터페이스에서 정한 구간(기본 60초) 동안
   핸드셰이크를 기다립니다.
4. **04 대조.** 관측이 끝나거나 pqcota 수집이 `results/`에 결과를 쓰면 화면을 다시 불러옵니다. 결과가 없으면 화면이 그
   폴더를 가리킵니다.

   **어느 앱이 무엇과 통신했는지 보려면** 04의 토폴로지 아래에서 「이력에서 앱별 통신 보기」를 엽니다. 04의 그림은 작업 폴더의
   결과를 선언과 대조한 것이고, 이 화면은 pqcota 이력에 적재된 관측을 그대로 보입니다. 화면 계정의 조회 권한만으로 열립니다.

   1. **조회한 조직과 비교 결과를 봅니다.** 첫머리에 조회한 조직(판정 조직)과 노드마다의 이력 스냅샷이 있고, 작업 폴더의 관측
      결과와 같은지가 「일치」 · 「다름」 · 「비교 불가」로 적혀 있습니다. 같은 노드의 스냅샷 지문으로 견준 것입니다. 「다름」이면 적재한
      뒤에 다시 관측했거나 다른 정책으로 적재한 것이므로, 지금 결과를 보려면 05의 안내대로 다시 적재합니다. 「비교 불가」는 작업
      폴더에 그 노드의 결과가 없다는 뜻입니다. 「이 조직의 pqcota 이력에 스냅샷이 없습니다」가 보이면 03의 결과를 그 조직으로 아직
      적재하지 않은 것입니다.
   2. **노드를 골라 앱과 상대를 읽습니다.** 「노드별 요약」에서 노드를 고르고 「보기」를 누르면 그 노드의 앱, 같은 앱 키로 관측된
      라이브러리, 저장된 연결 기록(방향 · 상대 · 통신 · 횟수)이 남습니다. 기록은 관측한 노드마다 하나씩 저장되므로 같은 연결을 두
      노드가 보면 두 줄입니다. 그래서 이 수는 04의 「협상된 통신」 건수와 다릅니다. 같은 앱 키의 라이브러리는 그 연결이 그것을
      썼다는 근거가 아닙니다.
   3. **그림과 DOT.** 그림은 조직 전체이고 화면이 뜬 뒤에 따로 그려집니다(04의 대조 그림도 그렇습니다). `dot`(Graphviz)이 없거나, 그리는 데 정한 시간이나
      크기를 넘으면 그림 대신 까닭이 보입니다. 표와 「DOT 내려받기」는 그와 관계없이 됩니다. 받은 파일은 다른 기계에서
      `dot -Tsvg pqcota-topology.dot -o topology.svg`로 그릴 수 있습니다.

   **상대가 주소로만 보일 때.** 이 화면은 판정 조직에 등록된 엔드포인트로만 상대를 노드로 잇습니다. 선언의 IP나 다른 조직의
   등록 정보는 쓰지 않습니다. 등록된 엔드포인트가 없으면 화면이 그렇게 알리고 모든 상대를 주소로 적습니다. 노드 이름으로 보려면
   상류 `pqcota-hosts`로 판정 조직에 엔드포인트를 등록합니다. 03이 저장한 `hosts.csv`가 그 형식입니다. 적재 계정으로
   `PQCOTA_ORG='<판정 조직>' PQCOTA_AUTO_DDL=0 pqcota-hosts --dsn 'postgres://pqcota_ingest@<호스트>:5432/pqcota' '<작업 폴더>/hosts.csv'`를
   실행합니다. 끝나면 `[hosts] upserted 3 endpoints into the inventory (organization <판정 조직>; no secrets)`처럼 등록한 수와 조직을
   알립니다. 화면을 다시 불러오면 그 상대가 노드 이름으로 보입니다. 접속 비밀은 이력에 들어가지 않고 노드 ID · 이름 · IP · 포트만 등록됩니다. 등록되는 IP는 SSH로 접속하는 주소
   하나이므로, 노드가 다른 주소로 통신했으면 그 상대는 여전히 주소로 보입니다. 한 IP를 등록 노드 여럿이 함께 쓰면 그 사실을
   적고 어느 노드로도 잇지 않습니다.
5. **05 판정.** 정책마다 결론을 적고, 계획에 넣을 항목은 조치 종류 · 목표 · 수준을 고르고, 서명합니다. 「확정」을 누르면
   확인 페이지가 무엇이 쓰이는지 보이고, 확인하면 세션 · 계획 · 원장을 한 번에 씁니다. 같은 세션을 같은 내용으로 다시
   확정해도 다시 쓰지 않습니다.
6. **06 프로비저닝.** 승인 서명과 플레이북 생성을 하는 화면입니다. 승인할 계획의 조치를 확인하고, 승인자 이름과 검토 확인을 넣어 「승인 서명」을 누릅니다. 확인
   페이지가 계획 · 계획 파일 해시 · 승인자 · 키 지문을 보이고, 거기서 한 번 더 눌러야 서명합니다. 확인한 뒤에 계획이나
   키가 바뀌었으면 서명하지 않습니다. 서명한 계획은 `plan.signed.json`(FINALIZED)에 따로 남고 `plan.json`은 그대로입니다.
   **이미 있는 서명 파일은 어떤 경우에도 덮지 않습니다.** 지금 계획과 맞지 않으면 「지난 서명 보관」으로 이름에 시각을 붙여
   옆에 둔 뒤 새로 서명합니다. 이어서 기본 위임 수준을 골라 「플레이북 생성」을 누르면 `plan.provision.yml`과
   `plan.rollback.yml`이 생깁니다. 생성기가 짚은 빈 곳이 있으면 그 수와 내용이 보입니다.
   L2로 생성했고 설정 파일만 놓는 플레이북이면 「N개 노드 미리 보기」로 대상 노드에 `--check`를 실행해 무엇이 바뀔지
   확인할 수 있습니다. 미리 보기는 노드를 바꾸지 않고, 접근 정보(`hosts.csv`)에 있는 노드에만 실행합니다. provider 모듈을
   놓는 작업 · L3 조치 · 수동 단계가 있으면 화면이 그 노드와 작업, 까닭을 보이고 실행하지 않습니다. 작업을 시작한 노드가
   120초 동안 답하지 않으면 실행을 중단하고 그 노드를 「결과를 알 수 없음」으로 보입니다. 이미 시작한 원격 작업은 뒤늦게
   끝날 수 있습니다.
   운영 기계에 적용하는 일은 화면이 하지 않습니다. 06 화면의 도움말에 적힌 명령으로 운영자가 실행합니다. 05에서 다시
   확정해 계획이 바뀌면 06 카드가 「서명이 지금 계획과 맞지 않습니다」가 되고, 지난 서명을 보관하고 새 계획에 서명해야
   만들 수 있습니다.
7. **다음 회차.** 확정한 세션은 고칠 수 없어 읽기 전용으로 보입니다. 새 관측이 들어왔거나 결론을 바꾸려면 05 화면의
   「새 검토 시작」(스코프는 02 화면의 「새 스코프 검토 시작」)을 누릅니다. 새 세션 ID로 새 회차가 서고, 결론과 계획 칸은
   옮겨 오며, 승인 서명은 새로 받습니다. 지난 회차의 계획 · 원장 기록은 그대로 남습니다.
   **06은 이 작업 폴더에서 확정한 계획만 서명 · 생성합니다.** 05 확정이 계획과 함께 상태 폴더의 처리 기록에 그 계획을 만든
   정책의 지문을 남기고, 06은 그것을 지금 활성 정책과 견주기 때문입니다. 다른 작업 폴더에서 옮겨 온 계획이나 이 판보다
   먼저 확정한 계획은 열람과 보관만 됩니다. 옮겨 온 계획은 pqcota의 `pqcota-approve` · `pqcota-provision`으로 승인하고
   생성합니다. 이 판보다 먼저 확정한 계획을 06에서 다루려면 현재 정책에서 05의 새 판정 회차를 열어 다시 확정합니다.


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
   `prepared copy at` 뒤의 경로를 `PREPARED`에 넣습니다(아래는 예시입니다). `<…>`를 실제 값으로 바꾼 뒤 실행하십시오.

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

## 9. 판 올리기

### v0.32.0에서 v0.33.0으로

v0.33.0은 04에서 여는 「이력에서 앱별 통신 보기」를 더합니다. 함께 쓰는 pqcota(v0.20.0) · 규칙 묶음 · 상태 형식 · 세션 · 계획 형식이
v0.32.0과 같아 작업 폴더와 이력을 그대로 씁니다. 화면을 끄고 [2절](#2-설치)의 `go install`로 바이너리를 바꿉니다. 새 화면은 화면
계정이 이미 가진 조회 권한으로 열리므로 권한을 더 주지 않습니다. 상대를 노드 이름으로 보려면 [6절](#6-하루-절차) 04 항목의
「상대가 주소로만 보일 때」대로 판정 조직에 엔드포인트를 등록합니다. 04의 토폴로지 그림은 이제 화면이 뜬 뒤에 따로 그려지고, `dot`이 낸 SVG를
검사한 뒤에 실으며, 그리는 데 시간(10초)과 크기(4 MiB) 상한이 있습니다. 그리지 못하면 까닭과 DOT 원문이 보입니다.

### v0.31.1에서 v0.32.0으로

v0.32.0은 상류 pqcota v0.20.0(v0.17.0 · v0.18.0 · v0.19.0 포함)을 반영합니다. 규칙 묶음(`pqcota-enrich/v4`) · 계약 · 이력의 표와 열이
v0.16.0과 같고 상태 형식 · 세션 · 계획 형식도 v0.31.1과 같아 작업 폴더를 그대로 엽니다.

1. 화면을 끕니다.
2. **상류 도구를 v0.20.0으로 함께 올립니다**(2절 「pqcota 준비」의 `pqcota-ctl` 묶음). 같은 이력에 적재 · 정리 · 기록하는 도구는 같은
   판으로 둡니다. 스키마는 v0.16.0과 같아 `pqcota-inventory -init-schema`를 다시 돌리지 않아도 됩니다.
3. [2절](#2-설치)의 `go install`로 `pqcaton-ui`와 준비 도구 `pqcaton-collectors`를 v0.32.0으로 설치하고, 「받은 수집기를 확인하기」대로
   v0.20.0 릴리스 자산을 받아 수집기 폴더를 바꿉니다. 03은 v0.16.0 수집기를 「맞지 않음」으로 막습니다.
4. 수집기 서명 키와 검증 값(`PQCOTA_VERIFY_KEY`의 수집기 ID 결합 형식)은 그대로 씁니다. 상류 v0.17.0의 노드별 키 파일
   (`PQCOTA_VERIFY_KEY_FILE`)로 옮기지 않습니다. 이 판의 03은 수집기 ID마다 키 하나로 서명합니다.
5. 화면을 띄우고 03에서 다시 관측합니다. v0.20.0의 `pqcota-netcap`은 노드가 받은 연결도 기록하므로, 서비스를 받기만 하던 노드에
   통신 엣지가 새로 생길 수 있습니다. 04는 같은 연결의 두 기록을 한 엣지로 보이고, 방향을 확인하지 못한 연결은 대조 표 아래에
   따로 보입니다. 상류 `pqcota-inventory -diff`는 받은 쪽 기록을 「추가됨」으로 보이는데, 자산이 바뀐 것은 아닙니다.

### v0.31.0에서 v0.31.1로

v0.31.1은 01 선언의 저장 결함(v0.6.0부터)을 고친 패치입니다. 상태 형식 · 세션 · 계획 형식 · 규칙 묶음과 함께 쓰는 pqcota가
v0.31.0과 같아 작업 폴더를 그대로 엽니다. 화면을 끄고 [2절](#2-설치)의 `go install`로 바이너리를 바꿉니다. 01은 이제 이름이나 IP가
빈 노드를 저장하지 않습니다. 선언 파일에 IP 없는 노드가 이미 있으면(손으로 고친 파일) 01이 그 줄을 보여 주고, IP를 채우거나
「제거」로 지워야 저장됩니다.

### v0.30.0에서 v0.31.0으로

v0.31.0은 상류 pqcota v0.16.0(v0.15.0 포함)을 반영합니다. 규칙 묶음(`pqcota-enrich/v4`) · 계약 · 이력의 표와 열이 v0.14.0과 같고 상태
형식 · 세션 · 계획 형식도 v0.30.0과 같아, **규칙 전환 회차를 거치지 않고** 작업 폴더를 그대로 엽니다. 바뀌는 것은 셋입니다.
**03 관측은 수집기 서명 키 없이 시작하지 않고, 운영자의 `ansible.cfg`를 쓰지 않으며, Windows 노드를 관측하지 않습니다.** pqcota
이력은 스키마를 정해진 명령으로 만들고, 조회 명령은 표를 만들지 않습니다.

1. 화면을 끄고 `-recover-status`가 `no unfinished transactions`를 찍는지 확인한 뒤, 작업 폴더를 [7절](#7-백업과-복원)대로
   백업합니다.
2. **상류 도구를 v0.16.0으로 함께 올립니다**(2절 「pqcota 준비」의 `pqcota-ctl` 묶음). 같은 이력에 적재 · 정리 · 기록하는 도구는 같은
   판이어야 합니다.
3. **pqcota 이력을 준비합니다.** 아래 「기존 이력을 세 역할로 옮기기」를 따릅니다. 옮기지 않고 지금의 두 역할을 그대로 쓰더라도,
   표를 소유한 역할로 `pqcota-inventory -init-schema`를 한 번 돌립니다. v0.16.0부터 조회 명령은 표를 만들지 않으므로, 이 판이 더하는
   표(메타데이터 · 소스 근거)가 없으면 그 명령들이 멈춥니다.
4. [2절](#2-설치)의 `go install`로 `pqcaton-ui`와 준비 도구 `pqcaton-collectors`를 v0.31.0으로 설치하고, 「받은 수집기를 확인하기」대로
   v0.16.0 릴리스 자산을 받아 수집기 폴더를 바꿉니다. 03은 v0.14.0 수집기를 「맞지 않음」으로 막습니다.
5. **수집기 서명 키를 준비합니다**(2절 「수집기 서명 키 준비」). **v0.31.0의 03은 이것 없이 관측을 시작하지 않습니다.**
6. **노드를 맞춥니다.** 접근 정보에서 Windows 노드를 뺍니다. 일반 계정으로 접속하는 노드는 그 계정의 sudo를 `NOPASSWD`로 두고
   입출력 기록을 끕니다(2절 「노드 점검」).
7. **`ansible.cfg`의 값을 옮깁니다**(아래 표). 03은 운영자의 `ansible.cfg`와 `ANSIBLE_*` 환경 변수를 읽지 않습니다.
8. 화면을 띄워 03의 점검이 모두 ✓인지 보고 관측합니다. 실행 로그에서 노드마다 점검 통과와 결과마다 「signature checked」를 봅니다.
9. **적재를 수집기 ID 결합 형식으로 바꿉니다.** 상류가 권하는 순서는 중앙 도구 → 수집기 → 확인 → 새 값입니다. 2와 4가 앞의 둘이고,
   8에서 받은 결과를 `pqcota-ingest -verify-keys`로 확인한 뒤, 적재 쪽 `PQCOTA_VERIFY_KEY`를 `verify.pub`의 값으로 바꿉니다
   ([6절](#6-하루-절차)의 재적재 명령). **03 밖에서 직접 돌리는 수집기도 같은 키로 서명하게 한 뒤에** 바꿉니다. 먼저 바꾸면 서명하지
   않는 수집기의 결과가 모두 거절됩니다. 옛 키 목록 꼴(ID 없이 키만)은 상류가 경고와 함께 아직 받습니다.

<a id="기존-이력을-세-역할로-옮기기"></a>

**기존 이력을 세 역할로 옮기기.** v0.30.0의 안내대로 만든 이력은 적재 계정(`pqcota_ingest`)이 데이터베이스와 표를 소유합니다. 표는
소유자만 바꿀 수 있으므로, 새 스키마 역할이 그대로 `pqcota-inventory -init-schema`를 돌리면 「must be owner of table pqcota_snapshots … (rolled back,
nothing was changed)」로 아무것도 바꾸지 않고 실패합니다. 그래서 표의 소유자를 스키마 역할로 옮깁니다. 관리자 계정으로 실행합니다.

1. 지금 누가 무엇을 소유하는지 봅니다(`pqcota` 데이터베이스에서).

   ```sql
   SELECT tablename, tableowner FROM pg_tables WHERE tablename LIKE 'pqcota\_%';
   ```

2. 스키마 역할을 만들고 연결 · 스키마 권한을 줍니다(`postgres` 데이터베이스에서 앞의 세 줄, `pqcota` 데이터베이스에서 마지막 줄).

   ```sql
   CREATE ROLE pqcota_schema LOGIN;
   \password pqcota_schema
   GRANT CONNECT ON DATABASE pqcota TO pqcota_schema;
   GRANT USAGE, CREATE ON SCHEMA public TO pqcota_schema;
   ```

3. `pqcota` 데이터베이스에서 pqcota 표의 소유자를 옮기고, 2절 「pqcota 준비」 4번의 권한을 줍니다. 첫 줄은 psql의 `\gexec`로 표마다
   `ALTER TABLE … OWNER TO pqcota_schema`를 돌립니다. 표의 시퀀스와 인덱스는 표를 따라 옮겨집니다. `REASSIGN OWNED BY pqcota_ingest`는
   그 역할이 가진 모든 것을 옮기므로, pqcota만 쓰는 데이터베이스가 아니면 쓰지 않습니다.

   ```sql
   SELECT format('ALTER TABLE %I OWNER TO pqcota_schema', tablename) FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'pqcota\_%' \gexec
   GRANT USAGE ON SCHEMA public TO pqcota_ingest, pqcaton_ui;
   GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO pqcota_ingest;
   GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO pqcota_ingest;
   GRANT SELECT ON ALL TABLES IN SCHEMA public TO pqcaton_ui;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pqcota_ingest;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT USAGE ON SEQUENCES TO pqcota_ingest;
   ALTER DEFAULT PRIVILEGES FOR ROLE pqcota_schema IN SCHEMA public GRANT SELECT ON TABLES TO pqcaton_ui;
   ```

4. `pqcota_schema`로 `pqcota-inventory -init-schema`를 돌립니다. 있는 표는 그대로 두고 이 판이 더하는 표만 만들며, 끝나면 「schema ready: N created,
   M already present」로 알립니다. 그 뒤로 적재 계정은 `PQCOTA_AUTO_DDL=0`으로 돌립니다. 더는 표를 소유하지
   않으므로, 이 값 없이 적재하면 「permission denied for schema public」으로 실패합니다. `<…>`를 실제 값으로 바꾼 뒤 실행하십시오.

   ```bash
   PQCOTA_DSN='postgres://pqcota_schema@<호스트>:5432/pqcota' pqcota-inventory -init-schema
   ```

**`ansible.cfg`에서 옮길 값.** 03은 실행마다 제품이 만든 설정 하나만 읽습니다. 기존 `ansible.cfg`에 두던 값은 아래처럼 옮기거나,
이번 판에서는 쓰지 않습니다. 03 밖에서 운영자가 06의 플레이북을 적용할 때는 지금처럼 자기 `ansible.cfg`를 씁니다.

| `ansible.cfg`의 값 | 03에서 |
|---|---|
| `remote_user` | 접근 정보(`hosts.csv`)의 계정(`ssh_user`)에 노드마다 적습니다 |
| `private_key_file` | 접근 정보의 SSH 키 경로(`ssh_key`)에 적거나, `~/.ssh/config`의 `IdentityFile`로 둡니다. ssh-agent도 씁니다 |
| `remote_port` | 접근 정보의 포트에 적습니다 |
| `ssh_args`, 점프 호스트(`ProxyJump` · `ProxyCommand`) | `~/.ssh/config`로 옮깁니다. ssh가 그대로 읽습니다. 연결 공유(`ControlMaster` · `ControlPersist`)는 03이 끕니다. 포트는 접근 정보의 값을 언제나 쓰고, 사용자는 접근 정보에 계정을 비운 노드에서만 `~/.ssh/config`를 따릅니다 |
| `host_key_checking = False` | 지원하지 않습니다. 03은 호스트 키를 검사하므로 노드마다 `~/.ssh/known_hosts`에 받아 둡니다 |
| 권한 상승(`become_method` · `become_user` · `become_exe` · `become_flags` · `become_ask_pass`) | 바꿀 수 없습니다. 03은 내장 sudo로 root가 되고 비밀번호를 묻지 않습니다. root로 접속하거나 sudo를 `NOPASSWD`로 둡니다 |
| `pipelining` · `keep_remote_files` · `verbosity` | 03이 고정합니다(pipelining 켬, 원격 파일 남기지 않음, 기본 출력) |
| `log_path` · `callbacks_enabled` · `stdout_callback` | 지원하지 않습니다. 실행 로그는 `observations/<실행>/log.txt`와 03 화면에 남습니다 |
| 플러그인 · 컬렉션 · 역할 경로, `library` | 쓰지 않습니다. 03은 내장 모듈만 씁니다 |
| `interpreter_python` | 지원하지 않습니다. 노드의 Python을 자동으로 찾습니다(노드에 Python 3이 있어야 합니다) |
| `forks` · `timeout` · `gathering` | 지원하지 않습니다. Ansible의 기본값을 씁니다 |
| `inventory`, vault 설정 | 쓰지 않습니다. 인벤토리는 03이 접근 정보로 실행마다 만들고, 접근 비밀은 `hosts.csv`에 있습니다 |

### v0.29.1에서 v0.30.0으로

v0.30.0은 상류 pqcota v0.14.0을 반영합니다. 규칙 묶음이 `pqcota-enrich/v4` 그대로이고 상태 형식 · 세션 · 계획 형식도
v0.29.1과 같아, **규칙 전환 회차를 거치지 않고** 작업 폴더를 그대로 엽니다. pqcota 이력도 다시 적재하지 않아도 됩니다.

1. 화면을 끄고 `-recover-status`가 `no unfinished transactions`를 찍는지 확인한 뒤, 작업 폴더를 [7절](#7-백업과-복원)대로
   백업합니다.
2. 상류 도구(`pqcota-ingest` · `pqcota-cbom-ingest` · `pqcota-prune` · `pqcota-approve` · `pqcota-provision`)를 v0.14.0으로 함께 올립니다.
   **같은 pqcota 이력에 적재하고 정리하는 도구는 같은 판이어야 합니다.** 상류 v0.13.1부터 적재와 정리가 노드 잠금을 함께
   잡으므로, 옛 적재 도구나 정리 도구가 남아 있으면 새 도구와 겹칠 수 있습니다.
3. [2절](#2-설치)의 `go install`로 `pqcaton-ui`와 준비 도구 `pqcaton-collectors`를 v0.30.0으로 설치합니다.
4. 「받은 수집기를 확인하기」(2절)대로 `pqcaton-collectors fetch`로 v0.14.0 릴리스 자산을 받고, 노드의 수집기 셋과 `collector.jar`를
   그것으로 바꿉니다. 03은 v0.13.0 수집기를 「맞지 않음」으로 막습니다.
5. **공식 자산으로 바꿨다면 `-allow-unverified-collectors`를 빼고 시작합니다.** v0.29.x까지는 릴리스 자산도 「확인 불가」라 이
   옵션이 필요했지만, v0.14.0 릴리스 자산은 03에서 「확인됨」입니다. 화면을 띄운 뒤 03에서 수집기와 `collector.jar`가 모두
   「확인됨」인지 보고 다시 관측합니다.
6. **`collector.jar` 검사가 엄격해졌습니다.** 판 정보가 없는 JAR이나 직접 빌드한 JAR, 고친 트리의 JAR은 이제 「확인 불가」라
   `-allow-unverified-collectors` 없이는 시작하지 않습니다. 상류 v0.14.0 릴리스의 `collector.jar`로 바꾸는 것이 기본
   해결책입니다.
7. **이전 정책으로 되돌리는 활성화는 그 정책으로 결과를 다시 적재한 뒤에 할 수 있습니다.** 다시 적재하기 전에는 지금처럼
   `upstream-revert`로 거절합니다.

### v0.29.0에서 v0.29.1로

v0.29.1은 관측 뒤 회수의 결함(v0.27.0부터)과, 마무리 중인 관측 위로 새 관측을 받던 결함(v0.24.0부터)을 고친 패치입니다. 상태 형식 · 세션 · 계획 형식 · 규칙 묶음과 함께 쓰는 pqcota가
v0.29.0과 같아 작업 폴더를 그대로 엽니다. 화면을 끄고 [2절](#2-설치)의 `go install`로 바이너리를 바꿉니다. 03의 「남은 프로세스」에
이 판보다 먼저 남은 것이 보이면 그 안내대로 확인한 뒤 끝냅니다.

### v0.28.0에서 v0.29.0으로

v0.29.0은 상류 pqcota v0.13.0을 반영합니다. 상류 v0.12.0에서 규칙 묶음이 `pqcota-enrich/v4`로 바뀌어(JCA 배포본 식별) v0.28.0으로
쌓은 세션 · 계획은 규칙 전환 회차로 넘깁니다. 상태 형식과 세션 · 계획 형식은 v0.28.0과 같습니다. 상류 v0.12.0을 거칠 필요는 없고
v0.13.0으로 바로 올립니다.

1. 화면을 끄고 `-recover-status`가 `no unfinished transactions`를 찍는지 확인한 뒤, 작업 폴더를 [7절](#7-백업과-복원)대로
   백업합니다. 상류 DB도 백업합니다.
2. **같은 DB를 읽는 상류 도구부터 올립니다.** v0.11.0의 `pqcota-inventory`는 v0.12.0 이상이 쓴 스냅샷이 든 노드의 이력을 열지
   못하고, v0.11.0의 `pqcota-provision --dsn`은 그런 DB에서 플레이북을 낸 뒤 종료 상태 1로 실패할 수 있습니다. 다섯 리포를
   v0.13.0으로 함께 둡니다. 상류를 아직 올리지 않는다면 pqcaton도 v0.28.0에 둡니다.
3. 노드의 수집기 셋(`pqcota-nodescan` · `pqcota-netcap` · `pqcota-jvmscan`)과 `collector.jar`를 [2절](#2-설치)대로 v0.13.0 릴리스
   자산으로 바꿉니다. 릴리스 자산도 「확인 불가」라 `-allow-unverified-collectors`는 그대로 필요합니다.
4. [2절](#2-설치)의 `go install`로 바이너리를 바꾸고 화면을 띄운 뒤 03에서 다시 관측합니다. **pqcota 이력도 v4로 다시 적재합니다.**
   규칙 판이 스냅샷 지문에 들어가므로, v0.11.0이 v3로 적재한 이력으로는 05의 활성화 · 기준선 대조가 통과하지 않습니다.
   `PQCOTA_ORG='<판정 조직>' pqcota-ingest -scope-assets '<작업 폴더>/scope-assets.csv' '<작업 폴더>/results'`로 다시 적재합니다.
5. 05가 「이 세션은 지금과 다른 상류 규칙 판으로 열렸습니다」를 보이면 「지금 규칙으로 새 회차 시작」을 누릅니다(명령으로는
   `pqcaton-decide new-round`). **지난 결론은 참고값으로만 옮겨집니다.** Java 자산은 근거가 배포본 단위의 새 finding으로 바뀌어
   보고서에 「바뀜」으로 보이고, 「사라짐」 · 「생김」에는 식별 규칙이 바뀐 영향이 섞일 수 있습니다. 항목마다 다시 판정하거나
   「다시 채택」을 누릅니다. 옛 결론을 새 근거에 자동으로 잇는 규칙은 없습니다.
6. **자산 스코프 규칙의 `app_key`에 사용자 서비스 `user@<uid>.service`를 적었다면 고칩니다.** v0.13.0 수집기는 사용자 서비스의
   앱 키를 `user@<uid>.service/<유닛>`으로 내므로 옛 값은 더는 맞지 않습니다. 아래처럼 찾은 뒤 02에서 규칙을 고쳐 확정하고 05에서
   활성화합니다. `<…>`를 실제 값으로 바꾼 뒤 실행하십시오.

   ```bash
   grep -n 'user@[0-9]*\.service' '<작업 폴더>/asset-scope.csv' '<작업 폴더>/scope-assets.csv'
   ```

7. **엣지의 관측 횟수(`observed_count`)는 판 올림 전후로 견주지 않습니다.** v0.13.0부터 핸드셰이크 메시지 수가 아니라 관측
   구간에 본 고유 연결 수를 셉니다. 줄었다고 통신이 줄어든 것이 아닙니다. 엣지도 연결을 연 애플리케이션마다 나뉘어, 상류의
   diff에는 규칙 경고 없이 사라짐과 생김으로 보입니다. pqcaton의 엣지는 출발 · 도착 · 포트 · 프로토콜로 셈하므로 대조 상태는
   그대로입니다.
8. 06 화면은 v3 규칙 판의 계획을 서명 · 생성하지 않습니다. 이미 서명된 v3 계획과 그 플레이북은 열람 · 보관만 합니다. 새 회차에서
   확정한 계획을 서명합니다.

### v0.27.1에서 v0.28.0으로

v0.28.0부터 화면은 pqcota 이력 없이 시작하지 않고, 정책 활성화와 기준선 채택은 언제나 pqcota 이력 대조를 거칩니다. 상태 형식 ·
세션 · 계획 형식과 규칙 묶음(`pqcota-enrich/v3`)은 v0.27.1과 같아 작업 폴더를 그대로 엽니다. 상류는 v0.11.0 그대로입니다.

1. **연결 정보를 먼저 둡니다.** [2절](#2-설치) 「pqcota 연결」대로 `pqcota.env`(0600, `PQCOTA_DSN=<연결 정보>` 한 줄)를 만들거나,
   화면을 띄우는 셸에 `PQCOTA_DSN`을 둡니다. 서비스 · 실행기로 띄운다면 파일이 있어야 합니다. 없으면 화면이 시작하지 않습니다.
2. **pqcota 적재를 판정 조직으로 합니다.** pqcota 이력의 조직은 판정 조직이어야 합니다. 지금까지 다른 조직(기본 조직 등)에
   적재했다면 `PQCOTA_ORG='<판정 조직>' pqcota-ingest -scope-assets '<작업 폴더>/scope-assets.csv' '<작업 폴더>/results'`로 다시 적재합니다.
   `-upstream-org`에 판정 조직과 다른 값을 주던 명령은 이제 멈춥니다.
3. 화면을 끄고 [2절](#2-설치)의 `go install`로 바이너리를 바꿉니다.
4. **`pqcaton-decide activate-scope -confirm-ingest`를 쓰던 스크립트를 고칩니다.** 이 판은 그 플래그를 받으면 아무것도 쓰지 않고 멈춥니다. 활성화는 화면 05에서
   하거나, `PQCOTA_DSN`을 둔 채 `pqcaton-decide activate-scope`를 플래그 없이 돌립니다.
5. **이전 정책으로 되돌리는 활성화는 이 판에서 할 수 없습니다**(`upstream-revert`). 되돌려야 하면 v0.27.x에서 하던 운영자 진술을
   대신할 길이 없으므로, 상류 이슈가 풀릴 때까지 기다리거나 새 정책으로 확정합니다.

### v0.27.0에서 v0.27.1로

v0.27.1은 상류 pqcota v0.11.0을 반영합니다. 상태 형식 · 세션 · 계획 형식과 규칙 묶음(`pqcota-enrich/v3`)이 v0.27.0과 같아
작업 폴더를 그대로 엽니다.

1. **기록을 읽는 상류 도구부터 올립니다.** v0.11.0이 쓴 기록은 v0.10.7의 `pqcota-records`가 읽지 못합니다. 같은 DB를 쓰는
   `pqcota-records`를 v0.11.0으로 바꾼 뒤에 `pqcota-ingest` · `pqcota-approve` · `pqcota-provision`을 올립니다. 다섯 리포를 같은
   태그로 둡니다.
2. 화면을 끄고 v0.27.1을 `go install …@v0.27.1`로 받습니다.
3. 수집기는 [2절](#2-설치)대로 v0.11.0 릴리스 자산을 받아 `SHA256SUMS`로 확인한 뒤 둡니다. 릴리스 자산도 「확인 불가」라
   `-allow-unverified-collectors`는 그대로 필요합니다.
4. **소스 근거나 승인 가이드가 든 계획은 06이 서명 · 생성하지 않습니다**(`plan-fields`). 05가 만드는 계획에는 그 필드가 없어
   지금 쓰는 절차는 그대로입니다. 그런 계획은 `pqcota-approve --review`로 확인한 뒤 상류 도구로 승인하고 생성합니다.

### v0.26.x에서 v0.27.0으로

v0.27.0은 상태 형식 · 세션 · 계획 형식과 규칙 묶음(`pqcota-enrich/v3`)이 v0.26.3과 같아 작업 폴더를 그대로 엽니다. 함께 쓰는
pqcota도 v0.10.7 그대로입니다.

1. 화면을 끄고 [2절](#2-설치)의 `go install`로 바이너리를 바꿉니다. v0.26.2를 쓰고 있다면 v0.26.2의 보안 수정도 이 판에 들어 있습니다.
2. **03이 관측을 시작하기 전에 수집기의 판을 봅니다.** 지금의 상류 빌드는 의존 모듈을 옆 폴더 replace로 빌드하므로 모두
   「확인 불가」로 보이고, 기본으로는 관측을 시작하지 않습니다. 옆의 두 리포도 같은 태그인지 확인한 뒤 화면을
   `-allow-unverified-collectors`로 띄웁니다([2절](#2-설치)의 「수집기의 판」). 서비스로 띄운다면 서비스의 `ExecStart`에도
   더합니다. 판이 맞지 않는 수집기는 이 옵션으로도 쓸 수 없으니 v0.10.7로 다시 빌드합니다.
3. **승인 키를 기본 위치에 두면 플래그가 필요 없습니다.** `-approval-key`를 그대로 쓰는 설정은 그대로 동작합니다. 기본 위치로
   옮기려면 [2절](#2-설치)의 「승인 키 준비」대로 두고, 확인용 공개 키(`approval.pub`)도 함께 둡니다. 명시한 키가 잘못됐으면 이제
   화면이 시작하지 않습니다.
4. 「회수 필요」가 보이면 03의 「남은 프로세스」가 상태를 보입니다. 이 판보다 먼저 남긴 기록은 시작 시각이 없어 「같은
   프로세스인지 확인할 수 없음」으로 보입니다.

### v0.26.x에서 v0.26.3으로

v0.26.3은 상류 pqcota v0.10.7을 반영한 보안 패치입니다. 같은 수정을 담은 v0.26.2는 `go install`로 받을 수 없어
v0.26.3으로 다시 냈습니다. v0.26.2를 이미 쓰고 있다면 바이너리만 바꿉니다. 상태 형식 · 세션 · 계획 형식과 규칙 묶음(`pqcota-enrich/v3`)은
v0.26.1과 같아 작업 폴더를 그대로 엽니다.

1. 화면을 끄고 [2절](#2-설치)의 `go install`로 바이너리를 바꿉니다.
2. **상류도 v0.10.7로 올립니다.** 특히 `pqcota-approve` · `pqcota-provision`은 v0.10.7이어야 합니다. v0.10.6의 승인 서명은
   문자열에 NUL 문자가 든 계획에서 다른 계획에도 통과할 수 있었습니다. 노드의 `pqcota-jvmscan`도 v0.10.7로 바꿉니다.
3. 문자열에 NUL 문자가 든 계획은 05가 확정하지 않고(`session-text`), 이미 확정된 그런 계획은 06이 서명 · 생성하지 않습니다
   (`plan-text`). 그런 계획은 새 판정 회차를 열어 그 값을 고친 뒤 다시 확정합니다. NUL 문자가 없는 계획은 승인 정규형이
   v0.10.6과 같아, 이미 한 승인이 그대로 유효합니다.

### v0.25에서 v0.26으로

v0.26은 판정 세션과 확정 처리 기록에 자산 스코프 정책의 지문을 남기고, 02에서 확정한 정책을 정식 절차로만 대조에 씁니다.

1. 화면을 끄고 `-recover-status`가 `no unfinished transactions`를 찍는지 확인한 뒤, 작업 폴더를 [7절](#7-백업과-복원)대로
   백업합니다. **되돌릴 수 없게 되는 시점이 있습니다.** 이 판에서 05를 확정하거나, 정책을 활성화하거나, 기준선 회차를 열면
   상태 폴더에 형식 2 처리 기록이 생기고, 그 직전에 상태 문서(`.pqcaton/state.json`)가 형식 2로 올라갑니다. 이 판이 새로
   만든 상태 폴더는 처음부터 형식 2입니다. 그 뒤로 v0.25.x 이하는 이 작업 폴더를 열지 않습니다. 화면은 시작하지 않고,
   상태 폴더를 쓰는 명령도 「state.json is damaged or from another version」으로 멈춥니다. 손상이 아니라 새 판의 상태
   폴더를 옛 판이 읽지 못한 것입니다. 옛 판은 형식 2 처리 기록을 읽기 전까지는 멈추지 않아 활성화 전 계획에 서명 양식까지
   내므로, 이 판은 상태 문서로 옛 판을 시작 단계에서 막습니다. 옛 판으로 돌아가야 하면 백업을 복원합니다.
2. [2절](#2-설치)의 `go install`로 바이너리를 바꾸고 화면을 띄웁니다. 상류는 v0.10.6 그대로입니다.
3. **이 판보다 먼저 만든 판정 세션은 정책 기록이 없어 05가 잠깁니다.** 지금 `scope-assets.csv`로 pqcota를 적재한 뒤 05
   위쪽에서 기준선 회차를 엽니다. 활성화와 같은 순서로 pqcota 이력을 지금 정책으로 대조하고, 일치할 때만 엽니다. 옛 세션은
   보관되고, 지금 정책으로 다시 세운 새 회차가 열리며, 지난 결론은 「지난 판정」으로만 옮겨집니다. 명령으로는
   `pqcaton-decide baseline`입니다(`PQCOTA_DSN` 필요, `pqcaton-decide baseline -confirm-ingest`는 지원을 끝냈습니다).
4. 이 판보다 먼저 확정한 계획은 06에서 열람 · 보관만 됩니다. 기준선 회차에서 다시 확정한 계획을 서명합니다.
5. 명령으로 확정하던 운영자는 `pqcaton-decide close`에 세션을 열 때와 같은 `-scope-assets`를 줍니다. 세션의 정책과 다르면
   확정하지 않습니다.

### v0.24에서 v0.25로

v0.25는 상류 pqcota v0.10.6의 규칙 묶음(`pqcota-enrich/v3`)으로 정규화합니다. v0.24.x로 쌓은 세션 · 계획 · 판정 원장은
그대로 남고, 지금 규칙으로 다시 판정하는 회차를 엽니다.

1. 화면을 끄고 `-recover-status`가 `no unfinished transactions`를 찍는지 확인한 뒤, 작업 폴더를 [7절](#7-백업과-복원)대로
   백업합니다.
2. **상류를 함께 올립니다.** 상류 다섯 리포를 v0.10.6으로 올리고(pqcaton 판마다 함께 쓰는 판은 [판별 표](compatibility.ko.md)), 노드의 수집기와 `collector.jar`도 v0.10.6으로 바꿉니다.
   상류 DB는 올리기 전에 백업합니다(상류 v0.10.6의 안내). v0.10.5 이하의 상류 이력과 섞으면 계획의 근거를 찾지 못합니다.
   상류를 아직 올리지 않는다면 pqcaton도 v0.24.1에 둡니다.
3. [2절](#2-설치)의 `go install`로 바이너리를 바꾸고 화면을 띄웁니다.
4. 판정 화면(05)이 「이 세션은 지금과 다른 상류 규칙 판으로 열렸습니다」를 보이면 「지금 규칙으로 새 회차 시작」을 누릅니다.
   옛 세션은 `session.ruleset-<별칭>-<세션 ID>.json`으로 바이트 그대로 보관되고, 그 자리에 지금 규칙의 새 세션이, 옆에 변환
   보고서(`session.ruleset-change-<세션 ID>.json`)가 생깁니다. 명령으로는 `pqcaton-decide new-round`입니다.
5. **지난 결론은 참고값으로만 옮겨집니다.** 항목마다 다시 판정하거나 「다시 채택」을 누릅니다. 보고서에서 사라짐 · 생김 ·
   바뀜으로 보이는 것은 규칙이 바뀐 것만으로 생겼을 수 있습니다.
6. 같은 자산 키 아래 설치가 둘 이상이면(같은 이름의 OpenSSL을 다른 경로에서 봤으면) 확정 전에 「이 조치가 아래 설치 전부에
   적용됨을 확인합니다」를 고릅니다. 설치마다 다른 조치는 이 판에서 표현하지 못합니다.
7. 06 화면은 옛 규칙 판의 계획을 서명 · 생성하지 않습니다. 이미 서명된 옛 계획과 그 플레이북은 그대로 두고 열람 · 보관만
   합니다. 새 회차에서 확정한 계획을 서명합니다.
8. 03 · 04 화면이 「v0.10.6 JCA 수집 결과임을 확인할 수 없는 노드」를 알리면 그 노드의 수집기와 `collector.jar` 판을
   확인합니다.

### v0.22에서 v0.23으로

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
| `plan-ruleset` | 계획이 지금과 다른 상류 규칙 판으로 판정됐거나 근거의 규칙 판을 확인할 수 없어, 서명 · 생성하지 않았습니다. 있는 파일은 그대로입니다 | 새 회차에서 다시 확정합니다. 상류를 v0.10.5 이하로 운영한다면 pqcaton v0.24.x나 상류 `pqcota-provision` v0.10.5로 생성합니다 |
| `plan-scope` | 계획이 이 작업 폴더에서 확정한 기록과 이어지지 않거나(다른 곳에서 옮겨 왔거나, 확정한 뒤 계획 파일이 바뀌었거나, 이 판보다 먼저 확정해 정책 기록이 없음), 그 계획을 만든 자산 스코프 정책이 지금 활성 정책과 달라 서명 · 생성하지 않았습니다. 있는 파일은 그대로입니다 | 현재 정책에서 05의 새 판정 회차를 열어 다시 확정합니다. 다른 곳에서 옮겨 온 계획은 `pqcota-approve`와 `pqcota-provision`으로 승인하고 생성합니다 |
| `plan-fields` | 계획(또는 서명한 계획)에 소스 근거나 승인 가이드(상류 v0.11.0)가 있어 서명 · 생성하지 않았습니다. 계획이 손상된 것은 아니고, pqcaton이 아직 그 내용을 06에서 검토하거나 플레이북에 싣지 않습니다. 있는 파일은 그대로입니다 | `pqcota-approve --review`로 내용을 확인한 뒤 `pqcota-approve` · `pqcota-provision`으로 승인하고 생성합니다. 서명한 계획에 있으면 보관할 수 있습니다 |
| `plan-text` | 계획(또는 서명한 계획)의 문자열에 NUL 문자가 있어 서명 · 생성하지 않았습니다. 그런 계획은 승인이 다른 계획에서도 통과할 수 있어 pqcota가 서명과 생성을 거절합니다. 있는 파일은 그대로입니다 | 새 판정 회차를 열어 해당 값을 고친 뒤 다시 확정합니다. 서명한 계획에 있으면 먼저 보관합니다 |
| `session-text` | 확정하면 만들어질 계획의 문자열에 NUL 문자가 있어 확정하지 않았습니다. 세션 · 계획 · 처리 기록은 그대로입니다 | 해당 값을 고친 뒤 다시 확정합니다 |
| `scope-policy` | 판정 세션을 세울 때 쓴 자산 스코프 정책이 지금 정책과 다르거나, 세션에 정책 기록이 없어(이 판보다 먼저 만든 세션) 확정하지 않았습니다. 파일은 그대로입니다 | 세션을 세울 때와 같은 정책으로 확정하거나, 지금 정책으로 새 세션을 엽니다. 명령은 `pqcaton-decide close`에 세션을 열 때와 같은 `-scope-assets`를 줍니다 |
| `scope-baseline` | 판정 세션에 자산 스코프 정책 기록이 없어(이 판보다 먼저 만든 세션) 저장 · 확정하지 않았습니다 | 지금 `scope-assets.csv`로 pqcota를 적재한 뒤 판정 화면 위쪽에서 기준선 회차를 엽니다. pqcota 이력 대조를 거칩니다 |
| `scope-unactivated` | `scope-assets.csv`가 활성화 기록 없이 바뀌어 저장 · 확정하지 않았습니다 | 이전 정책 파일을 복원하거나, 상류를 새 정책으로 다시 적재한 뒤 정식 정책 활성화를 거칩니다 |
| `upstream-unreachable` | pqcota 이력에 닿지 못했거나 조회가 10초 안에 끝나지 않아 정책을 활성화하거나 기준선을 채택하지 않았습니다(503). 파일은 그대로입니다 | pqcota 연결을 확인하고 다시 합니다. 운영자 진술로 대신하는 길은 없습니다 |
| `upstream-org` | 연결한 pqcota 이력 · 판정 조직 · 판정 세션의 조직이 같지 않아 조회하지 않았습니다. 아무것도 쓰지 않았습니다 | 판정 조직의 pqcota 이력으로 진행합니다. 다른 조직의 이력으로는 활성화하거나 기준선을 채택할 수 없습니다 |
| `upstream-missing` | pqcota 이력에서 일부 노드의 최신 적재 결과가 없거나 이 정책으로 만든 결과와 다릅니다. 아무것도 쓰지 않았습니다 | 05에 보이는 명령으로 pqcota를 이 정책으로 다시 적재한 뒤 다시 합니다 |
| `upstream-empty` | 결과 폴더에 pqcota 이력과 대조할 관측 결과가 없습니다. 아무것도 쓰지 않았습니다 | 03에서 관측한 뒤 다시 합니다 |
| `upstream-revert` | pqcota의 최신 적재 결과가 이 정책과 다릅니다. 이 정책으로 만든 결과는 이력에 있지만 노드의 최신이 아닙니다. 아무것도 쓰지 않았습니다 | 이 정책으로 결과를 다시 적재한 뒤 활성화합니다 |
| `scope-same` | 02에서 확정한 정책이 이미 활성 정책과 같아(note · 공백만 다를 수 있음) 활성화할 것이 없습니다. 아무것도 쓰지 않았습니다 | 할 일이 없습니다 |
| `scope-not-finalized` | 02에서 확정한 정책이 없거나, 확정한 뒤 `asset-scope.csv`가 바뀌었거나, 읽을 수 없어 활성화하지 않았습니다 | 02에서 확정합니다. 손으로 고친 파일은 활성화하지 않습니다 |
| `ruleset-changed` | 세션이 지금과 다른 상류 규칙 판으로 열렸거나 규칙 판이 없어, 섞이지 않도록 저장 · 확정하지 않았습니다 | 판정 화면 위쪽의 「지금 규칙으로 새 회차 시작」을 누릅니다(명령은 `pqcaton-decide new-round`). 지난 결론은 참고값으로만 옮겨지므로 항목마다 다시 판정합니다 |
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
| `run-not-ready` | 관측을 시작하기 전에 갖출 것(접근 정보 · Ansible · 수집기 · 수집기 서명 키 · 신뢰할 공개 키)이 빠졌거나, Windows 노드가 있거나, 「회수 필요」인 지난 관측이 있습니다 | 03 화면의 점검 목록에서 ✗ 표시된 것을 갖춥니다 |
| `run-busy` | 이미 관측이 돌고 있습니다 | 끝나거나 멈춘 뒤에 다시 시작합니다 |
| `prov-not-ready` | 06의 미리 보기를 화면에서 실행할 조건이 맞지 않습니다(승인 · L2 플레이북 · 접근 정보 · Ansible, 또는 화면이 실행하지 않는 종류의 작업) | 06 화면의 「화면에서 실행하지 않는 까닭」에 적힌 것을 갖춥니다 |
| `prov-busy` | 06의 실행이 이미 돌고 있습니다 | 끝나거나 멈춘 뒤에 다시 시작합니다 |
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
