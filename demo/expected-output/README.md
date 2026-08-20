# 예상 결과 — 확장 샘플

`../scripts/extend.sh`(실행 중인 pqcota 디스커버리 데모 위)를 돌리면 나오는 **대표 결과**입니다.
관측 등급(pqcota) 위에 **선언 대비 3-상태 대조 + 리뷰 큐 + 거버넌스 토폴로지**가 얹힙니다.

| 파일 | 내용 |
|---|---|
| [report.txt](report.txt) | 콘솔 — ① 관측(무엇을 보았고 **무엇을 못 봤나**) → ② 자산 3-상태 + 리뷰 큐 → ③ 엣지 대조 |
| [topology-governance.svg](topology-governance.svg) | 거버넌스 토폴로지 (색=등급, 선형=상태: 실선 CONFIRMED / 굵은선 shadow / 점선 UNOBSERVED) |
| [plan.json](plan.json) | **확정 계획** — 판정을 거쳐 나온 것. `pqcota-provision` 의 입력입니다 |

**대조에서 끝나지 않습니다.** 데모는 판정을 거쳐 `plan.json` 까지 만듭니다 — 그 사이가 끊겨
있으면 여기서 드러나야 합니다. v0.9.0 전에는 데모가 대조에서 멈춰, **관측한 노드를 판정할
길이 없다는 사실을 아무도 몰랐습니다.** 계획이 겨누는 것은 `pay-app`·`web-gw` 로, 실제로
관측한 노드입니다.

**이 파일들은 손으로 만들지 않습니다.** `extend.sh` 가 실행되면서 그대로 갖고 옵니다 —
`report.txt` 만 한동안 손으로 만들어 둔 것이라, 명령의 출력이 영어가 된 날에도 한국어인
채 남아 있었습니다(2026-08-19에 고쳤습니다).

**①이 먼저 나오는 이유** — pqcota 데모를 거치지 않고 이 리포트만 보는 사람에게, 대조 앞에
무엇이 있었는지가 보여야 합니다. 특히 「못 본 계층」이 없으면 ②의 UNOBSERVED가 「없다」인지
「원리상 못 봤다」인지 읽는 사람이 가를 수 없습니다.

핵심 서사(선언 대비):
- 🟢 `web-gw→pay-app` MLKEM **CONFIRMED** · 🔴 `web-gw→pay-db` 고전 **CONFIRMED**
- 🟢 `web-gw→pay-app` SSH sntrup761 · 🔴 `web-gw→pay-db` SSH curve25519 — **둘 다 UNDECLARED(shadow)**.
  아무도 선언하지 않은 관리 통신이 실제로 돌고 있습니다
- ⚪ `pay-app→web-gw` **UNOBSERVED** — 선언했으나 미관측(≠부재). 대장에는 양방향으로
  적혀 있는데 트래픽은 한 방향인, 흔한 모양입니다

> **이 결과는 pqcota 데모의 기본 토폴로지**(`demo/topology/topology.example.yaml`)에서 나온
> 것입니다. **선언도 그 파일에서 만듭니다**([`declare.py`](../scripts/declare.py)) — 노드를
> 늘리거나 이름을 바꿔도 우리 쪽은 손대지 않고 그대로 따라갑니다. 실제로 노드를 하나 더해
> 확인했습니다: 노드 3→4, CONFIRMED 4→6.

## 실제 실행 시 달라질 수 있는 점 (그리고 이유)

- **엣지 대조 구성**: core가 관측한 엣지 집합을 그대로 대조합니다. core `demo.sh`가 **retry-until-complete**로
  목표 엣지까지 재수집하므로(첫 실행도 완전) 확장 결과의 엣지도 일관됩니다. 캡처 구간에 따라
  shadow 엣지가 하나 더 나타날 수 있습니다.
- **버전·IP**: [core expected-output](../../../pqcota/demo/expected-output/README.md)과 동일 —
  base 이미지 digest 핀으로 버전 문자열 고정, IP만 매 실행 동적(서사 무관).
- **선언 규칙**: [`../scripts/declare.py`](../scripts/declare.py)의 `DECLARED_PROTOS`를 바꾸면
  분포가 달라집니다. `ssh`를 넣으면 shadow가 사라집니다 — 대장이 완벽한 조직의 모습입니다.

## 결정론

같은 관측·선언이면 3-상태 대조·confidence·리뷰 큐는 항상 동일합니다. 위 "차이"는 core가 관측한 입력의
차이(캡처 타이밍)이지, 대조 로직의 비결정성이 아닙니다.
