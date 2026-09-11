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

이 데모는 **독립 스택이 아니라 확장**입니다. [pqcota의 디스커버리 데모](https://github.com/randyinthedev-hash/pqcota/tree/main/demo)를
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

# 3) 정리 (core 데모가 환경 소유)
../../pqcota/demo/scripts/down.sh
```

`extend.sh`는 새 컨테이너를 만들지 않습니다. 실행 중인 `pqcota-ctl`에 이 리포의 `pqcaton-report`를 주입하고,
core가 이미 수집한 `/work/results`에 **선언(declaration.json)** 을 대조해 3-상태 인벤토리 + 거버넌스
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

## 요구 사항
- **pqcota 리포 체크아웃**: 선언을 그쪽 `topology.yaml`에서 만듭니다. 형제 디렉터리
  (`../pqcota`)에 두거나 `PQCOTA_DIR=/경로` 로 알려 줍니다.
- 실행 중인 pqcota 디스커버리 데모(위 1단계).
- 빌드 기계에 **Go**와 **python3**. 그게 전부입니다. pqcota v0.5.0부터 모듈 경로가 리포 주소와 같아져
  `go build`가 계약을 스스로 받아옵니다. 형제 체크아웃도 `replace`도 필요 없습니다.
- **Graphviz(`dot`)는 이 기계에 없어도 됩니다.** 토폴로지 SVG는 `pqcota-ctl` 컨테이너 안에서
  그립니다. 없으면 DOT 원문만 회수하고 나머지는 그대로 됩니다
  ([사전 준비](../README.md)).

## 바꿔 보기

| 무엇을 바꾸나 | 어디를 고치나 |
|---|---|
| **환경**: 노드·망·버전·provider | pqcota의 `demo/topology/topology.yaml`. 고친 뒤 `up.sh`부터 다시 |
| **선언 규칙**: 무엇을 선언하고 무엇을 뺄지 | [`scripts/declare.py`](scripts/declare.py)의 `ASSETS`·`DECLARED_PROTOS` |

예를 들어 `DECLARED_PROTOS`에 `ssh`를 넣으면 UNDECLARED 가 사라지고 전부 CONFIRMED가 됩니다.
**선언이 완벽한 조직**이 어떻게 보이는지가 그것입니다.
