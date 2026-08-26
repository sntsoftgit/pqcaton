# 테스트케이스: 인벤토리 거버넌스

[개요](../README.md) · [릴리스 노트](../RELEASE_NOTES.md) · [여정](journey.md) · [설계](design.md) · **검증 기준** · [데모](../demo/README.md) · [구조 그림](https://www.sntsoft.co.kr/pqcaton/)

케이스 번호가 곧 테스트 파일 링크입니다. **그 대응은 `make check-cases` 가 잽니다**
(§M). 설계는 [design.md](design.md)에 있습니다.

**문서 성격**: [설계](design.md) §1(인벤토리 엔진)을 **검증할 수 있는 인수 기준**으로 풀어 적습니다.
구현은 이 테스트를 통과하는 것을 목표로 합니다. `pkg/inventory/*` 와 `inventory/cmd/*`.
**검증 대상**: 3-상태 대조·신뢰도·리뷰 큐·리뷰-확정 상태기계·판정 영속화·확정 계획·핸드오프 관문·엣지 대조.
**실행 방침**: 대조·상태기계·관문은 순수 로직이라 실물이 필요 없고, 판정 영속화와 행 수준 보안은 Postgres 통합.

> 적재·이력·보존·자산 스코프 등 **pqcota가 구현하는 부분**의 인수 기준은
> [pqcota 인벤토리 테스트케이스](https://github.com/randyinthedev-hash/pqcota/blob/main/inventory/testcases.md)에 있습니다.

---

## 1. 테스트케이스

### R. 3-상태 대조 (§3.3①) ✅
| TC | Given → When | Then |
|---|---|---|
| [IC-R1](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 자산이 선언 ∩ 관측 | CONFIRMED |
| [IC-R2](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 관측 only(선언 안 됨) | **UNDECLARED**, NeedsReview |
| [IC-R3](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 선언 only(관측 안 됨) | UNOBSERVED, NeedsReview(기계 확정 불가) |
| [IC-R4](../pkg/inventory/reconcile/edge_test.go) ✅ | UNOBSERVED + 완전성 맵에 해당 계층 **갭** | "재수집 후보"로 표시(갭이면 미관측일 뿐). 갭 아니면 실존/stale 사람 판정(§3.3) |

### O. 대조의 조직 축 (설계 §1.1) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-O1](../pkg/inventory/reconcile/reconcile_test.go) ✅** | 다른 조직의 자산이 선언·관측 어느 레인에든 섞임 | **대조하지 않고 중단한다**(`ErrOrgMismatch`). 그냥 두면 오류가 아니라 그럴듯한 결과가 나온다 |
| [IC-O2](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 식별자에 조직이 빔 / 빈 조직으로 엔진 열기 | 둘 다 거부한다. 빈 것은 「아무 조직」이 아니라 「모른다」다 |
| [IC-O3](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 스냅샷에서 관측 자산을 뽑음 | 엔진이 조직을 찍는다. 찍는 자리가 하나여야 조직 없는 식별자가 안 생긴다 |
| [IC-O4](../pkg/inventory/reconcile/edge_test.go) ✅ | 다른 조직의 선언 엣지 | 자산과 같은 규칙으로 중단한다 |
| [IC-O5](../pkg/inventory/reconcile/edge_test.go) ✅ | 관측 엣지(조직 없음) | 엔진이 찍는다. 안 찍으면 선언과 영영 안 맞아 전부 UNDECLARED 로 올라온다 |

### L. 행 수준 보안 (설계 §2.2) ✅: Postgres
| TC | Given → When | Then |
|---|---|---|
| **[IC-L1](../pkg/inventory/decision/rls_test.go) ✅** | 앱 롤로 붙어 **org 조건 없는 날것의 질의** (핸들 격리가 뚫린 상황) | 남의 조직 행이 **0건**이다. DB가 막는다. 이 케이스가 통과하지 않으면 더한 한 겹은 없는 것이다 |
| [IC-L2](../pkg/inventory/decision/rls_test.go) ✅ | 남의 조직 이름으로 INSERT | 거부한다(`WITH CHECK`). 읽기만 막으면 안 보이는 행이 들어와 쌓인다 |
| [IC-L3](../pkg/inventory/decision/rls_test.go) ✅ | 자기 조직 판정 조회 | 그대로 보인다. 막는 것만 재면 전부 막아도 통과한다 |
| **[IC-L4](../pkg/inventory/decision/rls_test.go) ✅** | 슈퍼유저로 붙고 `PQCATON_REQUIRE_RLS=1` | **저장소가 열리지 않는다**. 걸어 놓고 안 무는 것이 가장 위험한 거짓 안심이다 |

> **실행**: `PQCOTA_TEST_DSN` 이 있을 때만 실행됩니다. 케이스가 슈퍼유저 DSN으로 앱 롤(`NOSUPERUSER NOBYPASSRLS`)을
> 만들어 그 롤로 다시 붙습니다. **슈퍼유저로는 무엇을 걸어도 통과하므로 그 조건에서는 아무것도 재지 못합니다.**

### LS. 로컬 스캔의 정직성 (`pkg/inventory/localscan`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-L5](../pkg/inventory/localscan/localscan_test.go) ✅** | `/proc` 을 열 수 없음(비-리눅스) | **중단하고 대안을 알려 준다**. 그 상태로 대조하면 선언 자산이 전부 UNOBSERVED 로 나오고 리포트에 「관측이 완전합니다(`not seen: nothing`)」라고까지 적힌다 |
| [IC-L6](../pkg/inventory/localscan/localscan_test.go) ✅ | `/proc` 은 열렸으나 접근 가능 0(권한) | 결과는 보여 주되 **「없는 것이 아니라 못 본 것」**이라고 알려 준다 |
| [IC-L7](../pkg/inventory/localscan/localscan_test.go) ✅ | 정상 스캔 | 조용하다. 막는 것만 재면 전부 막아도 통과한다 |
| **[IC-L8](../pkg/inventory/localscan/localscan_test.go) ✅** | 기본이 아닌 노드 이름을 줌 | **경고한다**. 이름표일 뿐 관측 대상이 아니다. 이름이 맞으면 대조까지 되어 **다른 기계의 관측으로 CONFIRMED 가 나온다** |

### S. 자산 스코프 거버넌스 (설계 §1.6) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-S1](../pkg/inventory/scope/scope_test.go) ✅** | 상위 계층 exclude 뒤에 하위 계층 include | **하위 계층의 것이 적용된다**. pqcota의 「매치되는 마지막 규칙이 결정한다」가 그대로 상속 규칙이다. 순서를 뒤집으면 결과도 뒤집힌다 |
| [IC-S2](../pkg/inventory/scope/scope_test.go) ✅ | 지금 정책 vs 제안된 계층 합본 | **바뀐 규칙만** 올린다. exclude 추가는 근거 필수 |
| [IC-S3](../pkg/inventory/scope/scope_test.go) ✅ | 규칙이 사라짐 | 리뷰에는 올리되 근거 필수는 아니다. 넓어지는 방향은 성격이 다르다 |
| [IC-S4](../pkg/inventory/scope/scope_test.go) ✅ | note 만 고친 규칙 | 같은 규칙으로 본다. 설명을 다듬었다고 재승인을 받으면 리뷰가 잡음으로 찬다 |
| **[IC-S5](../pkg/inventory/scope/scope_test.go) ✅** | 확정된 정책을 CSV로 냄 | **pqcota `LoadAssetPolicy`가 그대로 읽는다**. 우리 형식을 만들면 「거버넌스가 확정한 정책을 pqcota가 집행한다」가 거짓이 된다 |
| [IC-S6](../pkg/inventory/scope/scope_test.go) ✅ | 정책이 뺀 자산 | **이름으로** 보여 준다(pqcota는 수만 센다). 관측을 걸러낸 것이므로 지금도 실재하는 것이다 |
| **[IC-S7](../pkg/inventory/scope/scope_test.go) ✅** | 제외분 재검토 | 승인이 없거나 만료된 것만 다시 올린다. **제외는 영구 면제가 아니다.** 살아 있는 승인은 그대로 둔다 |
| [IC-S8](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 계층 파일을 쓰고 다시 읽음 | 같은 규칙이 나오고 **계층 이름은 파일 이름에서** 온다. 어긋나면 저장할 때마다 규칙이 조금씩 달라진다 |
| [IC-S9](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 계층 파일 저장 | 쓰다 만 파일을 남기지 않는다. 잘린 CSV가 남으면 다음에 열 때 규칙이 통째로 사라진 것처럼 보인다 |
| **[IC-S10](../inventory/cmd/pqcaton-scope/main_test.go) ✅** | note 만 고치고 세션 재개 | 적어 둔 판정이 남는다(동일성은 `RuleID`, note는 넣지 않는다). 고칠 때마다 다시 적게 하면 아무도 화면에서 안 고친다 |
| **[IC-S11](../inventory/cmd/pqcaton-scope/main_test.go) ✅** | 계층에 **못 보던 변경**이 생김 | 그 계층의 일괄 결론을 **지운다.** 그대로 두면 방금 넣은 exclude 가 **누가 승인한 적 없는 근거를 달고** 확정을 통과한다 |
| [IC-S12](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 정책이 그대로인 채 세션 재개 | 서명도 그대로다. 새로고침마다 지우면 사람이 서명 칸을 계속 다시 채운다 |
| [IC-S13](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 빈 조직으로 `open` | 열리지 않는다. 저장소들과 같은 규칙이다 |
| [IC-S14](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 계층 파일 경로 | **계층 이름은 파일 이름에서 온다**. 그 이름이 곧 일괄 판정의 식별자라, 어긋나면 승인 단위가 흩어진다 |

### C. confidence 스코어링 (§3.5): 부분
| TC | Given → When | Then |
|---|---|---|
| [IC-C1](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 상태별 | CONFIRMED > UNDECLARED > UNOBSERVED |
| [IC-C2](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 관측 finding의 evidence_strength=inferred-low | confidence 상한 하향(불확실 관측은 신뢰 낮춤) |
| IC-C3 ⏳ | 관측빈도·기간·선언신선도 실측 | f(...) 캘리브레이션(설계 §11, Phase 1 데이터 필요) |

### Q. 리뷰 큐 (§3.3②) ✅ / 부분
| TC | Given → When | Then |
|---|---|---|
| [IC-Q1](../pkg/inventory/reconcile/reconcile_test.go) ✅ | CONFIRMED + 고신뢰 | 자동통과 후보(일괄 승인 제안, 승인은 사람) |
| [IC-Q2](../pkg/inventory/reconcile/reconcile_test.go) ✅ | UNDECLARED | 필수 개별 리뷰, **최우선** |
| [IC-Q3](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 저신뢰 CONFIRMED | 필수 개별 리뷰 |
| IC-Q4 🔜 | 우선순위 = 위험도 × 영향 범위 × 데이터민감도 | 실측 프록시(현재 상태 기반 P1~3은 임시) |
| **[IC-Q5](../pkg/inventory/review/build_test.go) ✅** | 관측이 갱신되어 큐를 다시 세움 | 적어 둔 판정과 계획 표시가 남는다. 그때마다 다시 적게 하면 아무도 화면을 안 쓴다 |
| **[IC-Q6](../pkg/inventory/review/build_test.go) ✅** | 정책에 **못 보던 항목**이 생김 | 그 정책의 일괄 결론과 서명을 **지운다.** 새로 관측된 UNDECLARED 는 사람이 본 적이 없다 |
| [IC-Q7](../pkg/inventory/review/build_test.go) ✅ | 사라진 항목 | 판정이 따라오지 않는다. 더는 올라온 것이 아니다 |

### F. 리뷰-확정 상태기계 (§3.3③, §6) ✅: 핵심
| TC | Given → When | Then |
|---|---|---|
| [IC-F1](../pkg/inventory/decision/session_test.go) ✅ | 신규 판정 대상 | 상태 = **draft** |
| [IC-F2](../pkg/inventory/decision/session_test.go) ✅ | draft → in-review 전이 | 허용 |
| [IC-F3](../pkg/inventory/decision/session_test.go) ✅ | in-review에서 finalize (전 필수항목 판정 + 승인 서명 有) | **finalized** |
| [IC-F4](../pkg/inventory/decision/session_test.go) ✅ | 필수 항목 미판정 상태로 finalize 시도 | **거부**(전 필수 판정 전 불가) |
| [IC-F5](../pkg/inventory/decision/session_test.go) ✅ | 승인 서명 없이 finalize | **거부** |
| [IC-F6](../pkg/inventory/decision/session_test.go) ✅ | 링/도메인 단위 부분 확정 | 허용(부분 finalize, §3.3③) |
| [IC-F7](../pkg/inventory/decision/session_test.go) ✅ | 정책 단위 판정(버전×링크모드 템플릿) | 동종 자산 일괄 적용(§3.4) / 예외만 엣지 단위 |

### D. 판정 영속화 (§3.6, §7) ✅: 파일과 Postgres
| TC | Given → When | Then |
|---|---|---|
| [IC-D1](../pkg/inventory/decision/judgment_test.go) ✅ | 판정 후 재수집(새 스냅샷) | 판정(사람의 결론)이 **그대로 붙어 있다**. 엣지 상태가 바뀌어도 결론은 남는다 |
| [IC-D2](../pkg/inventory/decision/judgment_test.go) ✅ | 근거 증거(BasisHash) 실질 변화 | **해당 판정만** 재검토 플래그(델타 리뷰), 나머지 유지 |
| [IC-D3](../pkg/inventory/decision/judgment_test.go) ✅ | 근거 불변 | 판정 유지(재리뷰 안 함) |
| [IC-D4](../pkg/inventory/decision/judgment_test.go) ✅ | stale 판정 + 만료 경과 | 신뢰도 감쇠 + 주기 재확인 플래그 |
| [IC-D5](../pkg/inventory/decision/pg_test.go) ✅ | 영속화(Postgres) 라운드트립 | Decision 보존(append-only, §0.2) |
| [IC-D6](../pkg/inventory/decision/file_test.go) ✅ | 같은 대상을 다시 판정 | **파일 원장도 쌓기만 한다**. 덮어쓰면 「언제 무엇으로 바뀌었나」가 사라진다(§0.2) |
| [IC-D7](../pkg/inventory/decision/file_test.go) ✅ | 다른 조직의 판정이 섞인 파일 | 읽지 않는다. 파일은 누구나 이어 쓸 수 있어, 거르지 않으면 격리가 파일 권한에만 기댄다 |
| [IC-D8](../pkg/inventory/decision/file_test.go) ✅ | 조직 없이 열기 · 아직 아무것도 없는 파일 | 조직 없이는 열리지 않는다(Mem·Pg와 같은 규칙). 빈 파일은 오류가 아니다 |

### P. 확정 계획 & 핸드오프 (§3.7, §5, §8) ✅
| TC | Given → When | Then |
|---|---|---|
| [IC-P1](../pkg/inventory/decision/plan_test.go) ✅ | finalized 계획 생성 | PlanItem[]: node·remediation_class·**deploy_automation_level**·provider_choice |
| [IC-P2](../pkg/inventory/decision/plan_test.go) ✅ | deploy_automation_level 판정 | 자산별로 리뷰어가 판정한다(§4.5 MANUAL). 전사 일괄이 아니다 |
| [IC-P3](../pkg/inventory/decision/plan_test.go) ✅ | 규제 대상 자산(fips_validation 요구) | **FIPS 검증 provider로 라우팅 강제**(§4.10, Java=BC-FJA) · **CNG 는 빈 값이다**. 갈아 끼울 provider 가 관측에 없고 FIPS 여부는 알 수 없다(§2.5). 이름을 지어내면 계획을 받는 쪽이 검증된 선택으로 읽는다 |
| **[IC-P4](../pkg/inventory/decision/plan_test.go) ✅** | **finalized 아닌 계획을 Deploy로** | **실행을 거부한다**(§5. 반드시 거쳐야 하는 관문). 핵심 인수 기준이다 |
| [IC-P5](../pkg/inventory/decision/plan_test.go) ✅ | finalized 계획 | 프로비저닝의 **유일** 실행 근거(§3.7) |
| **[IC-P6](../pkg/inventory/review/review_test.go) ✅** | 스코프가 URI인 노드(`host://local`)를 계약 형식으로 | **겨눈 노드와 런타임이 그대로 간다**. v0.1.0은 id를 쪼개 `host:`를 겨누고 런타임을 기본값으로 떨어뜨렸다 |
| [IC-P7](../pkg/inventory/review/review_test.go) ✅ | node가 빈 항목을 계획에 | 확정 직전에 거부하고 `open`을 다시 돌리라고 알려 준다. 이름 없는 노드에 조치를 걸지 않는다 |

### E. 통신 엣지 대조와 토폴로지 (§12) 🔶: 엔진·렌더·저장 완료(unit); 라이브 관측은 network-collector(§2.5)가 공급
| TC | Given → When | Then |
|---|---|---|
| [IC-E1](../pkg/inventory/reconcile/edge_test.go) ✅ | 관측 엣지(TLS/SSH 협상) vs 선언 엣지 | 엣지 3-상태(CONFIRMED/UNDECLARED/UNOBSERVED) + 등급 부착 |
| [IC-E2](../pkg/inventory/reconcile/topology_test.go) ✅ | 토폴로지 렌더 | 색=등급(🟢PQC/🔴취약/⚪불명), 미관측=점선(≠부재, §12.2 정직성) |
| [IC-E3](../pkg/inventory/reconcile/edge_test.go) ✅ | 스코프 밖 관측 상대 | off-scope 표기 "등재 판정 요청"(§0.4/§5) |

### X. 라이선스 관문 (`tools/checklicenses`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-X1](../tools/checklicenses/main_test.go) ✅** | 금지 목록의 라이선스를 **전부** | 하나도 통과하지 못하고 **왜 막혔는지** 알려 준다. 한 종류만 재면 나머지가 새는지 알 수 없다 |
| **[IC-X2](../tools/checklicenses/main_test.go) ✅** | 금지 목록에도 허용 목록에도 없는 라이선스 | 막는다. 모르는 것을 통과시키면 관문이 아니라 블랙리스트다 |
| [IC-X3](../tools/checklicenses/main_test.go) ✅ | `licenses.txt`에 적히지 않은 모듈 | 막는다. `go get` 한 번이면 새 의존성이 들어온다 |
| [IC-X4](../tools/checklicenses/main_test.go) ✅ | 허용적 라이선스 | 통과한다. 막는 것만 재면 전부 막아도 케이스는 통과한다 |
| [IC-X5](../tools/checklicenses/main_test.go) ✅ | 허용 목록의 주석·빈 줄·형식이 깨진 줄 | 앞 둘은 건너뛰고 깨진 줄은 **중단한다**(넘기면 그 모듈이 목록에서 사라진다) |
| [IC-X6](../tools/checklicenses/main_test.go) ✅ | `licenses.txt` 자체가 없음 | 열리되 **전부 「모름」으로 막힌다**. 파일이 없다고 관문이 열리지 않는다 |
| **[IC-X7](../tools/checklicenses/main_test.go) ✅** | 링크 모듈 목록 | 메인 모듈이 섞이지 않고 **0개가 아니다.** 0개를 통과로 보면 하위 폴더에서 돌렸을 때 아무것도 검사하지 않고 초록이 된다 |
| **[IC-X8](../tools/checklicenses/main_test.go) ✅** | 리포에 놓인 `.js`·`.css`·`.mjs` | 확장자로 훑고 `.git`·`node_modules`·`testdata` 는 뺀다. 배포물이 아니다. **디렉터리 규약을 믿지 않는 이유**는 규약을 어긴 사람이 아니라 모르고 다른 곳에 놓은 사람이 지나가기 때문이다 |
| **[IC-X9](../tools/checklicenses/main_test.go) ✅** | `licenses.txt`에 적히지 않은 웹 자산 | 모듈과 똑같이 막는다. 관문을 넓힌 이유가 여기다. 예전에는 프런트 라이브러리를 받아 넣어도 초록이었다 |
| [IC-X10](../tools/checklicenses/main_test.go) ✅ | 카피레프트 `.js` | 막는다. 프런트에는 GPL 라이브러리가 흔하다 |
| **[IC-X11](../tools/checklicenses/main_test.go) ✅** | 실제로 싣고 있는 htmx | `licenses.txt` 값이 옆의 LICENSE 원문과 맞는다. **파일만 갈아 끼우고 목록을 안 고치는 날**을 잡는다 |

### U. 리뷰 화면 (`pqcaton-ui`) ✅
| TC | Given → When | Then |
|---|---|---|
| [IC-U1](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 세션을 화면으로 | **정책으로 묶어** 보여 준다. 한 줄씩 늘어놓으면 화면이 있어도 수천 대에서 리뷰가 안 끝난다 |
| [IC-U2](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 「저장만」 | 세션 파일에 남되 **확정 계획은 생기지 않는다**. 채우다 만 것을 확정으로 오해하면 감사 기록이 거짓이 된다 |
| **[IC-U3](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 서명 없이 「확정」 | **막고 무엇이 남았는지 화면에 그대로 보인다**. `close` 와 같은 관문(`review.Finalize`)를 탄다 |
| [IC-U4](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 필수 결론·서명을 채우고 확정 | 계약 형식 계획이 파일로 나오고 판정이 조직에 묶여 원장에 남는다 |
| **[IC-U5](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 화면으로 채운 세션 파일 | **명령이 그대로 읽고 확정한다**. 기계가 채운 `node`·`runtime` 이 왕복에서 사라지지 않는다 |
| [IC-U6](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 바인드 주소 | 루프백을 가려낸다. 밖으로 열리면 경고한다 |
| [IC-U7](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `/save`·`/finalize` 에 GET | 405 를 낸다. 새로고침으로 확정이 다시 돌지 않게 POST 뒤엔 리다이렉트한다 |
| **[IC-U8](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | `pqcaton-ui session.json -addr ...` | 위치 인자 뒤의 플래그도 먹는다. 표준 flag 는 거기서 멈춰 **-addr 이 아무 표시 없이 무시된다** |
| [IC-U9](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `-decl` 없이 / 주고 | 없으면 이동 링크도 `/decl` 도 없다(404). **없는 것을 눌러 보게 하지 않는다** |
| **[IC-U10](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 선언 화면을 염 | 설명이 **접혀 있다**. 날마다 쓰는 화면이지 읽는 화면이 아니다. 필요한 사람만 「도움말」을 편다 |
| [IC-U11](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 화면에서 선언 저장 | 명령이 그대로 읽고, 다 맞추면 짚을 것이 없다 |
| [IC-U12](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 노드 이름을 비워 저장 | 그 줄이 지워진다. 표에서 줄을 없애는 유일한 방법이다 |
| **[IC-U13](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 탭 순서 | **선언 → 대조 → 판정** 순이다. 절차 순서다. 뒤섞이면 화면이 절차를 가르치지 못한다 |
| **[IC-U24](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 탭 이름 | 화면이 쓰는 말과 이어진다. 판정을 **하는** 자리는 「④ 판정(리뷰 큐)」, **보는** 자리는 「인벤토리·판정 이력」. 대조 탭은 있는데 판정 탭이 없으면 어디서 하는지 알 수 없다 |
| [IC-U14](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `/` 로 들어옴 | 절차의 첫 자리로 보낸다(선언이 있으면 선언, 없으면 리뷰 큐) |
| [IC-U15](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `-results` 없이 | 대조 탭도 `/survey` 도 없다(404) |
| **[IC-U16](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 네 화면을 모두 준 상태의 탭 순서 | **선언 → 스코프 → 대조 → 판정** 순이다. 무엇을 계속 볼지가 정해져야 관측이 적재되므로 스코프가 대조 앞이다 |
| **[IC-U17](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 근거 없이 스코프 확정 | 막고 어느 변경이 막았는지 알려 준다. `pqcaton-scope close` 와 같은 관문 |
| [IC-U18](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 승인 뒤 나온 CSV | **pqcota가 그대로 읽고** 판정이 조직에 묶여 남는다 |
| **[IC-U19](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 「행 추가」에 모르는 표 이름 · 범위 밖 번호 | 막는다(400). 그리면 화면에는 줄이 생기는데 `ApplyDecl` 이 읽지 못하는 자리에 놓인다. 사람은 적어 넣고 저장했는데 **아무 일도 일어나지 않는다** |
| [IC-U20](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `/static/htmx.min.js` · `/static/app.css` | 같은 서버가 내준다. 주소가 어긋나면 화면은 뜨는데 모양이 무너지고 「행 추가」가 아무 표시 없이 동작하지 않는다 |
| **[IC-U21](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 계층 CSV만 주고 **세션 파일 없음** | 화면이 세션을 연다. 입력 파일을 이미 가지고도 명령을 한 번 돌려야 열리는 것은 화면을 두는 이유와 어긋난다 |
| **[IC-U22](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 화면에서 고친 규칙 | **계층 CSV에 그대로 쓰이고 pqcota가 읽는다.** 새 exclude 가 판정 대상으로 올라온다. 빈 줄은 규칙이 되지 않는다 |
| **[IC-U23](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 선언과 관측 결과만 주고 **세션 파일 없음** | 리뷰 큐가 열린다 |

### T. 말의 경계 (`tools/checktext`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-T1](../tools/checktext/main_test.go) ✅** | 한국어 주석과 한국어 문자열이 함께 있는 파일 | **문자열만 잡고 주석은 건드리지 않는다**. 주석까지 막으면 이 리포가 판단 근거를 적어 두는 방식이 통째로 막힌다 |
| **[IC-T2](../tools/checktext/main_test.go) ✅** | URL 안에 `//` 가 있는 문자열 · 여러 줄 백틱 문자열 | 둘 다 잡는다. **정규식이 놓친 실제 세 자리**를 케이스로 남겨 둔 것이다. 그래서 `go/ast` 로 본다 |
| [IC-T3](../tools/checktext/main_test.go) ✅ | 봐주는 파일(카탈로그·토글·테스트·생성물)과 그 옆 파일 | 목록에 적힌 것만 넘어간다. 규약으로 두면 새 파일이 슬그머니 예외가 된다 |

### K. 문체의 경계 (`tools/checkprose`) ✅

문서와 화면 문구의 한국어를 잰다. **한 번 걷어낸 말이 다시 들어오지 않게** 막는 것이 전부다.
지금 있는 것은 `baseline.tsv` 에 파일마다 적어 두고 **늘어나는 것만** 막는다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-K1](../tools/checkprose/main_test.go) ✅** | 코드 블록 안의 엠대시 | 세지 않는다. 지침이 「인용·코드·코드 주석에는 적용하지 않는다」고 규정했다. 고칠 수 없는 것을 요구하는 관문은 아무도 켜지 않는다 |
| [IC-K2](../tools/checkprose/main_test.go) ✅ | 백틱 안의 경로·플래그 | 세지 않는다. 덮지 않으면 코드에 적힌 이름이 문체 위반으로 잡힌다 |
| **[IC-K3](../tools/checkprose/main_test.go) ✅** | 「헷갈리는」과 진짜 위반이 한 파일에 | 뒤엣것만 잡는다. RE2 에 뒤보기가 없어 그냥 재면 앞엣것이 함께 걸린다. 잘못 잡는 관문은 예외를 쌓게 하고, 예외가 쌓이면 진짜 위반이 그 속에 묻힌다 |
| **[IC-K4](../tools/checkprose/main_test.go) ✅** | 한국어 주석과 화면 문구가 함께 있는 Go 파일 | 문자열만 잡는다. **IC-T1 과 같은 선이다.** 주석까지 막으면 이 리포가 판단 근거를 적어 두는 방식이 통째로 막힌다 |
| [IC-K5](../tools/checkprose/main_test.go) ✅ | 덮은 문서 | 바이트 수가 그대로라 줄 번호가 맞는다. 보여 주는 것은 덮인 줄이 아니라 원문이다. 덮인 줄을 찍으면 어느 문장인지 알아볼 수 없다 |
| **[IC-K6](../tools/checkprose/main_test.go) ✅** | 기준선보다 늘어남 | 막고 **무엇이 얼마나 늘었는지 알려 준다** |
| **[IC-K7](../tools/checkprose/main_test.go) ✅** | 기준선보다 줄어듦 | 이것도 막고 새로 찍으라고 알려 준다. 고쳐 놓고 기준선을 안 내리면 그 자리가 도로 채워져도 알 수 없다 |
| [IC-K8](../tools/checkprose/main_test.go) ✅ | 기준선을 찍고 다시 읽음 | 같다. 찍는 쪽과 읽는 쪽이 어긋나면 관문이 매번 붉어지고, 그러면 기준선을 지우는 것으로 끝난다 |
| [IC-K9](../tools/checkprose/main_test.go) ✅ | 함께 나가는 `rules.tsv` | 읽힌다. 이름이 겹치지 않고 **무엇으로 바꿀지가 규칙마다 적혀 있다**. 막기만 하고 대안을 주지 않으면 고칠 수 없다 |
| [IC-K10](../tools/checkprose/main_test.go) ✅ | 목록에 적힌 화면 문구 파일 | 실제로 있다. 화면 문구가 네 파일에 나뉘어 있어(`text*.go`) 규약으로 두면 새 파일이 관문 밖이 된다 |
| [IC-K11](../tools/checkprose/main_test.go) ✅ | 함께 나가는 `overlap.txt` | 읽히고, 덮어도 바이트 수가 그대로다 |
| [IC-K12](../tools/checkprose/main_test.go) ✅ | 한국어가 없는 줄의 엠대시 | 세지 않는다. 지침은 한국어를 명확하게 쓰라는 것이지 외국어를 고치라는 것이 아니다(「동작 범위」 1항). 화면 카탈로그가 KO 와 EN 을 나란히 적는 자리라 이 선이 없으면 영어 문장까지 세게 된다 |
| [IC-K13](../tools/checkprose/main_test.go) ✅ | 한 줄에 `T{KO: …, EN: …}` 로 두 말이 나란히 | 한국어 문자열만 잰다. 줄 단위로만 보면 영어의 엠대시까지 세는데, 영어에서 그것은 맞는 문장부호다 |

> **규칙표(`rules.tsv`)와 잘못 잡는 말(`overlap.txt`)은 코드 밖에 있습니다.** 코드에 적으면
> `tools/checktext` 가 막고(Go 문자열의 한글), 두 목록 모두 지적받을 때마다 늘어나기
> 때문입니다. 목록을 늘리는 데 Go 를 건드리지 않아도 됩니다.

### M. 번호와 테스트의 대응 (`tools/checkcases`) ✅

이 문서 첫머리가 **「케이스 번호가 곧 테스트 파일 링크입니다」** 라고 약속한다. 그 약속을
사람이 지키게 두었더니 백일흔넷 가운데 링크가 하나도 없었고, 두 주 동안 아무도 몰랐다.
그래서 **약속을 지키는 일을 기계에 맡긴다.** 링크는 `-write` 가 찍는다.

**케이스 표가 둘이다.** 인벤토리는 이 문서에, 러너는
[`saas/runner/README.md`](../saas/runner/README.md)에 있다. 코드가 거기 있으니 케이스도 거기
있어야 한다. 관문이 둘을 함께 읽고, 문서마다 맡는 접두어(`IC` · `RUN`)만 그 문서에서 찾는다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-M1](../tools/checkcases/main_test.go) ✅** | 테스트가 `// IC-R1·R2·R3` 로 번호를 축약해 적음 | 셋으로 펴서 읽는다. 못 펴면 「✅ 인데 테스트가 없다」가 거짓으로 뜬다. 이 도구를 만들기 전에 손으로 세다가 실제로 여섯 건을 그렇게 잘못 셌다 |
| [IC-M2](../tools/checkcases/main_test.go) ✅ | 번호 칸의 두 모양(굵은 것·안 굵은 것)과 이미 링크가 붙은 것 | 셋 다 읽는다. 하나라도 못 읽으면 그 케이스만 관문 밖이 된다 |
| **[IC-M3](../tools/checkcases/main_test.go) ✅** | `-write` 로 링크를 찍음 | **굵게와 상태 표시를 그대로 둔다.** 모양이 달라지면 사람이 diff 를 못 읽고, 못 읽으면 다음부터 안 돌린다 |
| [IC-M4](../tools/checkcases/main_test.go) ✅ | 찍은 문서를 다시 찍음 | 달라지지 않는다. 링크가 겹쳐 쌓이면 문서가 망가진다 |
| [IC-M5](../tools/checkcases/main_test.go) ✅ | 한 번호를 두 파일이 잼(엣지판·본판) | 둘 다 잡고 정렬해 첫 파일로 링크한다. 어긋남으로 세지 않는다 |
| **[IC-M6](../tools/checkcases/main_test.go) ✅** | **함께 나가는 문서와 테스트** | 실제로 맞는다. 위 다섯은 만들어 낸 입력으로 재는 것이라, 진짜 리포를 재는 이 케이스가 이 도구의 존재 이유다 |
| **[IC-M7](../tools/checkcases/main_test.go) ✅** | 픽스처로 케이스 표 한 줄을 문자열에 담은 테스트 파일 | **주석의 번호만 잡는다.** 파일 전체를 정규식으로 훑었더니 그 문자열 때문에 미구현 케이스 하나가 이 도구의 테스트 파일로 링크됐다. `IC-T2` 와 같은 답을 쓴다 |
| **[IC-M8](../tools/checkcases/main_test.go) ✅** | 케이스 표가 자기가 안 맡는 접두어의 행을 가짐 | **읽지 않는다.** 러너 테스트는 이 리포에, 컨트롤 플레인 테스트는 비공개 리포에 있다. 문서마다 맡는 접두어를 적어 두지 않으면 남의 리포에 있는 테스트를 「없다」고 막는다 |
| [IC-M9](../tools/checkcases/main_test.go) ✅ | `docs/` 의 표와 테스트 바로 옆의 표 | 링크가 **그 문서에서 본 상대 경로**다. 한 가지로 적으면 한쪽이 깨진다 |

> **막는 것은 셋이다.** ✅ 인데 테스트에 번호가 없는 것, 테스트에 있는데 문서에 없는 것,
> 그리고 ⏳·🔜 인데 테스트가 있는 것(표시가 낡았다는 뜻이다).

### V. 선언 형식과 자체 검사 (`pkg/inventory/decl`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-D9](../pkg/inventory/decl/decl_test.go) ✅** | IP 없는 노드 / 스코프에만 있고 IP 표에 없는 노드 | **둘 다 짚고 결과를 같게 알려 준다**. IP가 노드로 이어지지 않으면 선언 엣지는 미관측으로, 관측 엣지는 UNDECLARED 로 구분된다 |
| [IC-D10](../pkg/inventory/decl/decl_test.go) ✅ | 같은 IP를 노드 둘이 주장 | 짚는다. IP가 뒤에 오는 노드로 이어져 통신이 엉뚱한 노드에 붙는다 |
| **[IC-D17](../pkg/inventory/decl/decl_test.go) ✅** | 같은 관측 이름을 노드 둘이 주장 | 짚는다. IP를 겹쳐 주장하는 것과 같은 사태다. 한쪽만 이기므로 그 기계의 자산이 남의 노드에 붙고, 진 노드의 선언은 미관측으로 남는다. 대소문자는 가리지 않는다 |
| **[IC-D18](../pkg/inventory/decl/decl_test.go) ✅** | 다른 노드의 이름을 관측 이름으로 적음 | 짚는다. 어느 쪽에 붙을지를 적은 순서가 정하게 된다. 겹치지 않으면 짚지 않는다 |
| [IC-D11](../pkg/inventory/decl/decl_test.go) ✅ | IP 자리에 `10.0.0.1:8443` · `db.internal` | 둘 다 짚는다. 잇기는 문자열이 정확히 맞을 때만 된다 |
| [IC-D12](../pkg/inventory/decl/decl_test.go) ✅ | 스코프 밖을 가리키는 자산·엣지 | 짚는다. 늘 미관측으로 남고 그것이 「없다」로 읽힌다 |
| [IC-D13](../pkg/inventory/decl/decl_test.go) ✅ | 포트 0인 엣지 | 짚는다. 엣지 동일성에 포트가 들어간다 |
| [IC-D14](../pkg/inventory/decl/decl_test.go) ✅ | 앞뒤가 맞는 선언 | **조용하다**. 막는 것만 재면 전부 짚어도 통과한다 |
| [IC-D15](../pkg/inventory/decl/decl_test.go) ✅ | 파일 왕복 | 화면이 쓴 것을 명령이 그대로 읽는다 |
| [IC-D16](../pkg/inventory/decl/decl_test.go) ✅ | 조직을 안 적음 | `local` |

### UI. 화면 공용 패키지 (`pkg/inventory/ui`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-UI1](../pkg/inventory/ui/ui_test.go) ✅** | 선언을 화면으로 | **있는 것을 다 보여 준다**. 표에 안 나온 줄은 폼으로 안 돌아와 저장하는 순간 사라진다 |
| **[IC-UI2](../pkg/inventory/ui/ui_test.go) ✅** | 그린 폼을 다시 읽음 | 같은 선언이 나온다. 그리기와 읽기가 어긋나면 저장할 때마다 오류 없이 달라진다 |
| [IC-UI3](../pkg/inventory/ui/ui_test.go) ✅ | IP를 쉼표로 / 공백으로 / 붙여서 | 셋 다 받는다 |
| [IC-UI4](../pkg/inventory/ui/ui_test.go) ✅ | 폼에 없는 `_comment` | 저장에서 사라지지 않는다. 생성 도구가 남긴 머리말이다 |
| [IC-UI5](../pkg/inventory/ui/ui_test.go) ✅ | 리뷰 세션을 화면으로 | 정책으로 묶이고 **정렬된다**(실행마다 순서가 흔들리지 않게) |
| [IC-UI6](../pkg/inventory/ui/ui_test.go) ✅ | 리뷰 폼을 읽음 | 승인 정보와 판정이 세션에 얹힌다 |
| [IC-UI7](../pkg/inventory/ui/ui_test.go) ✅ | 스코프 세션을 화면으로 | 계층으로 묶이고 정렬된다. 근거 필수 건수를 센다 |
| [IC-UI8](../pkg/inventory/ui/ui_test.go) ✅ | 스코프 폼을 읽음 | 계층 판정과 개별 결론이 세션에 얹힌다 |
| **[IC-UI9](../pkg/inventory/ui/ui_test.go) ✅** | 「행 추가」가 만든 줄 | 화면이 그리는 줄과 **같은 폼 이름**을 쓴다. 폼 이름이 곧 저장 경로다. 둘이 어긋나면 화면은 멀쩡하고 새 줄만 아무 표시 없이 저장되지 않는다 |
| **[IC-UI10](../pkg/inventory/ui/ui_test.go) ✅** | 줄을 하나 내줌 | 다음 번호가 하나 오르고 버튼이 **자기 자신을 갈아 끼운다**. 같은 번호면 앞 줄을 덮고, 건너뛰면 그 뒤가 통째로 저장되지 않는다 |
| [IC-UI11](../pkg/inventory/ui/ui_test.go) ✅ | 모르는 표 이름 | 받지 않는다. 주소는 밖에서 오는 값이다 |
| [IC-UI12](../pkg/inventory/ui/ui_test.go) ✅ | `/static/` 요청 | **바이너리에서** 나온다. CDN 이면 망이 끊긴 기계에서 화면이 깨지고, 남의 서버 스크립트는 라이선스 관문이 볼 수조차 없다 |
| [IC-UI13](../pkg/inventory/ui/ui_test.go) ✅ | 그려진 화면 | 그 둘을 실제로 부른다. 적어만 두고 안 부르면 아무 일도 일어나지 않는다 |
| **[IC-UI14](../pkg/inventory/ui/ui_test.go) ✅** | 규칙 표를 그리고 다시 읽음 | 같은 규칙이 나오고 **어느 파일의 어느 계층인지**가 남는다. 잃으면 저장할 곳을 잃는다 |
| **[IC-UI15](../pkg/inventory/ui/ui_test.go) ✅** | 세 칸이 모두 빈 줄에 `action`만 exclude | 규칙으로 만들지 않는다. 그대로 두면 `exclude,*,*,*` 가 되어 **인벤토리가 통째로 빈다.** `*`를 적으면 규칙이 된다 |
| [IC-UI16](../pkg/inventory/ui/ui_test.go) ✅ | 세 칸을 비움 | 그 규칙이 지워진다. 화면에서 규칙을 뺄 유일한 방법이다 |
| [IC-UI17](../pkg/inventory/ui/ui_test.go) ✅ | 규칙 「행 추가」 | 화면과 같은 폼 이름, 다음 번호로 오른 버튼, **기본은 include** (exclude 가 기본이면 실수 한 번이 인벤토리를 지운다) |
| [IC-UI18](../pkg/inventory/ui/ui_test.go) ✅ | 계층 파일 없이 스코프 화면 | 편집 표를 그리지 않는다. **저장할 곳이 없는 칸을 사람이 채우게 하지 않는다** |
| **[IC-UI19](../pkg/inventory/ui/ui_test.go) ✅** | 화면 넷을 통째로 영어로 그림 | **한글이 한 글자도 없다.** 문구 하나를 안 옮기면 그 자리만 한국어로 뜨는데, 문구가 200개라 눈으로는 못 찾는다. 말 바꾸기 토글만 예외다 |
| [IC-UI20](../pkg/inventory/ui/ui_test.go) ✅ | 같은 화면을 두 말로 | 둘 다 그려진다. 한쪽만 되면 토글이 장식이 된다 |
| **[IC-UI21](../pkg/inventory/ui/i18n_test.go) ✅** | `?lang` · 쿠키 · `Accept-Language` 가 다 있음 | **고른 것이 브라우저에 우선한다.** 브라우저가 우선하면 토글을 눌러도 다음 화면에서 되돌아간다 |
| [IC-UI22](../pkg/inventory/ui/i18n_test.go) ✅ | 말을 바꿈 | 보던 자리와 알림이 그대로 남는다. 첫 화면으로 되돌리면 아무도 안 바꾼다 |
| [IC-UI23](../pkg/inventory/ui/i18n_test.go) ✅ | 토글 이름표 | **가려는 쪽의 말로** 적는다. 지금 말을 못 읽는 사람도 자기 말은 알아본다 |
| **[IC-UI24](../pkg/inventory/ui/ui_test.go) ✅** | 걸러 낸 결과의 「몇 개 중 몇 개」를 두 말로 | **숫자가 뒤집히지 않는다.** 한국어는 「전체 중 몇 개」, 영어는 「몇 개 of 전체」로 어순이 다르다. 자리를 번호로 고정하지 않으면 한쪽 말에서만 바뀌어 뜨고, 보는 사람은 **걸러 낸 것을 전부로 읽는다** |
| **[IC-UI26](../pkg/inventory/ui/ui_test.go) ✅** | 스코프에만 있는 노드 · 주소 표에만 있는 노드 | **한 표로 합쳐 보여 준다**. 스코프 순서가 앞이고, IP를 모르는 노드는 IP 칸이 빈 줄이다 |
| **[IC-UI29](../pkg/inventory/ui/ui_test.go) ✅** | 화면을 그림 | 설명이 **「도움말」로 접혀 있다**. 날마다 여는 자리지 읽는 자리가 아니다. 지우지 않고 접어 두어, 필요한 사람만 편다 |
| **[IC-UI27](../pkg/inventory/ui/ui_test.go) ✅** | 합친 표를 저장한다. IP를 비운 줄이 섞여 있다 | **IP를 적은 줄만 관리 대상**이 된다. 이을 근거가 없는 이름을 관리 대상에 두면 대조 결과가 오류 없이 틀린다. **뺀 것은 알림에 적는다**. 표에서 사라진 것만 보이면 지워진 것으로 읽힌다 |
| **[IC-UI39](../pkg/inventory/ui/ui_test.go) ✅** | 리뷰 큐의 CNG 항목 | **「플랫폼 조치」**가 붙는다. 계획의 provider 칸이 비는 자리라, 말하지 않으면 빠뜨린 것으로 읽힌다 |
| **[IC-UI37](../pkg/inventory/ui/ui_test.go) ✅** | 「관측 이름」을 적고 저장 | 선언에 남고 다시 그려도 그대로다. collector 가 자기 id 로 보내는 흔한 자리를 사람이 한 번 적어 두는 곳이다 |
| **[IC-UI38](../pkg/inventory/ui/ui_test.go) ✅** | 어느 노드에도 안 붙은 관측 이름 | **「관측 이름」 칸에 후보로 뜬다**. 안 붙었다는 것은 그 노드의 자산이 통째로 UNDECLARED 로 오른다는 뜻이다 |
| **[IC-UI35](../pkg/inventory/ui/ui_test.go) ✅** | 관측이 있는 노드의 컴포넌트 칸 | **관측된 이름이 후보로 뜬다**. 그것이 대조가 맞다고 보는 이름이다. 같은 것이 여러 번 관측돼도 후보는 하나다 |
| **[IC-UI36](../pkg/inventory/ui/ui_test.go) ✅** | 관측이 없는 노드 | 후보 목록을 만들지 않는다. 빈 목록을 매달면 고르는 칸처럼 보이면서 아무것도 뜨지 않는다 |
| **[IC-UI33](../pkg/inventory/ui/ui_test.go) ✅** | 자산의 런타임 칸 | **목록에서 고른다**. 관측 결과에 나올 수 있는 이름만 들어 있다. 파일에 있던 낯선 이름은 목록에 더해 고른 채로 남긴다. 화면이 아무 표시 없이 바꿔 쓰면 선언이 사람 몰래 달라진다 |
| **[IC-UI34](../pkg/inventory/ui/ui_test.go) ✅** | 자산의 컴포넌트 칸 | 맞대는 방식이 화면에 있다. **글자 그대로 같아야** 하고, `.so` 뒤는 떼고 적으며, 벤더링 해시는 떼지 않는다. 모르면 오류 없이 미관측·UNDECLARED 로 구분된다 |
| **[IC-UI31](../pkg/inventory/ui/ui_test.go) ✅** | 「제거」로 가운데 줄을 지운 폼 | **뒤의 줄이 살아 남는다**. 번호가 끊긴 자리에서 읽기를 멈추면 지운 줄 뒤가 통째로, 오류 없이 저장되지 않는다 |
| **[IC-UI32](../pkg/inventory/ui/ui_test.go) ✅** | 제거·다시 불러오기 | **묻고 지운다**(hx-confirm). 물음에 「저장해야 파일에 반영된다」까지 적혀 있다. 화면에서 지운 것과 파일에서 지운 것은 다르다 |
| **[IC-UI30](../pkg/inventory/ui/ui_test.go) ✅** | 손으로 고친 파일의 IP 없는 스코프 이름 | 표에는 올린다. 저장하면 빠지지만, 화면에서 지우면 IP를 채워 넣을 자리조차 없다 |
| **[IC-UI25](../pkg/inventory/ui/ui_test.go) ✅** | 노드 · 런타임 · 컴포넌트 · 상태로 찾기 | 자유 문자열이 **아무 칸에나** 걸리고, 상태 조건과 함께 주면 둘 다 걸린다. 어느 칸인지 미리 고르게 하면 **무엇을 찾는지 모를 때 여는 화면**에서 그 칸을 알아야 한다 |

### R2. 여러 노드 대조 (`pkg/inventory/report`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-R8](../pkg/inventory/decl/decl_test.go) ✅** | 관측 IP → 스코프 노드 잇기(포트 붙은 주소 · 망 둘에 걸친 노드 · 스코프 밖 · 이미 이어진 것) | 맞는 것만 잇고 나머지는 그대로 둔다. **잘못 이으면 CONFIRMED 여야 할 엣지가 UNDECLARED 로 올라온다**(그럴듯한 오답) |
| [IC-R9](../pkg/inventory/report/report_test.go) ✅ | NETWORK 계층이 커버 vs 강등 | 강등을 커버로 세지 않는다. 세면 **못 본 노드가 「봤다」가 되어** 토폴로지 점선이 실선이 된다 |
| [IC-R10](../pkg/inventory/report/report_test.go) ✅ | 한 노드를 collector 둘이 봄 | 중복은 지우되 **처음 순서를 지키고 입력을 덮지 않는다** |
| [IC-R11](../pkg/inventory/report/report_test.go) ✅ | 깨진 결과 파일 | 나머지는 읽되 **건너뛴 것을 이름으로 알려 준다**. 모르면 「관측 안 됨」과 「못 읽음」이 뒤섞인다 |
| [IC-R12](../pkg/inventory/report/report_test.go) ✅ | 관측 결과가 하나도 없음 | 선언만으로 대조가 돌고 **전부 미관측**이 된다. 그것이 「없다」가 아니라 「아직 못 봤다」다 |
| **[IC-R16](../pkg/inventory/reconcile/reconcile_test.go) ✅** | CNG 관측(상류 v0.6.0) | **자산이 된다**. 런타임 `cng` · 컴포넌트 `cng-providers`. 갈래를 안 더하면 Windows 노드의 암호 자산이 인벤토리에서 통째로 사라진다. **모르는 런타임은 그대로 버린다**. 이름을 지어내면 선언과 영영 맞지 않는 자산이 생긴다 |
| **[IC-R15](../pkg/inventory/report/report_test.go) ✅** | 관측 이름이 겹치거나 이름과 부딪힘 | **이름이 이기고, 겹친 관측 이름은 먼저 적힌 쪽이 가진다**. 뒤에 적힌 것으로 뒤집히면 파일 순서만 바뀌어도 자산이 다른 노드에 붙는다 |
| **[IC-R14](../pkg/inventory/report/report_test.go) ✅** | 관측 노드 id 가 선언 이름과 다름 | **선언 노드로 잇는다**. 호스트명(짧은 이름 포함)이 같으면 알아서, 아니면 적어 둔 「관측 이름」으로. 대소문자는 가리지 않는다. **어디에도 안 걸리면 관측이 부른 이름을 그대로 둔다**. 억지로 고르면 남의 노드 자산이 붙는다 |
| **[IC-R13](../pkg/inventory/report/report_test.go) ✅** | 못 본 계층을 화면·콘솔에 냄 | 상류 enum 상수(`COLLECTION_LAYER_ARTIFACT`)를 그대로 내지 않고 **관측이 어디서 오는지**를 적되 원래 이름을 괄호에 남긴다. **모르는 값은 그대로 낸다**. 상류에 계층이 늘었을 때 뭉개면 못 본 것이 화면에서 사라진다 |

> **구현 위치**: 엣지 대조 `reconcile/edge.go`(없음) · 등급 분류 `pkg/kernel/posture/` · 토폴로지 DOT `reconcile/topology.go`(없음) · 저장 `pkg/discovery/history`(Snapshot.Edges, Postgres `edges` JSONB). 관측 엣지 스키마 `contracts` `ObservedEdge`(CollectionResult.observed_edges). 이 계약을 채우는 **network-collector(디스커버리 §2.5, AF_PACKET)가 라이브 관측을 공급합니다**(대조 엔진은 합성 데이터로도 검증됩니다).

---

## 2. 구현 순서 (pure 먼저)

| # | 대상 | TC | 상태 |
|---|---|---|---|
| 1 | 3-상태 대조 + 리뷰 큐 + confidence | R1~3, Q1~3, C1 | ✅ |
| 2 | **리뷰-확정 상태기계** | F1~7 | ✅ pure |
| 3 | **확정 계획 + 핸드오프 관문**(finalized-only) | P1~5 | ✅ pure |
| 4 | UNOBSERVED×완전성 갭 연동 + evidence_strength confidence | R4, C2 | ✅ pure |
| 5 | **판정 영속화**(Decision 저장소, Postgres) + 델타 리뷰 | D1~5 | ✅ integration |
| 6 | 통신 엣지 reconciliation + 토폴로지 | E1~3 | 🔶 unit ✅ / 라이브 관측은 pqcota의 network collector |
| 7 | **대조의 조직 축**(엔진이 조직을 들고 섞인 입력을 끊음) | O1~5 | ✅ pure |
| 8 | **자산 스코프 거버넌스**(계층 상속·변경 승인·감사·제외분 재검토) | S1~7 | ✅ pure |
| 9 | **행 수준 보안**(핸들 격리가 뚫려도 DB가 막음) | L1~4 | ✅ integration |
| 10 | **명령 계층**(관문·잇기·집계가 명령에서 실제로 실행되는가) | S8~14, P6~7 | ✅ |
| 11 | **라이선스 관문**(카피레프트가 링크되면 빌드가 멈추는가) | X1~7 | ✅ |
| 12 | **리뷰 화면**(명령과 같은 파일·같은 관문) | U1~8 | ✅ |
| 13 | **화면 공용 패키지 + 선언 편집**(두 배포 형태가 같은 화면) | UI1~6, D9~16, U9~12 | ✅ |
| 14 | **여러 노드 대조 + 대조 화면**(계산은 하나, 글과 표가 같은 답) | R8~12, U13~15 | ✅ |
| 15 | **자산 스코프 화면**(명령과 같은 파일·같은 관문) | U16~18, UI7~8 | ✅ |
| 16 | **로컬 스캔의 정직성**(못 본 것을 「없다」로 내지 않음) | L5~8 | ✅ |
| 17 | **인벤토리 조회 화면**(찾아보는 자리이지 절차의 한 단계가 아니다) | UI24~25 | ✅ |
| 18 | **선언 화면을 적는 자리로**(노드마다 자산 · 제거 · 후보 · 접은 설명) | U10, U24, UI9, UI27, UI29~36 | ✅ |
| 19 | **관측과 선언의 이름을 잇는다 + 상류 v0.6.3**(CNG 자산 · 플랫폼 조치) | R14~16, D17~18, UI37~39, P3 | ✅ |
| 20 | **문체 관문**(한 번 걷어낸 말이 다시 들어오는가) | K1~11 | ✅ |
| 21 | **케이스 관문**(번호와 테스트가 실제로 대응하는가) | M1~9 | ✅ |

**핵심 인수 기준**: **IC-P4**(finalized 아니면 Deploy 거부. 반드시 거쳐야 하는 관문)와 **IC-F3~F5**(승인 서명·전 필수 판정 없으면 확정 불가).

## 3. 데이터 모델 매핑 (구현 위치)

| TC 그룹 | 구현 위치 |
|---|---|
| 대조·큐·confidence | `pkg/inventory/reconcile/` |
| 상태기계·확정 계획·관문 | `pkg/inventory/decision/` |
| 판정 영속화 | `pkg/inventory/decision/` + pqcota `pkg/discovery/history`의 PgStore 패턴 |
| 엣지·토폴로지 | `pkg/inventory/reconcile/{edge,topology}.go` |
| Decision·FinalizedPlan 스키마 | pqcota `contracts/` (공개 스키마) |
