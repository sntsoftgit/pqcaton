# 예상 결과 — 확장 샘플

`../scripts/extend.sh`(실행 중인 pqcota 디스커버리 데모 위)를 돌리면 나오는 **대표 결과**입니다.
관측 등급(core) 위에 **선언 대비 3-상태 대조 + 리뷰 큐 + 거버넌스 토폴로지**가 얹힙니다.

| 파일 | 내용 |
|---|---|
| [report.txt](report.txt) | 콘솔 — 자산 3-상태(CONFIRMED/UNDECLARED shadow/UNOBSERVED) + 리뷰 큐 + 엣지 대조 |
| [topology-governance.svg](topology-governance.svg) | 거버넌스 토폴로지 (색=등급, 선형=상태: 실선 CONFIRMED / 굵은선 shadow / 점선 UNOBSERVED) |

핵심 서사(선언 대비):
- 🟢 `node-web→node-app` MLKEM **CONFIRMED** · 🔴 `node-web→node-db` 고전 **CONFIRMED**
- 🟢 `node-web→node-db` SSH sntrup761 **UNDECLARED(shadow)** — 선언 안 된 통신
- ⚪ `node-db→node-app` **UNOBSERVED** — 선언했으나 미관측(≠부재)

## 실제 실행 시 달라질 수 있는 점 (그리고 이유)

- **엣지 대조 구성**: core가 관측한 엣지 집합을 그대로 대조합니다. core `demo.sh`가 **retry-until-complete**로
  목표 엣지까지 재수집하므로(첫 실행도 완전) 확장 결과의 엣지도 일관됩니다. 드물게 `node-app→node-db`가
  추가로 관측되면 shadow 엣지가 하나 더 나타납니다(관측 구간 의존).
- **버전·IP**: [core expected-output](../../../pqcota/demo/expected-output/README.md)과 동일 —
  base 이미지 digest 핀으로 버전 문자열 고정, IP만 매 실행 동적(서사 무관).
- **선언 편집**: `../declaration.json`(고객 선언)을 바꾸면 CONFIRMED/UNDECLARED/UNOBSERVED 분포가 달라집니다.

## 결정론

같은 관측·선언이면 3-상태 대조·confidence·리뷰 큐는 항상 동일합니다. 위 "차이"는 core가 관측한 입력의
차이(캡처 타이밍)이지, 대조 로직의 비결정성이 아닙니다.
