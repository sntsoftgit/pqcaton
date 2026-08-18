# 테스트케이스 — 인벤토리 거버넌스

[개요](../README.md) · [릴리스 노트](../RELEASE_NOTES.md) · [여정](journey.md) · [설계](design.md) · **검증 기준** · [데모](../demo/README.md) · [구조 그림](https://www.sntsoft.co.kr/pqcaton/)

케이스 번호가 곧 테스트 파일 링크입니다. 설계는 [design.md](design.md)에 있습니다.

**문서 성격**: [설계](design.md) §1(인벤토리 엔진)를 **검증 가능한 인수 기준**으로 전개합니다.
구현은 이 테스트를 통과하는 것을 목표로 합니다 — `pkg/inventory/{reconcile,decision}`, `inventory/cmd/pqcaton-reconcile`.
**검증 대상**: 3-상태 reconciliation·confidence·리뷰 큐·리뷰-확정 상태기계·판정 영속화·확정 계획·핸드오프 게이트·엣지 대조.
**실행 방침**: 대조·상태기계·게이트는 순수 로직(실물 불필요), 판정 영속화는 Postgres 통합.

> 적재·이력·보존·자산 스코프 등 **pqcota가 구현하는 부분**의 인수 기준은
> [pqcota 인벤토리 테스트케이스](https://github.com/randyinthedev-hash/pqcota/blob/main/inventory/testcases.md)에 있습니다.

---

## 1. 테스트케이스

### R. 3-상태 대조 (§3.3①) ✅
| TC | Given → When | Then |
|---|---|---|
| IC-R1 ✅ | 자산이 선언 ∩ 관측 | CONFIRMED |
| IC-R2 ✅ | 관측 only(선언 안 됨) | **UNDECLARED**(shadow), NeedsReview |
| IC-R3 ✅ | 선언 only(관측 안 됨) | UNOBSERVED, NeedsReview(기계 확정 불가) |
| IC-R4 ✅ | UNOBSERVED + 완전성 맵에 해당 계층 **갭** | "재수집 후보"로 표시(갭이면 미관측일 뿐). 갭 아니면 실존/stale 사람 판정(§3.3) |

### C. confidence 스코어링 (§3.5) — 부분
| TC | Given → When | Then |
|---|---|---|
| IC-C1 ✅ | 상태별 | CONFIRMED > UNDECLARED > UNOBSERVED |
| IC-C2 ✅ | 관측 finding의 evidence_strength=inferred-low | confidence 상한 하향(불확실 관측은 신뢰 낮춤) |
| IC-C3 ⏳ | 관측빈도·기간·선언신선도 실측 | f(...) 캘리브레이션(설계 §11, Phase 1 데이터 필요) |

### Q. 리뷰 큐 (§3.3②) ✅ / 부분
| TC | Given → When | Then |
|---|---|---|
| IC-Q1 ✅ | CONFIRMED + 고신뢰 | 자동통과 후보(일괄 승인 제안, 승인은 사람) |
| IC-Q2 ✅ | UNDECLARED(shadow) | 필수 개별 리뷰, **최우선** |
| IC-Q3 ✅ | 저신뢰 CONFIRMED | 필수 개별 리뷰 |
| IC-Q4 🔜 | 우선순위 = 위험도 × 블라스트반경 × 데이터민감도 | 실측 프록시(현재 상태 기반 P1~3은 임시) |

### F. 리뷰-확정 상태기계 (§3.3③, §6) ✅ — 핵심
| TC | Given → When | Then |
|---|---|---|
| IC-F1 ✅ | 신규 판정 대상 | 상태 = **draft** |
| IC-F2 ✅ | draft → in-review 전이 | 허용 |
| IC-F3 ✅ | in-review에서 finalize (전 필수항목 판정 + 승인 서명 有) | **finalized** |
| IC-F4 ✅ | 필수 항목 미판정 상태로 finalize 시도 | **거부**(전 필수 판정 전 불가) |
| IC-F5 ✅ | 승인 서명 없이 finalize | **거부** |
| IC-F6 ✅ | 링/도메인 단위 부분 확정 | 허용(부분 finalize, §3.3③) |
| IC-F7 ✅ | 정책 단위 판정(버전×링크모드 템플릿) | 동종 자산 일괄 적용(§3.4) / 예외만 엣지 단위 |

### D. 판정 영속화 (§3.6, §7) ✅ — Postgres
| TC | Given → When | Then |
|---|---|---|
| IC-D1 ✅ | 판정 후 재수집(새 스냅샷) | 판정(인간 결론) **부착 유지** — 엣지 상태가 바뀌어도 결론은 남음 |
| IC-D2 ✅ | 근거 증거(BasisHash) 실질 변화 | **해당 판정만** 재검토 플래그(델타 리뷰), 나머지 유지 |
| IC-D3 ✅ | 근거 불변 | 판정 유지(재리뷰 안 함) |
| IC-D4 ✅ | stale 판정 + 만료 경과 | 신뢰도 감쇠 + 주기 재확인 플래그 |
| IC-D5 ✅ | 영속화(Postgres) 라운드트립 | Decision 보존(append-only, §0.2) |

### P. 확정 계획 & 핸드오프 (§3.7, §5, §8) ✅
| TC | Given → When | Then |
|---|---|---|
| IC-P1 ✅ | finalized 계획 생성 | PlanItem[]: node·remediation_class·**deploy_automation_level**·provider_choice |
| IC-P2 ✅ | deploy_automation_level 판정 | 자산별 리뷰어 판정(§4.5 MANUAL) — 전사 일괄 아님 |
| IC-P3 ✅ | 규제 대상 자산(fips_validation 요구) | **FIPS 검증 provider로 라우팅 강제**(§4.10, Java=BC-FJA) |
| **IC-P4 ✅** | **finalized 아닌 계획을 Deploy로** | **실행 거부**(§5 최강 게이트) — 핵심 인수 기준 |
| IC-P5 ✅ | finalized 계획 | 프로비저닝의 **유일** 실행 근거(§3.7) |
| **IC-C1 ✅** | 스코프가 URI인 노드(`host://local`)를 계약 형식으로 | **겨눈 노드와 런타임이 그대로 간다** — v0.1.0은 id를 쪼개 `host:`를 겨누고 런타임을 기본값으로 떨어뜨렸다 |
| IC-C2 ✅ | node가 빈 항목을 계획에 | 확정 직전에 거부하고 `open`을 다시 돌리라고 말한다 — 이름 없는 노드에 조치를 걸지 않는다 |

### E. 통신 엣지 reconciliation & 토폴로지 (§12) 🔶 — 엔진·렌더·저장 완료(unit); 라이브 관측은 network-collector(§2.5)가 공급
| TC | Given → When | Then |
|---|---|---|
| IC-E1 ✅ | 관측 엣지(TLS/SSH 협상) vs 선언 엣지 | 엣지 3-상태(CONFIRMED/UNDECLARED shadow/UNOBSERVED) + 등급 부착 |
| IC-E2 ✅ | 토폴로지 렌더 | 색=등급(🟢PQC/🔴취약/⚪불명), 미관측=점선(≠부재, §12.2 정직성) |
| IC-E3 ✅ | 스코프 밖 관측 상대 | off-scope 표기 "등재 판정 요청"(§0.4/§5) |

> **구현 위치**: 엣지 대조 `reconcile/edge.go`(없음) · 등급 분류 `pkg/kernel/posture/` · 토폴로지 DOT `reconcile/topology.go`(없음) · 저장 `pkg/discovery/history`(Snapshot.Edges, Postgres `edges` JSONB). 관측 엣지 스키마 `contracts` `ObservedEdge`(CollectionResult.observed_edges). 이 계약을 채우는 **network-collector(디스커버리 §2.5, AF_PACKET)가 라이브 관측을 공급합니다**(대조 엔진은 합성 데이터로도 검증됩니다).

---

## 2. 구현 순서 (pure 먼저)

| # | 대상 | TC | 상태 |
|---|---|---|---|
| 1 | 3-상태 대조 + 리뷰 큐 + confidence | R1~3, Q1~3, C1 | ✅ |
| 2 | **리뷰-확정 상태기계** | F1~7 | ✅ pure |
| 3 | **확정 계획 + 핸드오프 게이트**(finalized-only) | P1~5 | ✅ pure |
| 4 | UNOBSERVED×완전성 갭 연동 + evidence_strength confidence | R4, C2 | ✅ pure |
| 5 | **판정 영속화**(Decision 저장소, Postgres) + 델타 리뷰 | D1~5 | ✅ integration |
| 6 | 통신 엣지 reconciliation + 토폴로지 | E1~3 | 🔶 unit ✅ / 라이브 관측은 pqcota의 network collector |

**핵심 인수 기준**: **IC-P4**(finalized 아니면 Deploy 거부 — 최강 게이트)와 **IC-F3~F5**(승인 서명·전 필수 판정 없으면 확정 불가).

## 3. 데이터 모델 매핑 (구현 위치)

| TC 그룹 | 구현 위치 |
|---|---|
| 대조·큐·confidence | `pkg/inventory/reconcile/` |
| 상태기계·확정 계획·게이트 | `pkg/inventory/decision/` |
| 판정 영속화 | `pkg/inventory/decision/` + pqcota `pkg/discovery/history`의 PgStore 패턴 |
| 엣지·토폴로지 | `pkg/inventory/reconcile/{edge,topology}.go` |
| Decision·FinalizedPlan 스키마 | pqcota `contracts/` (공개 스키마) |
