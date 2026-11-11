# 데모: pqcota 위에 얹는 거버넌스

[개요](../README.md) · [릴리스 노트](../RELEASE_NOTES.md) · [여정](../docs/journey.md) · [설계](../docs/design.md) · [검증 기준](../docs/testcases.md) · **데모** · [구조 그림](https://www.sntsoft.co.kr/pqcaton/)

📊 **실행 전 예상 결과**는 [`expected-output/`](expected-output/)에 있습니다. 확장 리포트와 거버넌스 토폴로지 샘플, 그리고 차이점 설명이 들어 있습니다.

## 이 데모가 세우는 환경

pqcota 데모를 처음 보는 사람을 위해 적습니다. **결제 서비스를 흉내 낸 3노드**가 도커로 뜨고,
그 위에서 관측이 이뤄집니다.

| 노드 | 무엇이 도나 | 왜 그렇게 두었나 |
|---|---|---|
| `web-gw` | 최신 OpenSSL 3 · 클라이언트 | 트래픽을 보내는 쪽. SSH 등급은 클라이언트가 정합니다 |
| `pay-app` | JVM(JCA provider를 런타임에 등록) | **정적 스캔으로는 안 보이고** attach로만 잡힙니다 |
| `pay-db` | 레거시 OpenSSL 1.1.1 · 서버 · 앱 둘이 한 libssl 공유 | 양자취약이 어디까지 번지는지(영향 반경) |

망은 둘로 갈라져 있습니다. `corp`(웹·앱·DB가 닿는 곳)와 `db`(격리 tier). 그래서 어떤 통신은
관측되고 어떤 통신은 원리상 안 보입니다. **그 차이가 「없다」와 「못 봤다」를 구분합니다.**

환경 자체는 pqcota의 `demo/topology/topology.yaml` 하나가 정의합니다. 그 파일을 고치면 노드와
망이 달라지고, **선언은 거기서 만들어지므로 우리 쪽은 손댈 것이 없습니다**(아래).

## 무엇을 얹나

이 데모는 **독립 스택이 아니라 확장**입니다. [pqcota의 디스커버리 데모](https://github.com/randyinthedev-hash/pqcota/tree/v0.10.7/demo)를
그대로 띄운 뒤, 그 위에 이 리포의 기능, 곧 **선언 대비 3-상태 대조(CONFIRMED/UNDECLARED/UNOBSERVED) +
리뷰 큐 + 거버넌스 토폴로지**를 얹습니다.

```
관측 등급 (pqcota)              →   + 선언 대비 reconciliation (pqcaton)
🟢 web-gw→pay-app MLKEM                🟢 web-gw→pay-app  TLS  CONFIRMED
🔴 web-gw→pay-db  고전                 🔴 web-gw→pay-db   TLS  CONFIRMED
🟢 web-gw→pay-app SSH sntrup761        🟢 web-gw→pay-app  SSH  UNDECLARED ← 선언 안 된 통신!
🔴 web-gw→pay-db  SSH curve25519       🔴 web-gw→pay-db   SSH  UNDECLARED
                                       ⚪ pay-db→pay-app  TLS  UNOBSERVED (선언했으나 미관측 ≠ 부재)
```

## 실행

```bash
# 1) OSS 디스커버리 데모를 먼저 띄운다 (환경 + 수집)
../../pqcota/demo/scripts/up.sh
../../pqcota/demo/scripts/demo.sh

# 2) pqcaton의 대조·토폴로지를 그 위에 확장
./scripts/extend.sh          # 산출: demo/topology-governance.svg

# 3) (골라서) 같은 데모를 화면으로 본다. 화면은 pqcota-ctl 안에서 돌고, 이 기계의 브라우저로 연다
./scripts/ui.sh              # → http://127.0.0.1:8765 · 끝낼 때: ./scripts/ui.sh stop

# 4) 정리 (core 데모가 환경 소유)
../../pqcota/demo/scripts/down.sh
```

### 화면으로 보기

`ui.sh`는 **실제 배치와 같은 자리에** 화면을 띄웁니다. 03 관측은 화면이 떠 있는 기계에서 Ansible을 부르므로, 화면은
노드에 닿는 기계에서 돌아야 합니다. 데모에서 그 기계는 `pqcota-ctl`입니다. 운영자는 자기 기계에서 SSH 터널로 화면을
봅니다([운영자 안내서 5절](../docs/operator-guide.md#5-다른-기계에서-보기)). 데모의 `pqcota-ctl`에는 SSH 서버가 없어서,
그 터널을 `docker exec` 중계가 대신합니다.

**포트를 공개하지 않습니다.** 화면에는 로그인이 없어서 루프백 주소에서만 연결을 받습니다. 중계는 이 기계의
`127.0.0.1`에서만 연결을 받고, 연결마다 `docker exec`로 컨테이너 안의 `127.0.0.1:8765`에 잇습니다. 컨테이너에 포트를
공개하면 같은 Docker 망의 데모 노드도 로그인 없는 화면에 바로 닿으므로 그렇게 하지 않습니다.

처음 띄울 때 컨테이너 안에 작업 폴더 `/work/ui`를 만들고, 데모가 이미 가진 것을 실제 운영자가 넣는 자리에 둡니다.

| 넣는 것 | 쓰는 화면 |
|---|---|
| `extend.sh`가 만든 선언 | 01 선언 |
| 상류 적재에 준 자산 스코프 정책(`scope-assets.csv`) | 04 · 05. 같은 정책이어야 스냅샷 지문이 상류 이력과 맞습니다 |
| 노드 접근 정보(`hosts.csv`)와 상류가 빌드한 수집기 | 03 관측 실행 |
| 지금까지의 관측 결과 | 04 대조. 03에서 다시 관측하면 화면이 새 결과로 바꿉니다 |
| 승인 서명 키(작업 폴더 밖, `/work/ui-approval.key`) | 06 승인과 생성 |

02 화면은 「아직 계층이 없습니다」로 열립니다. 편집할 계층은 비어 있지만, 04 · 05가 쓰는 자산 스코프 정책
(`scope-assets.csv`)은 상류 적재에 쓴 것을 그대로 넣어 두었기 때문입니다. 02에서 계층을 만들어 확정하면 새 정책은
`asset-scope.csv`로 나가며, 데모가 이미 쓰는 `scope-assets.csv`를 자동으로 바꾸지는 않습니다. 화면을 다시 띄워도
04 · 05는 `scope-assets.csv`를 그대로 씁니다. 확정한 정책을 대조에 쓰려면 아래 「새 정책 활성화」를 따릅니다.

데모의 수집기는 pqcota 리포에서 옆 폴더 replace로 빌드됩니다. 그래서 03 화면은 그 수집기들을 「확인 불가」로 보이고,
기본으로는 관측을 시작하지 않습니다. `ui.sh`는 이것을 알고 화면을 `-allow-unverified-collectors`로 띄웁니다. 03 화면에
파일마다 판정 줄과 허용 상태가 보이고, 허용한 사실이 `/work/ui.log`에 남습니다([운영자 안내서 2절](../docs/operator-guide.md#2-설치)의 「수집기의 판」).

작업 폴더는 처음 한 번만 채웁니다. 그 뒤로는 화면에서 한 일이 그대로 남고, `ui.sh stop`도 폴더는 지우지 않습니다.
`down.sh`로 환경을 내리면 컨테이너와 함께 없어집니다. `Ctrl+C`는 중계만 닫으므로, 화면은 `ui.sh`를 다시 실행하면
그대로 이어서 볼 수 있습니다. 이 기계에서 다른 `pqcaton-ui`를 이미 8765로 띄웠다면 `PQCATON_UI_PORT=8766`처럼 다른
포트를 줍니다. 빌드에는 `extend.sh`와 같이 Go가 필요하고, 중계에는 python3가 필요합니다. Go가 없으면 linux용
`pqcaton-ui`를 `PQCATON_BIN_DIR`로 줍니다.

`extend.sh`는 새 컨테이너를 만들지 않습니다. 실행 중인 `pqcota-ctl`에 이 리포의 `pqcaton-report`를 주입하고,
core가 이미 수집한 `/work/results`에 **선언(declaration.json)**을 대조해 3-상태 인벤토리 + 거버넌스
토폴로지를 만들고, **그 결과를 판정해 계획을 만든 뒤, 상류의 승인 → 생성 → 적용 → 되돌림까지
실제로 돌립니다.**

**대조에서 멈추지 않습니다.** 관측 → 대조 → 판정 → 승인 → 생성 → 적용 → 되돌림이 한 바퀴로
이어집니다. 그 사이가 끊겨 있으면 데모에서 드러나야 합니다. 실제로 두 번 그랬습니다. v0.9.0 전에는
데모가 대조에서 멈춰 아무도 몰랐고, v0.16.0은 조치 종류를 고르지 않은 항목이 확정을 막게 됐는데
데모가 고르지 않아 `close`에서 끊긴 채 릴리스에 나갔습니다. 그리고 그 끊긴 데모를 다시 돌려서야
JVM 수집기의 결과 파일(`*.jsonl`)을 이 리포가 읽지 않고 있었다는 것이 드러났습니다.

**판정한 사람과 실행을 승인하는 사람이 다릅니다.** `pqcaton-decide close`가 내는 `/work/plan.json`은
`IN_REVIEW`이고 승인 칸이 비어 있습니다. 데모는 거버넌스 쪽 승인자(`governance-1`)가 자기 키를
만들어 등록하고 `pqcota-approve`로 `FINALIZED`로 올리는 것까지 보입니다. 판정자 표시(「데모 판정자」)는
판정 원장에 남고, 실행 승인 서명은 상류가 등록된 키로 검증합니다. 계획 id가 세션 id를 담으므로
`/work/judgments.jsonl`에서 그 세션의 판정을 되짚을 수 있습니다.

Go가 없는 호스트에서는 `PQCATON_BIN_DIR=<dir>`로 미리 빌드한 `pqcaton-report`·`pqcaton-decide`를 줍니다.

**되짚기 사슬을 재는 전용 픽스처**가 따로 있습니다. `PQCATON_E2E_TRACE=1`을 주면 선언에서 자산을
전부 빼서, 정책이 관리 대상으로 남긴 관측이 모두 UNDECLARED가 됩니다. 그래야 **이 리포가 만든 계획으로**
승인 → 생성 → 레코드 → 실제 스냅샷 id까지 사슬이 한 바퀴 돕니다. **데모 서사가 아니라 검증용입니다.**
없는 자산을 지어내는 대신 선언만 비우며, 이는 선언을 아직 작성하지 않은 조직에서 실제로 생길 수
있는 상태입니다. 기본 실행은 현실적인 선언 그대로 돌아, 정책이 관리 대상에서 뺀 자산에는 조치를
세우지 않는다는 것을 보입니다.

### 새 정책 활성화

02에서 확정한 정책을 대조에 쓰려면, 상류를 그 정책으로 다시 적재하고 활성화합니다. 데모에서는 한 명령으로 합니다.
화면이 떠 있으면 아무것도 하지 않으므로 먼저 끕니다.

```bash
./demo/scripts/ui.sh stop
```

```bash
./demo/scripts/ui.sh apply-scope
```

```bash
./demo/scripts/ui.sh
```

`apply-scope`는 차례로 다음을 합니다. 어느 단계에서 멈춰도 정책 · 판정 세션 · 계획 · 영수증은 그대로입니다.

1. 작업 폴더의 파일(02 확정 정책 · 02 세션 · 판정 세션 · 선언 · 결과), `pqcota-ingest`, 상류 DB, 지역 전제를 점검합니다.
   지역 전제는 판정 세션이 지금 정책으로 판정됐는지, 후보가 02 확정이 쓴 그대로인지, 두 정책이 다른지입니다.
2. 화면이 쓰는 `/work/ui/results`와 02 확정 정책으로 상류를 다시 적재합니다.
3. 새 정책으로 대조한 노드별 지문이 상류 이력에서 그 노드의 최신 스냅샷으로 **모두** 찾혀야 활성화합니다. 하나라도 빠지면
   아무것도 쓰지 않습니다. 과거 실행에 같은 지문이 있었다는 것만으로는 통과하지 않습니다.
4. 활성 정책 · 판정 세션 지문 · 잠금을 보입니다.

활성화하면 05가 새 정책으로 다시 세운 새 회차로 열리고, 지난 결론은 「지난 정책 결론」으로만 보입니다. 활성화 전에
확정한 계획은 06에서 서명 · 생성되지 않습니다.

### 종단 검증

`make e2e-demo`가 이 데모를 처음부터 띄워 정책 활성화를 끝까지 돌리고 실패 경로까지 잽니다(IC-U152). 상류 다섯 리포를
v0.10.7로 나란히 받아 두고 `PQCOTA_DIR`로 상류 `pqcota` 체크아웃을 줍니다.

```bash
PQCOTA_DIR=<상류 pqcota 체크아웃> make e2e-demo
```

상류 데모는 컨테이너 · 네트워크에 고정 이름(`pqcota-ctl` 등)을 씁니다. 그래서 이 검증은 실행마다 `pqcaton-e2e-…`라는
dind 컨테이너를 따로 띄우고, 상류 데모를 그 안의 Docker 데몬에 띄웁니다. 같은 호스트에서 pqcota 쪽이 데모를 띄워 쓰고
있어도 이름이 부딪히지 않고, 그 데모를 건드리지도 않습니다. 끝나면 실패했더라도 그 dind 컨테이너와 볼륨을 지웁니다.
dind 안의 데몬은 `docker exec`로 띄운 프로세스에 umask 0000을 물려주므로, 안쪽 실행에서는 그 umask를 호스트와 같은 022로
맞춥니다. 파일 권한 검사를 끄는 것이 아닙니다.
안쪽 데몬은 베이스 이미지를 처음부터 받으므로 첫 빌드가 오래 걸립니다. 호스트에 `docker:dind` 이미지가 있어야 하고, 필요한
프로그램이나 `PQCOTA_DIR`이 없을 때도 건너뛰지 않고 실패합니다.

우분투 기본 미러가 느린 곳에서는 `E2E_UBUNTU_MIRROR=<미러 이름>`(예: `kr.archive.ubuntu.com`)을 함께 줍니다. 그러면 dind 안의
빌드만 `archive.ubuntu.com` · `security.ubuntu.com`을 그 미러의 주소로 풉니다. 상류 Dockerfile은 고치지 않고, apt의 서명
검증도 그대로입니다.

`E2E_KEEP_WS=<폴더>`를 함께 주면 정리 전에(실패했더라도) 작업 폴더를 그 폴더로 꺼내 둡니다. 꺼낸 폴더는 컨테이너가 지워진 뒤에도
남으므로, 옛 판이 새 형식 영수증 앞에서 멈추는지 보는 시험에 그대로 쓸 수 있습니다.

## 요구 사항
- **pqcota 리포 체크아웃**: 선언을 그쪽 `topology.yaml`에서 만듭니다. 형제 디렉터리
  (`../pqcota`)에 두거나 `PQCOTA_DIR=/경로`로 알려 줍니다.
- **pqcota는 v0.10.7로 받습니다.** 상류 데모는 다섯 리포를 나란히 받아 한 이미지로 묶으므로, 다섯 리포를
  모두 같은 태그로 이 리포 옆에 받습니다. 다른 판을 받으면 상류가 다른 규칙 묶음으로 이력을 쌓아, 이 리포가
  만든 계획의 근거를 상류 이력에서 찾지 못할 수 있습니다. pqcaton 판마다 함께 쓰는 판은
  [판별 표](../docs/compatibility.md)에 있습니다.

  ```bash
  for r in pqcota pqcota-common pqcota-inventory pqcota-discovery pqcota-provisioning; do git clone --branch v0.10.7 https://github.com/randyinthedev-hash/$r; done
  ```
- 실행 중인 pqcota 디스커버리 데모(위 1단계).
- 빌드 기계에는 **Go**와 **python3**만 있으면 됩니다. pqcota v0.5.0부터 모듈 경로가 리포 주소와 같아져
  `go build`가 계약을 스스로 받아옵니다. 형제 체크아웃도 `replace`도 필요 없습니다.
- **Graphviz(`dot`)는 이 기계에 없어도 됩니다.** 토폴로지 SVG는 `pqcota-ctl` 컨테이너 안에서
  그립니다. 없으면 DOT 원문만 회수하고 나머지는 그대로 됩니다
  ([사전 준비](../README.md)).

## 바꿔 보기

| 무엇을 바꾸나 | 어디를 고치나 |
|---|---|
| **환경**: 노드·망·버전·provider | pqcota의 `demo/topology/topology.yaml`. 고친 뒤 `up.sh`부터 다시 |
| **선언 규칙**: 무엇을 선언하고 무엇을 뺄지 | [`scripts/declare.py`](scripts/declare.py)의 `ASSETS`·`DECLARED_PROTOS` |

예를 들어 `DECLARED_PROTOS`에 `ssh`를 넣으면 UNDECLARED가 사라지고 전부 CONFIRMED가 됩니다.
**선언이 완벽한 조직**이 어떻게 보이는지가 그것입니다.
