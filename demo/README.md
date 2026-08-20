# 데모 — pqcota 위에 얹는 거버넌스

[개요](../README.md) · [릴리스 노트](../RELEASE_NOTES.md) · [여정](../docs/journey.md) · [설계](../docs/design.md) · [검증 기준](../docs/testcases.md) · **데모** · [구조 그림](https://www.sntsoft.co.kr/pqcaton/)

📊 **실행 전 예상 결과**: [`expected-output/`](expected-output/) — 확장 리포트·거버넌스 토폴로지 샘플 + 차이점 설명.

## 이 데모가 세우는 환경

pqcota 데모를 처음 보는 사람을 위해 적습니다. **결제 서비스를 흉내 낸 3노드**가 도커로 뜨고,
그 위에서 관측이 이뤄집니다.

| 노드 | 무엇이 도나 | 왜 그렇게 두었나 |
|---|---|---|
| `web-gw` | 최신 OpenSSL 3 · 클라이언트 | 트래픽을 보내는 쪽. SSH 등급은 클라이언트가 가릅니다 |
| `pay-app` | JVM(JCA provider를 런타임에 등록) | **정적 스캔으로는 안 보이고** attach로만 잡힙니다 |
| `pay-db` | 레거시 OpenSSL 1.1.1 · 서버 · 앱 둘이 한 libssl 공유 | 양자취약이 어디까지 번지는지(영향 반경) |

망은 둘로 갈라져 있습니다 — `corp`(웹·앱·DB가 닿는 곳)와 `db`(격리 tier). 그래서 어떤 통신은
관측되고 어떤 통신은 원리상 안 보입니다. **그 차이가 「없다」와 「못 봤다」를 가릅니다.**

환경 자체는 pqcota의 `demo/topology/topology.yaml` 하나가 정의합니다. 그 파일을 고치면 노드와
망이 달라지고, **선언은 거기서 만들어지므로 우리 쪽은 손댈 것이 없습니다**(아래).

## 무엇을 얹나

이 데모는 **독립 스택이 아니라 확장**입니다. [pqcota의 디스커버리 데모](https://github.com/randyinthedev-hash/pqcota/tree/main/demo)를
그대로 띄운 뒤, 그 위에 이 리포의 기능 — **선언 대비 3-상태 대조(CONFIRMED/UNDECLARED shadow/UNOBSERVED) +
리뷰 큐 + 거버넌스 토폴로지** — 를 얹습니다.

```
관측 등급 (pqcota)              →   + 선언 대비 reconciliation (pqcaton)
🟢 web-gw→pay-app MLKEM                🟢 web-gw→pay-app  TLS  CONFIRMED
🔴 web-gw→pay-db  고전                 🔴 web-gw→pay-db   TLS  CONFIRMED
🟢 web-gw→pay-app SSH sntrup761        🟢 web-gw→pay-app  SSH  UNDECLARED(shadow) ← 선언 안 된 통신!
🔴 web-gw→pay-db  SSH curve25519       🔴 web-gw→pay-db   SSH  UNDECLARED(shadow)
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
토폴로지를 만들고, **그 결과를 판정해 확정 계획까지** 만듭니다.

**대조에서 멈추지 않습니다.** 관측 → 대조 → 판정 → 확정 계획이 한 바퀴로 이어집니다 — 그 사이가
끊겨 있으면 데모에서 드러나야 합니다(v0.9.0 전에는 데모가 대조에서 멈춰 아무도 몰랐습니다).
나온 `/work/plan.json` 은 `pqcota-provision` 의 입력입니다.

## 요구 사항
- **pqcota 리포 체크아웃** - 선언을 그쪽 `topology.yaml`에서 만듭니다. 형제 디렉터리
  (`../pqcota`)에 두거나 `PQCOTA_DIR=/경로` 로 알려 줍니다.
- 실행 중인 pqcota 디스커버리 데모(위 1단계).
- 빌드 머신에 **Go**와 **python3**. 그게 전부입니다 — pqcota v0.5.0부터 모듈 경로가 리포 주소와 같아져
  `go build`가 계약을 스스로 받아옵니다. 형제 체크아웃도 `replace`도 필요 없습니다.
- **Graphviz(`dot`)는 이 기계에 없어도 됩니다.** 토폴로지 SVG는 `pqcota-ctl` 컨테이너 안에서
  그립니다. 없으면 DOT 원문만 회수하고 나머지는 그대로 됩니다
  ([사전 준비](../README.md)).

## 바꿔 보기

| 무엇을 바꾸나 | 어디를 고치나 |
|---|---|
| **환경** - 노드·망·버전·provider | pqcota의 `demo/topology/topology.yaml`. 고친 뒤 `up.sh`부터 다시 |
| **선언 규칙** - 무엇을 선언하고 무엇을 뺄지 | [`scripts/declare.py`](scripts/declare.py)의 `ASSETS`·`DECLARED_PROTOS` |

예를 들어 `DECLARED_PROTOS`에 `ssh`를 넣으면 shadow가 사라지고 전부 CONFIRMED가 됩니다 -
**대장이 완벽한 조직**이 어떻게 보이는지가 그것입니다.
