# 설계 — 인벤토리 거버넌스

pqcota가 **관측**하고, 이 리포가 **그 관측을 판정으로 잇는다.**

pqcota는 관측한 사실만 낸다 — 무엇이 위험한지, 무엇을 먼저 바꿀지 판정하지 않는다.
[그 리포가 명시적으로 뺀 것](https://github.com/randyinthedev-hash/pqcota/blob/main/docs/architecture.md)이
선언(CMDB) 대조·confidence 스코어링·리뷰 큐·확정 거버넌스이고, 이 문서가 그것들을 다룬다.

> **§ 표기**: 별도 언급이 없으면 pqcota
> [규정서](https://github.com/randyinthedev-hash/pqcota/blob/main/docs/regulation.md)의 절 번호다.

---

## 1. 인벤토리 엔진 — 대조·판정·거버넌스

pqcota는 관측을 모아 **읽기전용 뷰**와 `plan`·`decision` 스키마까지 낸다.
그 위에서 무엇을 확정할지 정하는 엔진이 여기 있다.

### 1.1 대조 엔진 — 3-상태 reconciliation (규정서 §3.3①)
각 통신 엣지를 선언∩관측으로 분류. 결정론 부분은 AUTO, 모호 부분은 MANUAL로 강등.

| 상태 | 정의 | 등급 | 의미 |
|---|---|---|---|
| **CONFIRMED** | 선언 ∩ 관측 | AUTO | 신뢰도 최상 |
| **UNDECLARED** | 관측 only | AUTO | **shadow 통신** — 보안 최우선 발견 |
| **UNOBSERVED** | 선언 only | **MANUAL** | 실존(DR/배치) vs stale vs 커버리지 갭 — 기계 확정 불가 |

- UNOBSERVED의 핵심: pqcota의 **완전성 맵**(관측하지 못함 ≠ 없음)이 결정적 — "관측 안 됨"이 "실제 없음"인지 "원리상 못 봄(갭)"인지 갈린다. 갭이면 재수집, 아니면 사람 판정.
- 입력: 관측 엣지(network-collector TLS 등급 + CBOM), 선언 엣지(declaration-importer). evidence_strength가 confidence로 전파.

### 1.2 confidence 스코어링 (§3.5)
```
confidence = f(관측빈도, 관측기간, 선언신선도, 소스일치도, evidence_strength)
```
AUTO·결정론(파생 뷰). 리뷰 큐 우선순위·자동통과 후보 선별의 입력. `evidence_strength`(confirmed/inferred-*)가 1급 가중치 — inferred-low는 confidence 상한을 누른다.

### 1.3 리뷰 큐 (§3.3②) — PROPOSE
대조 엔진은 정답이 아니라 판정 대상을 구조화한다.
- **자동통과 후보**: CONFIRMED + 고신뢰 + 저위험 → 일괄 승인 묶음(승인은 사람).
- **필수 개별 리뷰**: UNDECLARED, 저신뢰 UNOBSERVED, 레거시 터치 필요, 컷오버가 상대방을 깨는 엣지.
- **우선순위 = 위험도 × 블라스트반경 × 데이터민감도.**

### 1.4 리뷰-확정 상태기계 (§3.3③) — MANUAL
```
draft ──▶ in-review ──▶ finalized
                         └ 전 필수항목 판정 + 승인 서명 (전제)
```
- **finalized 전에는 프로비저닝 실행 불가**(§3.7 최강 게이트). 링/도메인 단위 부분 확정 허용.
- **리뷰 granularity(§3.4, 하이브리드)**: 정책 단위 기본(버전×링크모드·JDK×provider 카탈로그가 리뷰 대상 정책 템플릿 → 동종 자산 일괄), 개별 격리 예외(정책 예외·고위험·shadow 엣지만 엣지 단위).

### 1.5 판정 영속화 (§3.6)
- 판정 = 엣지 상태가 아니라 "인간의 결론" → 재수집에도 부착 유지(`Decision.BasisHash`로 근거 추적).
- **무효화 트리거**: 근거 증거 실질 변화 시 해당 판정만 재검토 플래그 → **델타 리뷰**(전면 재리뷰 아님).
- stale 판정 만료: 신뢰도 감쇠·주기 재확인. 이 판정 이력이 provenance chain의 **판단 계열**(§0.3).

> 스키마(`Decision`·`FinalizedPlan`·`ReconState`)는 [pqcota의 계약](https://github.com/randyinthedev-hash/pqcota/tree/main/contracts)이 SSOT다.
> 이 리포는 그 어휘로 말하고, 위 엔진만 여기서 만든다.

---

### 1.6 자산 스코프 거버넌스 (2026-07-21)

pqcota가 **메커니즘**을 갖는다: `scope.AssetPolicy`(CSV 규칙, glob) — 노드를 등재해도 그 안에서
**무엇을 계속 관리할지**를 사용자가 선언하고, `pqcota-ingest -scope-assets`가 적재 전에 집행한다.
제외분은 `Snapshot.ExcludedByScope`로 세어 고지된다(제외 ≠ 부재, §2.7). 잡음을 못 거르면 인벤토리 자체가 못 쓰게 되므로,
이건 관측 도구가 스스로 갖춰야 한다 — pqcota 없이 이 리포만으로는 아무것도 못 한다는 뜻이 아니라,
그 반대다: **pqcota는 혼자서도 끝까지 된다.**

이 리포가 더하는 건 그 위의 **거버넌스와 규모**다:

| 축 | 이 리포가 더하는 것 | 왜 조직에서만 필요한가 |
|---|---|---|
| **리뷰-확정** | 스코프 변경(특히 exclude 추가)을 §2.4 상태기계에 태워 제안→검토→승인 | "이 자산은 안 본다"는 결정은 **감사 대상**이다. 혼자면 자기 책임이지만 조직은 근거·승인자를 남겨야 한다 |
| **감사 추적** | 누가·언제·왜 제외했고 그때 무엇이 빠졌는지 판정 영속(§2.5)에 기록 | 사고 후 "왜 이게 인벤토리에 없었나"에 답해야 한다 |
| **정책 상속·일괄** | 조직→환경(prod/dev)→노드군 계층 정책, 수천 대 일괄 적용·드리프트 검출 | CSV 한 장은 20대까진 되지만 5000대에선 관리 불가 |
| **제외분 재검토** | 제외된 자산을 주기적으로 다시 훑어 "빼둔 사이 위험해진 것"을 리뷰 큐로 | 제외는 영구 면제가 아니다. 이 판정이 곧 §2.1 대조의 확장 |

**경계 요약**: 규칙의 **정의·집행은 pqcota**, 규칙 변경의 **승인·감사·규모 운영은 여기**다.
이 리포는 pqcota의 `AssetPolicy`를 재구현하지 않고 **생성·배포**한다 — 거버넌스가 확정한
정책을 CSV·계약으로 내려보내면 pqcota의 집행기가 그대로 쓴다.

**구현 상태**: 설계만. pqcota 쪽 메커니즘은 완료.

---
