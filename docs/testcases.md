# 테스트케이스: 인벤토리 거버넌스

[개요](../README.md) · [릴리스 노트](../RELEASE_NOTES.md) · [여정](journey.md) · [설계](design.md) · **검증 기준** · [데모](../demo/README.md) · [구조 그림](https://www.sntsoft.co.kr/pqcaton/)

케이스 번호가 곧 테스트 파일 링크입니다. **그 대응은 `make check-cases`가 잽니다**
(§M). 설계는 [design.md](design.md)에 있습니다.

**문서 성격**: [설계](design.md) §1(인벤토리 엔진)을 **검증할 수 있는 인수 기준**으로 풀어 적습니다.
구현은 이 테스트를 통과하는 것을 목표로 합니다. 대상은 `pkg/inventory/*`와
`inventory/cmd/*`입니다.
**검증 대상**: 3-상태 대조·신뢰도·리뷰 큐·리뷰-확정 상태기계·판정 영속화·확정 계획·핸드오프 관문·엣지 대조.
**실행 방침**: 대조·상태기계·관문은 순수 로직이라 실물이 필요 없고, 판정 영속화와 행 수준 보안은 Postgres 통합.

> 적재·이력·보존·자산 스코프 등 **pqcota가 구현하는 부분**의 인수 기준은
> [pqcota 인벤토리 테스트케이스](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/inventory/testcases.md)에 있습니다.

---

## 1. 테스트케이스

### R. 3-상태 대조 (§3.3①) ✅
| TC | Given → When | Then |
|---|---|---|
| [IC-R1](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 자산이 선언 ∩ 관측 | CONFIRMED |
| [IC-R2](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 관측 only(선언 안 됨) | **UNDECLARED**, NeedsReview |
| [IC-R3](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 선언 only(관측 안 됨) | UNOBSERVED, NeedsReview(기계 확정 불가) |
| [IC-R4](../pkg/inventory/reconcile/edge_test.go) ✅ | UNOBSERVED + 완전성 맵에 해당 계층 **갭** | "재수집 후보"로 표시(갭이면 미관측일 뿐). 갭 아니면 실존/stale 사람 판정(§3.3) |
| [IC-R5](../pkg/inventory/reconcile/fingerprint_test.go) ✅ | 같은 관측을 다시 읽기 · 스냅샷 id와 규칙 판만 다른 관측 · 관측 없음 | 지문이 같다. **재수집만으로는 흔들리지 않는다.** 흔들리면 안 바뀐 관측이 매번 델타 큐에 올라오고, 그런 큐는 읽히지 않는다. 관측이 없으면 지문도 없다 |
| **[IC-R20](../pkg/inventory/reconcile/managed_test.go) ✅** | 관리 근거만 · 제외 근거만 · 둘 다 없음 | **관리 축은 근거의 구성에서 파생한다.** 관리 근거 ≥ 1이면 `MANAGED`. 관측은 있으나 전부 정책이 뺐으면 대조 축은 그대로(보았다)이고 관리 축이 `EXCLUDED_BY_POLICY`이며, 대표 근거는 비고 신뢰도는 미평가이고 사람 판정을 요구하지 않는다. 관측이 없으면 `UNOBSERVED` + `NOT_EVALUATED`이고 신뢰도는 상태 기본값이되 미평가다. 전에는 제외된 finding이 정규화 안에서 사라져 선언 자산이 「보지 못했다」로 읽혔다 |
| **[IC-R21](../pkg/inventory/reconcile/managed_test.go) ✅** | 같은 자산을 원천 노드 둘이 봤고 한쪽만 정책에 걸림 | **`MANAGED`다.** 관리할 근거가 있는데 관리하지 않으면 실재하는 관리 대상을 놓친다. 대표값·신뢰도는 **관리 근거에서만**, 제외 근거는 따로 보존해 「관리 n · 제외 m」으로 알린다. 제외 근거의 순서는 입력이 아니라 원천 노드·finding 순이다 |
| **[IC-R22](../pkg/inventory/reconcile/managed_test.go) ✅** | 정책 없이 정규화한 스냅샷 + 정책 | **정책 판정은 상류 `scope.AssetPolicy.Managed`가 한다.** 거짓인 finding만 제외 근거가 되고, 앱 식별자는 **전부** 실리며(공유 `.so` 하나를 여러 앱이 로드하면 그 목록 전부가 함께 빠진 것이다), 조직이 찍힌다. 정책이 nil이면 아무것도 빠지지 않는다 |
| **[IC-R23](../pkg/inventory/report/managed_test.go) ✅** | 선언은 `openssl/libcrypto`를 적었고 정책은 그것을 관측한 앱(`sshd`)을 뺌 · 데모의 모양 | `UNOBSERVED`가 **아니라** `CONFIRMED` + `EXCLUDED_BY_POLICY`다. 리포트가 선언·정책의 어긋남을 **자산 식별자·원천 노드·앱 식별자 전부·같은 정책 코드가 뺐다는 사실**로 경고한다. 어느 규칙인지는 말하지 않는다 - 상류 `Managed`는 bool만 돌려준다. 리뷰 큐에는 오르지 않는다. 정책을 걸지 않으면 관리 대상이고 어긋남도 없다 |
| **[IC-R24](../pkg/inventory/report/managed_test.go) ✅** | 정책을 걸고 대조 | 관리 근거는 정책을 건 스냅샷에서만 나오고 지문이 있다. 제외 근거에는 **지문이 없다** - 적재되지 않은 스냅샷의 지문은 아무것도 가리키지 않는다 |
| **[IC-R25](../pkg/inventory/report/managed_test.go) ✅** | 관리 자산 하나 · 정책이 뺀 자산 하나 · 미관측 자산 하나 | **관측 자산 수는 정책이 뺀 것도 센다**(2). 관리 수는 따로 든다(1). 머리의 관측 수와 런타임별 합계가 같다 | 관리 근거의 수로 세면 머리에서 「관측 2」라 하고 바로 아래 합계는 5라고 하는 리포트가 나온다. 제외를 부재로 세는 것이고, 이 판이 닫으려는 바로 그 결함이다 |
| **[IC-R26](../pkg/inventory/reconcile/path_test.go) ✅** | 한 노드에 같은 이름의 OpenSSL 셋(경로 둘 · 경로 모름 하나) | 한 자산 키 아래 근거 셋이 되고 **근거마다 설치 경로를 든다.** 모르는 경로는 빈 값이다 | 자산 키에는 경로가 없다. 근거에서 경로를 잃으면 사람이 두 설치를 가르지 못한다 |
| **[IC-R27](../pkg/inventory/report/jca_test.go) ✅** | JCA 결과가 있고 PQC 서비스 목록이 없는 노드 · 목록이 있는(비어 있어도) 노드 · JCA 결과가 없는 노드 | **첫째만** 「v0.10.6 JCA 수집 결과임을 확인할 수 없음」으로 알린다 | JCA를 관측하지 않은 것과 옛 수집기일 수 있는 것을 같은 경고로 섞으면 어느 쪽인지 알 수 없다. 판은 단정하지 않는다 |
| **[IC-R28](../pkg/inventory/review/scope_policy_test.go) ✅** | 세션의 정책 지문과 확정할 때의 정책 지문: 지문 없음 · 다름(정책으로 열고 없이 닫음 · 반대) · 같음 · 둘 다 정책 없음 | 같을 때만 통과한다. 지문이 없으면 `ErrScopePolicyMissing`이고 **빈 정책으로 읽지 않는다.** 다르면 `ErrScopePolicy`다. 정책 없음도 지문이 있다 |
| **[IC-R6](../pkg/inventory/reconcile/fingerprint_test.go) ✅** | **`finding_id`는 같은데 버전·검출 방법·증거 강도·강화 판정·알고리즘·로드한 앱이 달라진 관측** | **지문이 달라진다.** 상류의 id는 `sha256(노드\|이름\|런타임\|fork)`라 자산이 같으면 같아서, id만 보면 이 변화가 통째로 「그대로」로 읽힌다. 대조가 그 지문을 세션까지 들고 간다 |

### O. 대조의 조직 축 (설계 §1.1) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-O1](../pkg/inventory/reconcile/reconcile_test.go) ✅** | 다른 조직의 자산이 선언·관측 어느 레인에든 섞임 | **대조하지 않고 중단한다**(`ErrOrgMismatch`). 그냥 두면 오류가 아니라 그럴듯한 결과가 나온다 |
| [IC-O2](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 식별자에 조직이 빔 / 빈 조직으로 엔진 열기 | 둘 다 거부한다. 빈 것은 「아무 조직」이 아니라 「모른다」다 |
| [IC-O3](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 스냅샷에서 관측 자산을 뽑음 | 엔진이 조직을 붙인다. 붙이는 자리가 하나여야 조직 없는 식별자가 안 생긴다 |
| [IC-O4](../pkg/inventory/reconcile/edge_test.go) ✅ | 다른 조직의 선언 엣지 | 자산과 같은 규칙으로 중단한다 |
| [IC-O5](../pkg/inventory/reconcile/edge_test.go) ✅ | 관측 엣지(조직 없음) | 엔진이 붙인다. 안 붙이면 선언과 영영 안 맞아 전부 UNDECLARED로 올라온다 |

### L. 행 수준 보안 (설계 §2.2) ✅: Postgres
| TC | Given → When | Then |
|---|---|---|
| **[IC-L1](../pkg/inventory/decision/rls_test.go) ✅** | 앱 롤로 붙어 **org 조건 없는 날것의 질의** (핸들 격리가 우회된 상황) | 남의 조직 행이 **0건**이다. DB가 막는다. 이 케이스가 통과하지 않으면 더한 한 겹은 없는 것이다 |
| [IC-L2](../pkg/inventory/decision/rls_test.go) ✅ | 남의 조직 이름으로 INSERT | 거부한다(`WITH CHECK`). 읽기만 막으면 안 보이는 행이 들어와 쌓인다 |
| [IC-L3](../pkg/inventory/decision/rls_test.go) ✅ | 자기 조직 판정 조회 | 그대로 보인다. 막는 것만 재면 전부 막아도 통과한다 |
| **[IC-L4](../pkg/inventory/decision/rls_test.go) ✅** | 슈퍼유저로 붙고 `PQCATON_REQUIRE_RLS=1` | **저장소가 열리지 않는다**. 걸어 놓고 안 무는 것이 가장 위험한 거짓 안심이다 |

> **실행**: `PQCOTA_TEST_DSN`이 있을 때만 실행됩니다. 케이스가 슈퍼유저 DSN으로 앱 롤(`NOSUPERUSER NOBYPASSRLS`)을
> 만들어 그 롤로 다시 붙습니다. **슈퍼유저로는 무엇을 걸어도 통과하므로 그 조건에서는 아무것도 재지 못합니다.**

### LS. 로컬 스캔의 정직성 (`pkg/inventory/localscan`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-L5](../pkg/inventory/localscan/localscan_test.go) ✅** | `/proc`을 열 수 없음(비-리눅스) | **중단하고 대안을 알려 준다**. 그 상태로 대조하면 선언 자산이 전부 UNOBSERVED로 나오고 리포트에 「관측이 완전합니다(`not seen: nothing`)」라고까지 적힌다 |
| [IC-L6](../pkg/inventory/localscan/localscan_test.go) ✅ | `/proc`은 열렸으나 접근 가능 0(권한) | 결과는 보여 주되 **「없는 것이 아니라 못 본 것」**이라고 알려 준다 |
| [IC-L7](../pkg/inventory/localscan/localscan_test.go) ✅ | 정상 스캔 | 조용하다. 막는 것만 재면 전부 막아도 통과한다 |
| **[IC-L8](../pkg/inventory/localscan/localscan_test.go) ✅** | 기본이 아닌 노드 이름을 줌 | **경고한다**. 이름표일 뿐 관측 대상이 아니다. 이름이 맞으면 대조까지 되어 **다른 기계의 관측으로 CONFIRMED가 나온다** |

### S. 자산 스코프 거버넌스 (설계 §1.6) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-S1](../pkg/inventory/scope/scope_test.go) ✅** | 상위 계층 exclude 뒤에 하위 계층 include | **하위 계층의 것이 적용된다**. pqcota의 「매치되는 마지막 규칙이 결정한다」가 그대로 상속 규칙이다. 순서를 뒤집으면 결과도 뒤집힌다 |
| [IC-S2](../pkg/inventory/scope/scope_test.go) ✅ | 지금 정책 vs 제안된 계층 합본 | **바뀐 규칙만** 리뷰에 올린다. exclude 추가는 근거 필수 |
| [IC-S3](../pkg/inventory/scope/scope_test.go) ✅ | 규칙이 사라짐 | 리뷰에는 올리되 근거 필수는 아니다. 넓어지는 방향은 성격이 다르다 |
| [IC-S4](../pkg/inventory/scope/scope_test.go) ✅ | note만 고친 규칙 | 같은 규칙으로 본다. 설명을 다듬었다고 재승인을 받으면 리뷰가 잡음으로 찬다 |
| **[IC-S5](../pkg/inventory/scope/scope_test.go) ✅** | 확정된 정책을 CSV로 냄 | **pqcota `LoadAssetPolicy`가 그대로 읽는다**. 우리 형식을 만들면 「거버넌스가 확정한 정책을 pqcota가 집행한다」가 거짓이 된다 |
| [IC-S6](../pkg/inventory/scope/scope_test.go) ✅ | 정책이 뺀 자산 | **이름으로** 보여 준다(pqcota는 수만 센다). 관측을 걸러낸 것이므로 지금도 실재하는 것이다 |
| **[IC-S7](../pkg/inventory/scope/scope_test.go) ✅** | 제외분 재검토 | 승인이 없거나 만료된 것만 다시 리뷰에 올린다. **제외는 영구 면제가 아니다.** 살아 있는 승인은 그대로 둔다 |
| [IC-S8](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 계층 파일을 쓰고 다시 읽음 | 같은 규칙이 나오고 **계층 이름은 파일 이름에서** 온다. 어긋나면 저장할 때마다 규칙이 조금씩 달라진다 |
| [IC-S9](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 계층 파일 저장 | 쓰다 만 파일을 남기지 않는다. 잘린 CSV가 남으면 다음에 열 때 규칙이 통째로 사라진 것처럼 보인다 |
| **[IC-S10](../inventory/cmd/pqcaton-scope/main_test.go) ✅** | note만 고치고 세션 재개 | 적어 둔 판정이 남는다(동일성은 `RuleID`, note는 넣지 않는다). 고칠 때마다 다시 적게 하면 아무도 화면에서 안 고친다 |
| **[IC-S11](../inventory/cmd/pqcaton-scope/main_test.go) ✅** | 계층에 **못 보던 변경**이 생김 | 그 계층의 일괄 결론을 **지운다.** 그대로 두면 방금 넣은 exclude가 **누가 승인한 적 없는 근거를 달고** 확정을 통과한다 |
| [IC-S12](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 정책이 그대로인 채 세션 재개 | 서명도 그대로다. 새로고침마다 지우면 사람이 서명 칸을 계속 다시 채운다 |
| [IC-S13](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 빈 조직으로 `open` | 열리지 않는다. 저장소들과 같은 규칙이다 |
| [IC-S14](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | 계층 파일 경로 | **계층 이름은 파일 이름에서 온다**. 그 이름이 곧 일괄 판정의 식별자라, 어긋나면 승인 단위가 흩어진다 |
| **[IC-S15](../pkg/inventory/scope/scope_test.go) ✅** | 원장에 그 대상의 **계획 선택 행만** 있는 제외 | 여전히 「승인이 아예 없다」로 올라온다. **계획 선택 행은 판정이 아니다.** 종류를 보지 않고 대상으로 덮으면 결론이 빈 행이 「판정이 있다」로 읽혀, 「제외 ≠ 부재」를 시간 축에서 지키는 이 경고가 사라진다. 사라진 경고는 화면에 보이지 않는다 |
| [IC-S16](../pkg/inventory/scope/digest_test.go) ✅ | 정책 지문: note · 주석 줄 · 빈 줄 · 머리글 · 칸 앞 공백만 다른 두 정책 | 지문이 같다. 판정에 쓰이지 않는 것이 지문을 바꾸면 설명을 다듬을 때마다 「정책이 바뀌었다」가 뜬다 |
| **[IC-S17](../pkg/inventory/scope/digest_test.go) ✅** | 같은 규칙을 순서만 바꾼 두 정책(exclude와 include) | 지문이 다르다. 상류가 고르는 자산도 실제로 다르다(매치되는 마지막 규칙이 결정한다) |
| [IC-S18](../pkg/inventory/scope/digest_test.go) ✅ | 빈 칸과 `*` · 패턴의 대소문자 | 빈 칸과 `*`는 지문이 같다. 대소문자가 다르면 지문이 다르다(상류 glob이 가린다) |
| **[IC-S19](../pkg/inventory/scope/digest_test.go) ✅** | 정책 파일 없음 · 빈 경로 · 규칙 0개 파일 · 읽을 수 없는 파일 · 형식이 틀린 파일 | 앞의 셋은 지문이 같다. 뒤의 둘은 **빈 정책이 되지 않고 오류다.** 빈 정책으로 읽으면 깨진 정책이 「제한 없음」으로 비교를 통과한다 |
| **[IC-S20](../pkg/inventory/report/provenance_test.go) ✅** | 같은 결과 폴더를 정책을 건 상류 적재와 이 리포의 대조에 넣음 | 노드의 v1 지문이 바이트까지 같다. 정책이 실제로 무언가를 뺐는지(정책 없는 지문과 다른지)도 함께 본다. IC-R18은 정책 없이만 잰다 |
| **[IC-S21](../pkg/inventory/upstream/upstream_test.go) ✅** | 정책 B로 적재한 상류 이력에서 B로 대조한 근거 · 정책 없이 대조한 근거를 찾음 · 그 뒤 정책 없이 다시 적재 · 근거 없음 · 정책 C(B에 제외 하나를 더해 한 노드의 관리 자산만 0개)로 다시 적재하지 않고 확인 · C로 다시 적재한 뒤 확인 | B의 근거는 모두 찾히고 다른 정책의 근거는 빠진 것으로 나온다. 다시 적재한 뒤에는 B의 스냅샷이 남아 있어도 **노드의 최신이 아니므로** 빠진 것으로 나온다. 근거가 없으면 증명된 것이 아니다. **확인 대상은 결과 폴더의 모든 원천 노드다.** 관리 자산이 0개가 된 노드도 들어가므로, 다시 적재하지 않았으면 그 노드의 빈 스냅샷이 최신이 아니어서 반드시 거절한다. C로 다시 적재하면 상류가 빈 스냅샷도 적재하므로 통과한다 |
| **[IC-S22](../pkg/inventory/upstream/reconcile_test.go) ✅** | 정책 B로 적재한 pqcota 이력을 B로 대조 · 정책 없이만 적재된 이력을 B로 대조 · B로 적재한 뒤 정책 없이 다시 적재하고 B로 대조 · 근거 없는 결과 폴더 | **화면과 명령이 함께 쓰는 판정(`upstream.Reconcile`)이다.** 차례로 통과(확인 시각 · 조직 · 노드별 지문의 기록) · `ErrMissing` · `ErrMissing`과 `ErrRevert`(있지만 최신이 아님, 이전 정책 복귀가 이렇게 보인다) · `ErrNothingToVerify`다 |
| **[IC-S23](../pkg/inventory/upstream/reconcile_test.go) ✅** | 조회 하나하나는 제한보다 짧지만 합이 넘는 이력 · 제한 안에 끝나는 이력 | **대조 전체가 제한 시간 하나에 묶인다.** 앞은 제한 시간에 바로 `ErrUnreachable`(timed out)로 돌아오고, 뒤는 통과한다. 노드마다 시간을 새로 주지 않는다 |
| **[IC-S24](../pkg/inventory/upstream/reconcile_test.go) ✅** | 호스트 · 사용자 · 비밀번호를 실은 오류로 실패하는 이력에 Reconcile · Probe, 읽히는 이력에 Probe, 빈 DSN · 닿지 않는 DSN으로 OpenPg | **조회 오류는 연결 정보 없이 분류만 남긴다.** 실패는 모두 `ErrUnreachable`이고 그 값들이 없다. 읽히는 이력은 통과한다. 빈 DSN은 `ErrNoDSN`이다 |

### C. confidence 스코어링 (§3.5): 부분
| TC | Given → When | Then |
|---|---|---|
| [IC-C1](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 상태별 | CONFIRMED > UNDECLARED > UNOBSERVED |
| [IC-C2](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 관측 finding의 evidence_strength=inferred-low | confidence 상한 하향(불확실 관측은 신뢰 낮춤) |
| IC-C3 ⏳ | 관측빈도·기간·선언신선도 실측 | f(...) 캘리브레이션(설계 §1.2, Phase 1 데이터 필요) |

### Q. 리뷰 큐 (§3.3②) ✅ / 부분
| TC | Given → When | Then |
|---|---|---|
| [IC-Q1](../pkg/inventory/reconcile/reconcile_test.go) ✅ | CONFIRMED + 고신뢰 | 자동통과 후보(일괄 승인 제안, 승인은 사람) |
| [IC-Q2](../pkg/inventory/reconcile/reconcile_test.go) ✅ | UNDECLARED | 필수 개별 리뷰, **최우선** |
| [IC-Q3](../pkg/inventory/reconcile/reconcile_test.go) ✅ | 저신뢰 CONFIRMED | 필수 개별 리뷰 |
| IC-Q4 🔜 | 우선순위 = 위험도 × 영향 범위 × 데이터민감도 | 실측 프록시(현재 상태 기반 P1~3은 임시) |
| **[IC-Q5](../pkg/inventory/review/build_test.go) ✅** | 관측이 갱신되어 큐를 다시 세움 | 적어 둔 판정과 계획 표시가 남는다. 그때마다 다시 적게 하면 아무도 화면을 안 쓴다 |
| **[IC-Q6](../pkg/inventory/review/build_test.go) ✅** | 정책에 **못 보던 항목**이 생김 | 그 정책의 일괄 결론과 서명을 **지운다.** 새로 관측된 UNDECLARED는 사람이 본 적이 없다 |
| [IC-Q7](../pkg/inventory/review/build_test.go) ✅ | 사라진 항목 | 판정이 따라오지 않는다. 더는 올라온 것이 아니다 |
| **[IC-Q8](../pkg/inventory/reconcile/managed_test.go) ✅** | 제외 전용 · 혼합 · `NOT_EVALUATED` + `UNOBSERVED` | **큐는 관리 축을 신뢰도보다 먼저 본다.** 제외 전용은 신뢰도가 미평가라 `≥ 0.8` 비교에 넣으면 0으로 읽혀 필수 리뷰에 올라간다 - 관리하지 않기로 한 자산을 판정하라고 올리는 것이다. 큐에도 자동통과에도 넣지 않는다. 혼합은 관리 근거의 신뢰도로 가고, `NOT_EVALUATED` + `UNOBSERVED`는 지금대로 필수 리뷰다 |

### F. 리뷰-확정 상태기계 (§3.3③, 설계 §1.4) ✅: 핵심
| TC | Given → When | Then |
|---|---|---|
| [IC-F1](../pkg/inventory/decision/session_test.go) ✅ | 신규 판정 대상 | 상태 = **draft** |
| [IC-F2](../pkg/inventory/decision/session_test.go) ✅ | draft → in-review 전이 | 허용 |
| [IC-F3](../pkg/inventory/decision/session_test.go) ✅ | in-review에서 finalize (전 필수항목 판정 + 판정자 서명 有) | **finalized**. 이 리포의 finalized는 판정 세션이 닫혔다는 뜻이고 계약의 `FINALIZED`(실행 승인 완료)와 다르다 |
| [IC-F4](../pkg/inventory/decision/session_test.go) ✅ | 필수 항목 미판정 상태로 finalize 시도 | **거부**(전 필수 판정 전 불가) |
| [IC-F5](../pkg/inventory/decision/session_test.go) ✅ | 판정자 서명 없이 finalize | **거부** |
| [IC-F6](../pkg/inventory/decision/session_test.go) ✅ | 링/도메인 단위 부분 확정 | 허용(부분 finalize, §3.3③) |
| [IC-F7](../pkg/inventory/decision/session_test.go) ✅ | 정책 단위 판정(버전×링크모드 템플릿) | 동종 자산 일괄 적용(§3.4) / 예외만 엣지 단위 |

### D. 판정 영속화 (§3.6, 설계 §1.5) ✅: 파일과 Postgres
| TC | Given → When | Then |
|---|---|---|
| [IC-D1](../pkg/inventory/decision/judgment_test.go) ✅ | 판정 후 재수집(새 스냅샷) | 판정(사람의 결론)이 **그대로 붙어 있다**. 엣지 상태가 바뀌어도 결론은 남는다 |
| [IC-D2](../pkg/inventory/decision/judgment_test.go) ✅ | 근거 증거(BasisHash) 실질 변화 | **해당 판정만** 재검토 플래그(델타 리뷰), 나머지 유지 |
| [IC-D3](../pkg/inventory/decision/judgment_test.go) ✅ | 근거 불변 | 판정 유지(재리뷰 안 함) |
| [IC-D4](../pkg/inventory/decision/judgment_test.go) ✅ | stale 판정 + 만료 경과 | 신뢰도 감쇠 + 주기 재확인 플래그 |
| [IC-D5](../pkg/inventory/decision/pg_test.go) ✅ | 영속화(Postgres) 라운드트립 | Decision 보존(append-only, §1.2) |
| [IC-D6](../pkg/inventory/decision/file_test.go) ✅ | 같은 대상을 다시 판정 | **파일 원장도 쌓기만 한다**. 덮어쓰면 「언제 무엇으로 바뀌었나」가 사라진다(§1.2) |
| [IC-D7](../pkg/inventory/decision/file_test.go) ✅ | 다른 조직의 판정이 섞인 파일 | 읽지 않는다. 파일은 누구나 이어 쓸 수 있어, 거르지 않으면 격리가 파일 권한에만 기댄다 |
| [IC-D8](../pkg/inventory/decision/file_test.go) ✅ | 조직 없이 열기 · 아직 아무것도 없는 파일 | 조직 없이는 열리지 않는다(Mem·Pg와 같은 규칙). 빈 파일은 오류가 아니다 |
| [IC-D19](../pkg/inventory/review/basis_test.go) ✅ | 같은 관측을 두 번 · 규칙 판·대조 상태·신뢰도·정책·관측 지문·재수집 후보 여부를 하나씩 바꾸기 | 같으면 근거 해시가 같고, **여섯 가운데 하나라도 움직이면 달라진다.** 근거를 세는 자리는 `BasisOf` 하나다 |
| **[IC-D20](../pkg/inventory/review/basis_test.go) ✅** | **규칙 판은 그대로인데 관측 지문(또는 신뢰도)만 달라진 세션을 다시 열기** | **승인 서명이 지워진다.** 전에는 id와 상태만 비교해서, 델타 리뷰에는 올라오는 변화가 서명은 그대로 지나갔다. 승인자가 본 적 없는 근거에 이름이 남는 자리다 |
| [IC-D21](../pkg/inventory/review/basis_test.go) ✅ | 항목이 하나도 없는 세션에서 규칙 판만 바뀌기 | 서명이 지워진다. 항목별 비교만 하면 빈 큐에서는 전부 참이라 규칙 변경이 지나간다 |
| **[IC-D22](../pkg/inventory/decision/session_id_test.go) ✅** | **두 세션의 판정과 옛 빌드의 행(세션 id 없음)을 한 원장에 · 계획 id에서 세션 id를 읽어 원장을 찾기** | `BySessionID`가 그 세션의 판정만 돌려주고 다른 세션 것은 섞이지 않는다. **메모리·파일·Postgres 셋이 같은 답이다.** 빈 id로 찾으면 거절한다. 옛 행의 값이 비어 있어서, 그것으로 찾으면 세션이 아니라 「세션을 모르는 판정 전부」가 나온다. Postgres는 열과 `(org, session_id, seq)` 인덱스를 `ALTER TABLE … IF NOT EXISTS`로 더한다 |
| **[IC-D23](../pkg/inventory/decision/judgment_test.go) ✅** | 만료된 판정이 평가됨 · 미평가 · 계획 선택 행 | 평가된 것은 `Stale`·재확인이 서고 **숫자가 감쇠**한다. **미평가는 표시만 서고 숫자는 그대로다** - 재지 않은 값을 줄이면 「재 봤더니 더 낮아졌다」로 읽힌다. 계획 선택 행은 만료 계산에 아예 들어가지 않는다 |
| **[IC-D24](../pkg/inventory/decision/judgment_test.go) ✅** | 같은 자산에 판정 행 뒤에 계획 선택 행이 쌓임 · 계획 선택 행만 있는 자산 | **최신 판정과 델타 판정의 결과가 달라지지 않는다.** `LatestPerSubject`는 판정 행을 돌려주고, `DeltaReview`는 계획 선택 행에 재검토 표시를 붙이지 않는다. 옛 행(빈 종류)은 판정으로 읽는다 |
| **[IC-D25](../pkg/inventory/decision/file_test.go) ✅** | 파일 원장의 옛 줄(`ConfidenceEvaluated` 칸 없음) · 새 미평가 줄 · 계획 선택 줄 | 옛 줄은 **평가된 판정**으로 읽히고, 새 줄의 명시적 `false`는 그대로 보존된다. 새로 쓴 줄에는 부재가 없다. `Judgment`에 json 태그가 없어 `bool`로 두면 옛 줄의 부재가 `false`로 읽히고, 그러면 옛 판정 전부가 미평가로 둔갑해 만료 시 신뢰도가 감쇠되지 않는다 - wire 형식이 포인터로 받는다 |
| **[IC-D26](../pkg/inventory/decision/migrate_pg_test.go) ✅** | v0.17 모양의 Postgres 원장(`record_kind` · `confidence_evaluated` 열 없음)을 이 판이 엶 · 옛 행 하나 · 새 미평가 행 · 계획 선택 행 | 두 열이 `ADD COLUMN IF NOT EXISTS … DEFAULT`로 더해지고, **옛 행은 평가된 판정으로 이행**된다(기본값 `TRUE`). 새 행은 명시적으로 저장된다. 최신 판정은 계획 선택 행을 세지 않는다. 전용 스키마에서 돌아 공유 표의 모양을 흔들지 않는다. `PQCOTA_TEST_DSN`이 있을 때만 |
| **[IC-D27](../pkg/inventory/decision/schema_pg_test.go) ✅** | 같은 데이터베이스의 두 스키마에 같은 이름의 표(하나는 이 판의 모양, 하나는 옛 모양) · 각각을 보는 연결 | 옛 표를 보는 연결은 **준비되지 않았다**, 새 표를 보는 연결은 **준비됐다**. `to_regclass`가 고른 관계의 OID를 `pg_attribute`에 맞댄다 | 표 이름으로만 세면 앞은 다른 스키마의 열을 보고 준비됐다고 하고, 뒤는 열이 넷이라 준비되지 않았다고 한다. `PQCOTA_TEST_DSN`이 있을 때만 |
| **[IC-D28](../pkg/inventory/review/installations_test.go) ✅** | 근거 하나의 설치 경로만 바꿈 · 경로 칸을 비운 근거 | 경로가 바뀌면 **근거 해시가 바뀐다.** 경로를 모르는 근거의 해시는 칸이 생기기 전과 같다 | 경로가 다르면 다른 설치를 본 것이라 지난 판정 · 확인의 근거가 아니다 |
| **[IC-D29](../inventory/cmd/pqcaton-decide/scope_policy_test.go) ✅** | 정책 지문이 없는 옛 세션 파일을 `close`(-o · 표준출력) | 거절하고 새 세션을 열라고 안내한다. 계획도 세션도 쓰지 않는다 |
| **[IC-D30](../inventory/cmd/pqcaton-decide/scope_policy_test.go) ✅** | 정책 A로 연 세션을 정책 B로 · `-scope-assets` 없이 · 정책 A로 `close` | 앞의 둘은 거절하고 아무것도 쓰지 않는다. A면 확정하고 영수증이 형식 2로 A의 지문을 적는다 |
| [IC-D31](../inventory/cmd/pqcaton-decide/scope_policy_test.go) ✅ | 정책 없이 연 세션을 `-scope-assets` 없이 `close` | 확정하고 영수증이 형식 2로 규칙 0개의 지문을 적는다 |
| **[IC-D32](../inventory/cmd/pqcaton-decide/scope_policy_test.go) ✅** | `open`을 `-scope-assets`와 함께 · 없이 | 세션에 실제로 읽은 정책의 지문(없으면 규칙 0개의 지문)이 적힌다. 지문 칸이 빈 세션은 나오지 않는다 |
| **[IC-D33](../inventory/cmd/pqcaton-decide/scope_cmd_test.go) ✅** | `activate-scope`를 `-confirm-ingest`로 · PQCOTA_DSN 없이 · 손으로 고친 후보로(그 정책으로 적재됨) · 새 정책으로 적재한 뒤 · 같은 정책으로 다시 | **pqcota 이력 대조를 언제나 한다.** `-confirm-ingest`는 지원 종료 오류, DSN이 없으면 `ErrNoDSN`, 손으로 고친 후보는 `ErrNoCandidate`이고 모두 아무것도 쓰지 않는다. 대조가 통과하면 정책이 바뀌고 새 세션이 새 지문을 들며 처리 기록의 basis가 upstream-verified, 보고서에 `upstream_check`가 실린다. 같은 정책이면 아무것도 쓰지 않는다 |
| **[IC-D34](../inventory/cmd/pqcaton-decide/scope_cmd_test.go) ✅** | `activate-scope`를 pqcota가 옛 정책으로만 적재된 상태 · pqcota에 닿지 못하는 상태로 | **pqcota가 새 정책으로 적재돼 있지 않으면 활성화하지 않는다.** 차례로 `ErrMissing` · `ErrUnreachable`이고 정책 · 세션이 그대로다 |
| **[IC-D35](../inventory/cmd/pqcaton-decide/scope_cmd_test.go) ✅** | `baseline`을 `-confirm-ingest`로 · 지문 있는 세션에서 · pqcota가 다른 정책으로 적재됨 · 지금 정책으로 적재됨 | **기준선도 같은 대조를 거친다.** 지원 종료 오류 · `ErrNotBaseline` · `ErrMissing`(아무것도 쓰지 않음)이고, 마지막은 새 세션이 지금 정책의 지문을 들고 basis가 upstream-verified다 |
| **[IC-D36](../inventory/cmd/pqcaton-decide/scope_cmd_test.go) ✅** | `activate-scope -dry-run`을 준비된 상태 · 손으로 고친 후보 · 같은 정책 · 지금 정책으로 판정되지 않은 세션으로, 그리고 `scope-status` | 차례로 통과 · `ErrNoCandidate` · `ErrSamePolicy` · 정책 잠금이고, 어느 경우에도 정책 · 세션 · 영수증이 그대로다. `scope-status`는 활성 정책 · 세션 지문 · 세션 ID · 잠금을 한 줄씩 적는다 |
| **[IC-D37](../inventory/cmd/pqcaton-decide/scope_cmd_test.go) ✅** | 대조하는 동안 결과 폴더에 파일이 생기는 pqcota 이력으로 `activate-scope` | **입력 스냅샷은 대조보다 먼저 뜬다.** 대조가 통과해도 잠금 안에서 `ErrStale`로 거절하고 정책 · 세션 · 처리 기록이 그대로다. 대조하지 않은 입력으로 upstream-verified를 쓰지 않는다 |
| **[IC-D38](../inventory/cmd/pqcaton-decide/scope_cmd_test.go) ✅** | `activate-scope`에 `-org`와 다른 `-upstream-org` · 다른 조직의 판정 세션 | **pqcota 이력의 조직은 판정 조직이다.** 앞은 이력을 열지 않고, 뒤는 조회하지 않고 `ErrOrg`이며 정책 · 세션이 그대로다 |

### W. 저장 안전성 (`pkg/inventory/store`) 🔶: 저장소 계층 완료, 화면 연결 전
화면과 명령이 파일을 바꿀 때 지나가는 한 길입니다. 상용 지원은 Linux 로컬 파일시스템뿐입니다.

| TC | Given → When | Then |
|---|---|---|
| [IC-W1](../pkg/inventory/store/state_test.go) ✅ | 상태 디렉토리를 처음 엶 | 디렉토리(0700)와 무작위 상태 ID(0600)를 만든다. 다시 열면 같은 ID를 읽고, 다른 상태 디렉토리는 다른 ID를 받는다 |
| [IC-W2](../pkg/inventory/store/state_test.go) ✅ | 상태 디렉토리가 0700보다, 안의 파일이 0600보다 넓음 | **열지 않고 `chmod`를 안내한다.** 권한을 고쳐 주지도 않는다. 넓은 권한은 이미 누가 읽었을 수 있다는 뜻이라 아무 표시 없이 좁히면 그 사실이 묻힌다 |
| [IC-W3](../pkg/inventory/store/state_test.go) ✅ | 심볼릭 링크인 상태 디렉토리 · 하드 링크가 둘인 상태 파일 · 안에 든 심볼릭 링크 | 모두 거부한다. 다른 이름을 거쳐 바뀌면 이쪽이 모르는 변경이 된다 |
| [IC-W4](../pkg/inventory/store/state_test.go) ✅ | 다른 사용자가 소유한 상태 디렉토리 | 거부한다. rename으로 교체하면 소유자가 바뀐다 |
| [IC-W5](../pkg/inventory/store/state_test.go) ✅ | 서버가 실행 잠금을 쥔 채 명령이 같은 상태 디렉토리를 씀 | 명령은 **기다리지 않고** 거부된다. 서버가 놓으면 잡힌다. 같은 상태 디렉토리로 서버 둘도 같은 이유로 막힌다 |
| [IC-W6](../pkg/inventory/store/state_test.go) ✅ | 대상 디렉토리를 다른 상태 디렉토리로 씀 · 상태 디렉토리와 함께 옮긴 데이터 | 잠금 파일에 적힌 상태 ID가 다르면 거부하고 `-state-dir`를 안내한다. 함께 옮긴 데이터는 그대로 열린다. 경로가 아니라 ID로 본다 |
| [IC-W7](../pkg/inventory/store/state_test.go) ✅ | 다른 요청이 대상 잠금을 쥠 | 정해진 시간까지 기다린다. 그 안에 풀리면 잡고, 못 잡으면 「다른 저장이 진행 중」으로 끝나며 그동안 잡은 잠금도 놓는다 |
| [IC-W8](../pkg/inventory/store/platform_test.go) ✅ | NFS · CIFS · SMB 위의 상태 디렉토리 | 시작하지 않는다. 잠금과 rename의 원자성을 믿을 수 없다. 로컬 파일시스템은 지나간다 |
| [IC-W9](../pkg/inventory/store/file_test.go) ✅ | 파일 교체 | 준비 파일을 다 쓰고 fsync한 뒤 rename으로 바꿔 끼운다. 있던 파일의 권한은 이어받고 새 파일은 0600이다. 끝나면 준비 파일이 남지 않는다. 남이 읽을 수 있는 파일은 경고만 한다 |
| [IC-W10](../pkg/inventory/store/file_test.go) ✅ | 심볼릭 링크 · 하드 링크 둘 · 남의 소유 · group/other 쓰기 권한인 대상 | 쓰지 않고, 대상은 한 바이트도 바뀌지 않는다. 쓰기 권한이 열린 파일은 남이 이미 바꿨을 수 있다 |
| [IC-W11](../pkg/inventory/store/file_test.go) ✅ | 준비 파일 쓰기 · fsync · rename · 디렉토리 fsync 앞에서 멈춤 | 대상은 옛 내용이거나 새 내용이다. 반쯤 쓴 내용은 보이지 않는다. 남은 준비 파일은 지우지 않는다(누가 만든 것인지는 기록이 정한다) |
| [IC-W12](../pkg/inventory/store/file_test.go) ✅ | 준비 파일 자리에 파일이나 심볼릭 링크가 있음 | 만들지 않고 따라가지도 않는다. 있던 것은 그대로 남는다 |
| [IC-W13](../pkg/inventory/store/txn_test.go) ✅ | 확정 한 번(계획 교체와 백업 · 세션 교체 · 원장 추가 · 영수증 생성) | 함께 끝난다. 끝나면 준비 파일도 트랜잭션 기록도 남지 않는다. 기록과 준비 파일은 0600이다 |
| **[IC-W14](../pkg/inventory/store/txn_test.go) ✅** | 기록 쓰기 · 준비 파일 · PREPARED 전환 · 항목마다 · rename · 원장 절반 · 기록 삭제 앞에서 멈춘 뒤 다시 엶 | **확정 전이거나 확정 후다. 그 중간은 없다.** PREPARED 전에 멈추면 기록에 적힌 준비 파일만 걷고, 그 뒤면 끝까지 간다. 확정 전으로 돌아간 뒤에는 같은 확정을 다시 할 수 있다 |
| [IC-W15](../pkg/inventory/store/txn_test.go) ✅ | 기록에 없는 준비 파일 | 지우지 않고 멈추며 그 파일을 이름으로 알린다. 누가 만든 것인지 가릴 수 없다. 복구는 기록에 열거된 것만 걷는다 |
| **[IC-W16](../pkg/inventory/store/txn_test.go) ✅** | 원장에 추가분이 반만 쓰이고 멈춤 | **자르지 않고** 남은 바이트를 이어 쓴다. 원장은 한 번도 짧아지지 않고, 추가분은 정확히 한 번 들어간다. 상태 보기는 「partial」로 보인다 |
| [IC-W17](../pkg/inventory/store/txn_test.go) ✅ | 반만 쓰인 원장의 넘친 바이트가 다름 · 기준 길이 앞부분이 바뀜 · 추가분 뒤에 남의 바이트 | 복구 필요로 멈춘다. 원장은 한 바이트도 바뀌지 않는다 |
| **[IC-W18](../pkg/inventory/store/txn_test.go) ✅** | 확정 도중 대상이 밖에서 바뀜 | 덮어쓰지 않고 복구 필요로 멈추며, 뒤의 항목(원장, 영수증)도 적용하지 않는다. 상태 보기는 파일 · 기대 해시 · 지금 해시 · 준비 파일을 보이고, 진단 묶음을 0600으로 새로 내보낸다(파일 내용은 담지 않는다). 그동안 그 디렉토리의 다른 저장은 막힌다 |
| [IC-W19](../pkg/inventory/store/txn_test.go) ✅ | 같은 원장에 동시에 쓰는 요청 여럿 | 차례로 들어간다. 줄이 섞이거나 빠지거나 두 번 들어가지 않는다 |
| **[IC-W20](../pkg/inventory/store/receipt_test.go) ✅** | 같은 세션을 같은 내용으로 다시 확정 · 서버를 다시 띄운 뒤 다시 확정 | 첫 확정이 영수증(0600)을 남긴다. 다시 확정하면 아무것도 다시 쓰지 않고 이전 결과를 돌려준다. **원장에 판정이 두 번 쌓이지 않는다.** 동일성은 종류 · 조직 · 세션 ID이고 내용은 영수증에 해시로만 적는다 |
| [IC-W21](../pkg/inventory/store/receipt_test.go) ✅ | 같은 세션을 다른 내용으로 다시 확정 | 거부하고 새 세션을 열라고 알린다. 아무것도 바뀌지 않는다 |
| [IC-W22](../pkg/inventory/store/receipt_test.go) ✅ | 확정된 세션의 일반 저장 | 막는다. 확정한 뒤에 세션이 바뀌면 영수증이 가리키는 것과 화면이 다르다. 다른 세션의 저장은 지나간다 |
| [IC-W23](../pkg/inventory/store/receipt_test.go) ✅ | 다른 세션이 뒤에 같은 출력을 확정한 뒤 앞 세션을 다시 확정 | 「다른 세션의 후속 확정으로 대체됨」으로 알리고 다시 쓰지 않는다. 오류가 아니다 |
| [IC-W24](../pkg/inventory/store/receipt_test.go) ✅ | 확정한 출력을 손으로 고침 · 원장 구간이 영수증과 다름 | 복구 필요로 알리고 다시 쓰지 않는다 |
| **[IC-W25](../pkg/inventory/store/receipt_test.go) ✅** | 영수증을 쓰기 직전에 멈춤 · 같은 세션을 동시에 두 번 확정 | 다시 열어 복구하면 영수증까지 끝나고, 그 뒤 같은 확정은 이전 결과다. 동시에 두 번이면 한쪽만 쓰고 다른 쪽은 이전 결과를 받는다. 어느 쪽이든 판정은 원장에 한 번만 들어간다 |
| [IC-W26](../pkg/inventory/store/receipt_test.go) ✅ | 종류 · 조직 · 세션 ID에 경로 구분자 · 「..」 · 빈 값 | 받지 않는다. 영수증 경로가 상태 디렉토리 밖으로 나가지 않는다 |
| **[IC-W27](../pkg/inventory/store/rev_test.go) ✅** | 세션 · 레이어 · 관측 파일의 한 바이트, 관측 파일의 추가 · 이름 바꿈, 입력의 자리, 「없음」과 「빈 파일」 | 하나라도 다르면 rev(화면이 본 입력의 지문)가 다르다. 입력이 같으면 rev가 같다. 저장 요청의 rev가 지금과 다르면 화면이 오래된 것이다 |
| [IC-W28](../pkg/inventory/store/rev_test.go) ✅ | 읽는 동안 입력이 바뀜 | 다시 읽어 바뀐 뒤의 rev를 낸다. 계속 바뀌면 「읽는 동안 바뀌었다」로 끝난다. 반쯤 바뀐 상태의 rev는 내지 않는다 |
| [IC-W29](../pkg/inventory/store/rev_test.go) ✅ | 크기와 수정 시각은 그대로 두고 내용만 바꿈 | rev가 바뀐다. 캐시하지 않고 매번 내용을 읽는다 |
| **[IC-W30](../pkg/inventory/store/check_test.go) ✅** | 같은 오래된 화면에서 동시에 저장 · rev 없음 · 확인한 것과 다른 확정 내용 · 잠근 대상 밖의 파일 | **rev는 저장 잠금 안에서 견준다.** 동시에 저장하면 하나만 쓰이고 나머지는 「화면이 오래됐다」로 거부된다. rev가 없거나 다르면, 확정 내용이 확인한 것과 다르면, 잠그지 않은 파일을 쓰려 하면 쓰지 않는다. 이미 같은 내용으로 확정됐으면 rev가 낡았어도 이전 결과다 |
| [IC-W31](../pkg/inventory/store/check_test.go) ✅ | 링크는 그대로인 채 대상 파일만 바뀜 · 링크가 끊김 · 입력 디렉토리 자체가 링크 · 안쪽의 디렉토리 링크 | 앞의 셋은 rev가 바뀐다. 링크가 가리키는 파일의 내용까지 본다. 안쪽의 디렉토리 링크는 따라가지 않고 거부한다 |
| [IC-W32](../pkg/inventory/store/check_test.go) ✅ | 영수증의 벽시각이 거꾸로 적힘 | 확정의 앞뒤는 영수증 순번으로 가린다. 뒤에 확정한 세션이 대체한 쪽이다 |
| **[IC-W33](../inventory/cmd/pqcaton-decide/main_test.go) ✅** | `pqcaton-decide close`에 `-judgments`만 주고 `-o`는 안 줌 | 거부하고 아무것도 쓰지 않는다. 원장과 표준출력은 한 트랜잭션으로 묶이지 않는다. 둘 다 없으면 예전처럼 표준출력으로만 계획을 내고 원장 · 영수증은 쓰지 않는다 |
| **[IC-W34](../inventory/cmd/pqcaton-decide/main_test.go) ✅** | `close -o plan.json -judgments …`를 두 번 | 세션 · 계획 · 판정 원장 · 영수증을 화면과 같은 트랜잭션으로 쓴다. 두 번째는 아무것도 다시 쓰지 않는다. 표준출력은 비어 있다 |
| [IC-W35](../inventory/cmd/pqcaton-decide/main_test.go) ✅ | 화면이 떠 있는 동안 `close -o` | 기다리지 않고 거부되며 아무것도 쓰지 않는다 |
| [IC-W36](../inventory/cmd/pqcaton-scope/main_test.go) ✅ | `pqcaton-scope close`(`-judgments`만 · `-o`로 두 번) | 리뷰 확정과 같은 규칙이다. `open`이 세션 ID를 박는다 |
| [IC-W37](../inventory/cmd/pqcaton-ui/recover_test.go) ✅ | `pqcaton-ui -recover-status [-diagnostics 파일]` | 고치지 않고 보여 준다. 남은 기록마다 전체 경로 · 기대 해시 · 지금 해시 · 준비 파일의 상태와 경로 · 판정을 보이고, 진단 묶음을 0600으로 새로 낸다. 남은 기록이 없으면 없다고 알린다 |
| **[IC-W38](../pkg/inventory/store/check_test.go) ✅** | 처음 rev를 견준 뒤, 저장 · 확정이 입력을 읽는 사이에 손 편집이 입력을 바꿈 | 첫 기록을 쓰기 직전에 다시 견주어 「화면이 오래됐다」로 멈춘다. 기록도 쓰지 않고, 밖에서 바꾼 내용은 그대로 남는다. 그 뒤에 바뀐 대상 파일은 기록의 적용 전 해시가 막는다 |
| **[IC-W39](../pkg/inventory/store/verify_test.go) ✅** | 확정 전 · 확정 뒤 · 계획을 손으로 고침 · 원장 구간을 잘라 냄 | **확정 상태는 영수증이 있는지가 아니라 출력 해시와 원장 구간으로 판정한다**(`VerifyFinalized`). 미확정 · 확정됨 · 복구 필요 · 복구 필요다. 판정은 아무 파일도 바꾸지 않고 기록도 남기지 않는다 |
| [IC-W40](../pkg/inventory/store/verify_test.go) ✅ | 출력 디렉토리에 잠금 파일이 없음 | 만들지 않고 잠그지 않은 채 판정한다. 읽기가 남의 디렉토리에 파일을 남기면 안 된다 |
| **[IC-W41](../pkg/inventory/store/verify_test.go) ✅** | 확정 뒤 다른 요청이 대상 잠금을 쥠 · 끝나지 않은 기록이 그 디렉토리를 건드림 | 기다리다 `ErrTargetBusy` · `ErrInProgress`다. 어느 쪽도 「복구 필요」로 단정하지 않는다 |
| **[IC-W42](../pkg/inventory/store/verify_test.go) ✅** | 뒤 세션이 같은 계획을 다시 확정 · 뒤 영수증의 순번을 앞보다 작게 고침 | 앞 세션은 대체됨, 뒤 세션은 확정됨이다. 순번이 설명되지 않으면 앞 세션은 복구 필요다 |
| **[IC-W43](../pkg/inventory/store/verify_test.go) ✅** | 영수증이 아직 없는데 다른 요청이 대상 디렉토리를 쥠 | **대상 잠금은 영수증을 처음 읽기 전에 잡는다.** 기다리다 `ErrTargetBusy`다. 읽고 나서 잠그면 그 사이 끝난 확정을 놓친 「미확정」이 나간다 |
| **[IC-W44](../pkg/inventory/store/verify_test.go) ✅** | 두 세션이 같은 계획을 번갈아 확정하는 동안 판정을 거듭함 | 확정됨 · 대체됨 · 확인할 수 없음만 나온다. 「복구 필요」나 「미확정」으로 잘못 판정하지 않는다 |
| **[IC-W45](../pkg/inventory/store/txn_test.go) ✅** | 없는 파일만 만들기(Create)에 이미 있는 파일을 섞음 | `ErrExists`이고 아무것도 쓰지 않는다. 있는 파일은 그대로, 없던 파일도 생기지 않는다 |
| **[IC-W46](../pkg/inventory/store/round_test.go) ✅** | 확정한 세션에서 새 회차를 열고, 새 세션을 저장해 세션 파일이 또 바뀜 · 옛 세션의 같은 확정을 다시 보냄 | 새 회차는 확정한 세션에서만, 한 번만 열린다. 새 세션 파일과 회차 표지가 함께 남고, 옛 세션은 「복구 필요」가 아니라 「대체됨」이다. 옛 확정을 다시 보내도 아무것도 쓰지 않는다. 넘겨받지 않은 출력(계획)이 밖에서 바뀌면 여전히 복구 필요다 |
| **[IC-W47](../pkg/inventory/store/round_test.go) ✅** | 새 회차를 여는 트랜잭션의 각 자리에서 멈추고 다시 열어 복구 | 세션 파일과 회차 표지가 함께 있거나 함께 없다. 옛 세션은 어느 경우에도 복구 필요로 읽히지 않는다 |
| **[IC-W48](../pkg/inventory/store/platform_test.go) ✅** | macOS에서 상태 디렉토리를 엶(Intel · Apple Silicon) | Intel(amd64)은 검증한 지원 플랫폼이라 경고하지 않는다. Apple Silicon(arm64)은 검증하지 않았다는 경고를 남긴다. 네트워크 파일시스템 거부는 그대로다 |
| **[IC-W49](../pkg/inventory/store/move_test.go) ✅** | 여러 파일의 이름 바꾸기(`MoveChecked`)가 준비 · 기록 · 적용 · 완료 직전에 멈춤 · 새 이름 하나가 이미 있음 | 다시 열어 복구하면 모두 옮겨졌거나 모두 그대로다. 새 이름을 다 만든 뒤에 옛 이름을 지운다. 새 이름이 있으면 `ErrExists`이고 아무것도 옮기지 않는다 |
| **[IC-W50](../pkg/inventory/store/move_test.go) ✅** | 다른 요청이 폴더를 쥔 동안 일반 트랜잭션(`Do`)으로 지우기 | 만들기 · 지우기의 폴더도 잠근다. 기다리다 `ErrTargetBusy`이고 아무것도 지우지 않는다 |
| **[IC-W51](../pkg/inventory/store/ruleset_round_test.go) ✅** | 규칙 전환 회차를 기록 · 준비 · 적용의 자리마다 멈춘 뒤 다시 열어 복구 | 보관본 · 새 세션 · 보고서 · 회차 표지가 **넷 다 있거나 넷 다 없다.** 넷 다 없으면 옛 세션 파일이 바이트 그대로다 |
| **[IC-W52](../pkg/inventory/store/ruleset_round_test.go) ✅** | 확정된 세션에서 규칙 전환 · 같은 전환을 다시 · 보관 자리가 이미 있음 · 확정 전 세션에서 새 세션을 다른 자리에 | 영수증은 그대로이고 **회차 표지가 옛 영수증 해시 · 옛 세션 해시 · 새 세션 ID를 잇는다.** 옛 세션은 「대체됨」, 보관본은 옛 바이트 그대로다. 다시 보내면 ErrStale이고 아무것도 더 만들지 않는다. 보관 자리가 있으면 ErrExists이고 아무것도 쓰지 않는다. 다른 자리에 쓰면 옛 파일은 제자리에 남는다 |
| **[IC-W53](../inventory/cmd/pqcaton-decide/newround_test.go) ✅** | 명령 `new-round`를 -o 없이 · 다른 자리의 -o로 · 같은 명령을 다시 · 옛 세션 자리의 -o로 | -o가 없으면 아무것도 하지 않는다. 다른 자리면 옛 파일이 **바이트 그대로 남고** 새 세션과 보고서가 생긴다. 다시 돌리면 거절되고 파일이 늘지 않는다. 옛 자리면 옛 바이트가 보관본으로 옮겨진다 |
| **[IC-W58](../pkg/inventory/store/receipt_test.go) ✅** | 정책 지문을 주어 확정 · 지문의 모양이 아닌 값을 주어 확정 · 확정한 계획 파일로 영수증 찾기 | 지문을 주면 영수증이 형식 2이고 그 지문을 적으며, 출력과 같은 트랜잭션의 마지막 항목이다. **형식 2는 지문 없이 쓸 수 없다**: 모양이 아니면 ErrPolicyDigest이고 계획도 영수증도 쓰지 않는다. 계획 파일은 그것을 지금 내용 그대로 쓴 세션의 영수증으로만 이어진다(다른 세션 · 바뀐 파일 · 쓸 수 없는 동일성은 거절) |
| **[IC-W59](../pkg/inventory/store/receipt_test.go) ✅** | 형식 1 영수증 · 지문을 담은 형식 1 · 형식 2(지문 있음 · 칸 없음 · 빈 칸) · 더 새 형식을 이 판으로 읽음 | 형식 1과 2는 읽히고 같은 확정을 다시 보내면 「이미 확정됨」이다. 정책 지문은 형식 2에 값이 있을 때만 나온다. **지문이 없는 것을 규칙 0개의 지문으로 읽지 않는다.** 더 새 형식은 손상이다 |
| **[IC-W61](../pkg/inventory/rulesetround/baseline_test.go) ✅** | 기준선 회차를 적용 중 · 끝난 자리에서 멈춘 뒤 다시 열어 복구 | 보관본 · 새 세션 · 보고서 · 표지 · scope-baseline 영수증이 **다 있거나 다 없다** |
| **[IC-W62](../pkg/inventory/rulesetround/activation_test.go) ✅** | 활성화를 적용 중 · 끝난 자리에서 멈춘 뒤 복구 · 정책 보관 자리가 이미 있음 | 정책 보관본 · 활성 정책 교체 · 옛 세션 보관본 · 새 세션 · 보고서 · 표지 · 영수증이 **다 있거나 다 없다.** 보관 자리가 있으면 `ErrExists`이고 아무것도 쓰지 않는다 |
| **[IC-W63](../pkg/inventory/store/receipt_test.go) ✅** | 새 상태 디렉토리 · 옛 판(형식 1)의 상태 디렉토리에서 형식 1 영수증만 씀 · 정책 지문을 담은 확정(올리는 자리에서 멈춤 · 끝까지) · 형식 1 · 2가 아닌 상태 문서 | 새 상태 디렉토리는 형식 2다. 형식 1 영수증만 쓰는 동안 옛 상태는 형식 1로 남는다. **형식 2 영수증을 쓰기 직전에 상태 문서를 형식 2로 올린다**(ID와 0600 그대로). 올리는 자리에서 멈추면 영수증도 출력도 쓰지 않는다. 형식 1 · 2가 아니면 손상이다. v0.25.x 이하는 형식 1이 아니면 상태 디렉토리를 열지 않는다 |

### P. 확정 계획 & 핸드오프 (§3.7, §5, [인벤토리 설계 §4](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/inventory/design.md)) ✅
| TC | Given → When | Then |
|---|---|---|
| [IC-P1](../pkg/inventory/decision/plan_test.go) ✅ | finalized 계획 생성 | PlanItem[]: node·remediation_class·**deploy_automation_level**·provider_choice |
| [IC-P2](../pkg/inventory/decision/plan_test.go) ✅ | deploy_automation_level 판정 | 자산별로 리뷰어가 판정한다(§4.7 MANUAL). 전사 일괄이 아니다 |
| [IC-P3](../pkg/inventory/decision/plan_test.go) ✅ | 규제 대상 자산(fips_validation 요구) | **FIPS 검증 provider로 라우팅 강제**([pqcota 프로비저닝 설계 §4.2](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/provisioning/design.md#42-jca-브랜치-jcago-jdk-세대와-provider가-kind를-결정한다), Java=BC-FJA) · **CNG는 빈 값이다**. 갈아 끼울 provider가 관측에 없고 FIPS 여부는 알 수 없다(§2.5). 이름을 지어내면 계획을 받는 쪽이 검증된 선택으로 읽는다 |
| **[IC-P4](../pkg/inventory/decision/plan_test.go) ✅** | **판정이 끝나지 않은 세션에서 계획을 만들거나, 판정자 표시 없는 계획을 넘기려 한다** | **거부한다**(`ErrNotJudged`, §5. 반드시 거쳐야 하는 관문). 이 관문은 실행을 허용하는 것이 아니라 **계약으로 넘길 수 있는지**를 본다(`ReadyForApproval`). 실행 허용은 상류의 `Executable`과 승인 검증이 한다 |
| [IC-P5](../pkg/inventory/decision/plan_test.go) ✅ | 판정이 끝난 세션에서 계획 | 만들어진다(`JudgedPlan`). 실행 근거가 되는 것은 상류의 승인이 `FINALIZED`로 올린 뒤다(§3.7) |
| **[IC-P6](../pkg/inventory/review/review_test.go) ✅** | 스코프가 URI인 노드(`host://local`)를 계약 형식으로 | **겨눈 노드와 런타임이 그대로 간다**. v0.1.0은 id를 쪼개 `host:`를 겨누고 런타임을 기본값으로 떨어뜨렸다 |
| [IC-P7](../pkg/inventory/review/review_test.go) ✅ | node가 빈 항목을 계획에 | 확정 직전에 거부하고 `open`을 다시 돌리라고 알려 준다. 이름 없는 노드에 조치를 걸지 않는다 |
| **[IC-P8](../pkg/inventory/review/session_id_test.go) ✅** | **판정이 끝난 세션을 계약으로** | 상태는 **`IN_REVIEW`**, `approval_signatures`와 `finalized_at`은 **비어 있다.** 계획 id는 `pqcaton:<스코프>:<세션 id>`이고 검토자가 적은 문자열은 들어가지 않는다. 판정과 실행 승인은 다른 단계라 승인 칸은 상류가 채운다. 전에는 검증되지 않는 판정자 문자열을 승인 칸에 넣고 `FINALIZED`를 달아, 아무것도 증명하지 않는 값이 상류 구조 관문의 「승인 항목 있음」을 충족하는 모양이 됐다 |
| [IC-P9](../pkg/inventory/review/session_id_test.go) ✅ | 세션 id 없는 세션(옛 빌드)을 확정 · 세션을 다시 열기 · 같은 내용의 새 세션 · 앞 세션에 id가 없는 채로 다시 열기 | 첫째는 **거부하고 다시 열라고 안내한다.** 여기서 새로 찍으면 계획은 세션을 가리키는데 원장에는 그 세션이 없다. 다시 열면 같은 id, 새 세션이면 다른 id, 앞 세션에 id가 없으면 새 id를 지킨다 |
| **[IC-P10](../pkg/inventory/review/evidence_test.go) ✅** | 근거 둘인 항목을 계약으로 · 보조 근거를 빼거나 지문을 바꾸거나 순서를 바꾸거나 스냅샷 지문을 바꿈 · 지문 없는 근거(옛 세션) | `evidence_sources`가 둘, 주 근거가 앞이고 호환 `finding_id`와 같다. 참조는 내용 지문(규격 판·지문·**스냅샷의** 규칙 판)에 원천 노드. 보조 근거가 빠지거나 바뀌면 **근거 해시가 달라지고**, 순서와 스냅샷 지문에는 흔들리지 않는다. 지문 없는 근거는 finding만 내고 참조를 지어내지 않는다 | 주 근거만 보면 보조 근거가 바뀌어도 판정 서명이 살아남는다. 스냅샷 지문은 위치이지 근거가 아니다. 말없이 빼면 상류가 알릴 기회가 없다 |
| **[IC-P11](../pkg/inventory/review/axes_test.go) ✅** | 리뷰 항목은 판정만 하고, **자동통과 항목만** 계획에 고름 | **실제 계획이 생긴다.** 자동통과에는 결론을 요구하지 않는다. 전에는 자동통과가 식별자 문자열 목록이라 계획 칸을 들 자리가 없었고, `Finalize`는 `Items`만 봐서 켜도 아무 알림 없이 빠졌다 | 자동통과는 「사람이 볼 필요가 없다」이지 「바꿀 필요가 없다」가 아니다. 확인된 자산이야말로 조치 대상이 되기 쉽다 |
| **[IC-P12](../pkg/inventory/review/axes_test.go) ✅** | `EXCLUDED_BY_POLICY` 항목을 세션 파일을 손으로 고쳐 계획에 넣음 | **확정이 막는다.** `Pending`도 그 항목의 계획 선택을 알린다(`plan-field`). 관리 축이 세션 파일에 있어야 이 검사가 가능하다 | 정책을 무시하던 동안 뺀 자산에 조치 계획을 세우던 결함으로 돌아가지 않는다 |
| **[IC-P13](../pkg/inventory/review/axes_test.go) ✅** | 옛 세션 파일(`autopass_candidates`가 문자열 배열) · 새 형식 · 모르는 형식 · 저장 뒤 다시 읽기 | 옛 목록은 **열리되 표시용으로만** 남고(`autopass_candidates_legacy`), 계획에 넣을 수 없다. 새 형식은 그대로 읽힌다. 모르는 형식은 **오류**다. 저장은 새 형식만 쓴다 | 형을 바로 바꾸면 `json.Unmarshal`이 형 오류로 먼저 실패해 변환 코드에 닿지 못한다. 빈 칸의 일반 항목으로 말없이 올리면 사람이 새 후보로 읽는다 |
| **[IC-P14](../pkg/inventory/review/axes_test.go) ✅** | 같은 자산이 자동통과에서 리뷰 항목으로(확신 하락) 옮겨 가며 지문도 바뀜 · 아무것도 안 바뀜 · 자동통과의 근거만 바뀜 · 옛 식별자 중 하나만 새 세션에 있음 | 계획 선택값(포함·종류·수준·목표·되돌림 메모)이 **따라온다.** 근거가 바뀌면 서명이 지워지고, 그대로면 남는다. **자동통과의 근거만 바뀌어도** 서명이 무효다. 옛 식별자는 같은 ID가 있으면 치환된 셈이고 없는 것만 남는다 | `sameBasis`는 자리(index)가 아니라 **ID**로 맞춘다. 항목이 컬렉션을 옮기면 길이도 자리도 어긋난다. `All`은 복사본이라 쓰는 자리는 `update`로 실제 컬렉션을 찾는다 |
| **[IC-P15](../pkg/inventory/review/axes_test.go) ✅** | v3 규칙 판에서 관리 판정·제외 근거·미평가 여부를 바꿈 · 같은 것을 v2 판에서 | **v3 해시는 셋 다 덮는다.** v2 해시는 그 칸을 보지 않는다 | 관리 근거만 덮으면 정책이 바뀌어 근거 구성이 달라져도 서명이 살아남는다. v2 세션의 원장 행·델타 비교와 어긋나면 안 된다 |
| **[IC-P16](../pkg/inventory/review/axes_test.go) ✅** | v2 세션의 빈 되돌림 메모 · v3 세션의 빈 되돌림 메모 · 적은 되돌림 메모 | v2는 결론으로 채워지고, **v3은 빈 값 그대로**, 적은 것은 그대로 나간다 | `rollback_note`는 계획 축의 값이다. 결론은 「어떻게 판정했나」이고 이것은 「어떻게 되돌리나」다. v3 사용자가 일부러 비운 자리에 결론이 다시 들어가면 사람의 선택을 덮는 것이다 |
| **[IC-P17](../pkg/inventory/review/axes_test.go) ✅** | 같은 ID가 리뷰 항목과 자동통과 양쪽에 있음 | **확정이 구조 오류로 중단한다**(판정 미완이 아니다) | 한 자산은 한 컬렉션에만 있다. 양쪽에 있으면 대조의 결함이고, 그대로 두면 `Carry`가 한쪽에만 쓰고 다른 쪽이 덮는다 |
| **[IC-P18](../pkg/inventory/review/axes_test.go) ✅** | v2 규칙 판의 세션을 그대로 확정 · 같은 세션을 v3으로 다시 열어 `Carry` | 확정은 **막히지 않고** 계획의 규칙 판은 세션의 것(v2)이며, 옛 규칙 판이라는 **경고**가 두 판을 값으로 적어 난다. 다시 열어 `Carry` 하면 서명은 지워진다 | 막으면 검토 중인 세션이 도구 교체로 버려져 사람이 한 일이 사라진다. 다만 그 근거 해시와 서명은 옛 규칙의 것이다 |
| **[IC-P19](../pkg/inventory/review/axes_test.go) ✅** | 상류 규칙 판만 다른 세션을 결론 · 서명까지 채워 확정 · 같은 세션을 지금 규칙 판으로 | 상류 판이 다르면 **계획을 내지 않고** `ErrRulesetChanged`로 거절하며, 거절문이 두 판을 값으로 적는다. 지금 판이면 확정된다 | 상류 규칙 판이 바뀌면 자산 ID와 스냅샷 지문이 달라져 옛 근거가 상류 이력에서 찾히지 않는다. 계획 규칙 판만 낮은 경우(IC-P18)와 달리 한 계획에 두 규칙 판이 섞인다 |
| **[IC-P20](../pkg/inventory/review/upstream_ruleset_test.go) ✅** | 상류 규칙 판이 다른 저장 세션을 결과에서 다시 세운 세션에 `Carry` · 계획 규칙 판만 다른 경우 | 상류 판이 다르면 **옮기지 않고 저장된 세션을 그대로** 돌려준다(규칙 판 · 세션 ID · 서명 그대로). 계획 규칙 판만 다르면 결론을 옮기고 서명을 지운다 | 옛 결론이 새 근거에 실리면 그 항목은 판정된 것이 되어 서명만으로 확정을 지나간다 |
| **[IC-P21](../pkg/inventory/review/installations_test.go) ✅** | 설치가 둘인 항목을 확인 없이 · 지금 목록으로 확인하고 · 확인 뒤 경로가 바뀐 채 · 「설치마다 다름」으로 확정 · 설치가 하나인 항목 | 확인이 지금 목록과 맞을 때만 확정되고 **두 설치가 모두 계획의 근거로 간다.** 나머지 셋은 계획을 내지 않는다. 설치가 하나면 확인이 필요 없다 | 확인 없이 지나가면 서로 다른 설치에 조치 하나가 말없이 간다. 이 판은 설치마다 다른 조치를 표현하지 못한다 |
| **[IC-P22](../pkg/inventory/review/installations_test.go) ✅** | 경로와 확인이 든 세션을 파일로 쓰고 읽은 뒤 다시 세운 세션에 `Carry` | 경로와 확인이 남고, 확인은 목록째 옮겨져 지금 목록과 맞는다 | 다시 열 때마다 확인이 사라지면 사람이 같은 확인을 되풀이하고, 경로가 사라지면 확인할 근거가 없다 |
| **[IC-P23](../pkg/inventory/review/installations_test.go) ✅** | 같은 경로를 원천 노드 둘이 봄 · 경로 없는 서로 다른 finding 둘을 확인 없이 · 확인하고 · 확인 뒤 한 finding id가 바뀜 | 같은 경로는 **설치 하나**라 확인 없이 확정되고, 원천 노드는 관측 출처로만 남는다. 경로 없는 둘은 설치 후보 둘이라 확인이 있어야 확정되며, finding id가 바뀌면 확인은 무효다 | 원천 노드는 관측 봉투의 이름이고 계획은 항목의 노드 하나로 나간다. 출처 둘을 설치 둘로 부르면 사실과 어긋나고 필요 없는 확인을 요구한다 |
| **[IC-P24](../pkg/inventory/review/ruleset_round_test.go) ✅** | 상류 규칙 판이 다른 판정 세션을 지금 규칙으로 다시 세운 세션에 넘김 · 서명과 검토자만 넣고 확정 | 새 세션은 새 ID에 서명 · 검토자가 비고, **옛 결론 · 계획 칸 · 정책 결론은 참고값(previous)에만 있고 현재 칸은 빈다.** 서명만으로는 확정되지 않는다 | 옛 결론이 현재 값이 되면 그 항목은 판정된 것이라 서명만으로 다른 규칙의 근거를 확정한다 |
| **[IC-P25](../pkg/inventory/review/ruleset_round_test.go) ✅** | 참고값이 있는 항목과 없는 항목을 다시 채택 · 정책 결론을 다시 채택 · 다시 세워 `Carry` · 같은 입력으로 보고서를 두 번 | 채택은 **그 항목 하나만** 옮기고 다설치 확인은 옮기지 않는다. 참고값과 전환 기록은 다시 세워도 남는다. 보고서는 같은 바이트이고, 사라진 항목 · 새 항목 · 설치가 갈린 항목과 맨 위 문장을 적는다 | 한꺼번에 옮기면 자동 이월이다. 보고서가 흔들리면 멈춘 뒤 복구한 결과와 처음 결과를 견줄 수 없다 |
| **[IC-P26](../pkg/inventory/rulesetround/rulesetround_test.go) ✅** | 공통 규칙 전환 함수로 끊김 없이 회차를 엶 · 적용 도중과 끝에서 멈춘 뒤 복구 · 같은 요청을 다시 · 이미 지금 판인 세션 | 멈췄다 복구한 새 세션 · 보고서 · 보관본이 **끊김 없이 연 것과 바이트까지 같다.** 다시 보내면 거절되고 파일이 늘지 않는다. 이름에 규칙 판 문자열이 없다 | 화면과 명령이 다른 함수를 쓰거나 복구 결과가 달라지면, 같은 전환이 어느 길로 갔느냐에 따라 다른 기록이 남는다 |
| **[IC-P27](../pkg/inventory/rulesetround/rulesetround_test.go) ✅** | 옛 세션 · 선언 · 결과 폴더 · 자산 스코프 정책의 지문을 재어 넘기고, 다시 세우는 도중에 하나씩 바꿈 | 넷 모두 **ErrStale이고 보관본 · 새 세션 · 보고서 · 회차 표지가 하나도 생기지 않는다** | 서로 다른 시점의 입력으로 새 세션과 보고서가 만들어지면 무엇을 보고 넘겼는지 되짚지 못한다 |
| **[IC-P28](../pkg/inventory/rulesetround/rulesetround_test.go) ✅** | 같은 항목 ID에 요청한 조직이 다름 · 다시 세운 세션만 다른 조직 | 둘 다 ErrOrgMismatch이고 파일이 생기지 않는다 | 다른 조직의 결론이 참고값으로라도 넘어가면 조직의 경계가 무너진다 |
| **[IC-P29](../pkg/inventory/rulesetround/rulesetround_test.go) ✅** | 세션 ID가 없는 옛 파일 둘을 한 상태 폴더에서 차례로 전환 | 둘 다 회차가 열린다 | 내부 이름이 하나(`unnamed-session`)면 첫 회차의 표지가 둘째를 막는다 |
| **[IC-P30](../pkg/inventory/review/plan_ruleset_test.go) ✅** | 지금 판의 계획 · 관측 근거가 없는 조치 · 최상위가 옛 판 · 근거 하나만 옛 판 · 근거에 스냅샷이 없음 · 근거의 규칙 판이 빔 · finding은 있는데 근거가 없음 | 앞의 둘은 지나가고, 나머지는 사유(최상위 · 근거 · 되짚을 수 없음)를 갈라 **거절한다** | 최상위만 보면 근거 하나만 옛 판인 섞인 계획이 서명된다. 되짚을 수 없는 근거로 만든 플레이북은 근거를 대지 못한다 |
| **[IC-P34](../inventory/cmd/pqcaton-ui/plan_scope_test.go) ✅** | 계획을 만든 정책과 지금 활성 정책이 다름 | 06에 서명 양식이 없고 그 계획의 문제를 적는다. 서명 확인은 409 `plan-scope`이고 작업 폴더의 어떤 파일도 바뀌지 않는다 |
| **[IC-P35](../inventory/cmd/pqcaton-ui/plan_scope_test.go) ✅** | 형식 1 영수증(이 판보다 먼저 확정)의 계획 · 활성 정책 없음 | 「확인할 수 없음」으로 거절한다. **없는 지문을 빈 정책의 지문으로 읽으면 통과하는 자리**에서 잰다 |
| [IC-P36](../inventory/cmd/pqcaton-ui/plan_scope_test.go) ✅ | 계획 ID가 옛 모양 · 세션이 빠짐 · 조직에 쓸 수 없는 글자 | 「계획 ID 형식」에서 거절한다. 영수증을 찾지 않는다 |
| [IC-P37](../inventory/cmd/pqcaton-ui/plan_scope_test.go) ✅ | 계획 ID의 조직이 이 작업 폴더의 조직이 아님 | 「동일성」에서 거절한다 |
| **[IC-P38](../inventory/cmd/pqcaton-ui/plan_scope_test.go) ✅** | 그 세션의 영수증이 없음(다른 작업 폴더에서 옮겨 온 계획) | 「영수증 없음」으로 거절한다. 옮긴 계획은 상류 `pqcota-approve` · `pqcota-provision`으로 다룬다 |
| **[IC-P39](../inventory/cmd/pqcaton-ui/plan_scope_test.go) ✅** | 확정한 뒤 계획 파일이 바뀜 | 「계획 파일 해시」에서 거절한다 |
| **[IC-P40](../pkg/inventory/review/scope_policy_test.go) ✅** | 저장된 세션과 다시 세운 세션의 정책 지문: 같음 · note만 바뀜 · 저장된 쪽에 없음 · 다름 | 앞의 둘은 결론을 옮긴다. 뒤의 둘은 **옮기지 않고 저장된 세션을 그대로 돌려준다**(규칙 판과 같은 기준) |
| **[IC-P41](../pkg/inventory/rulesetround/baseline_test.go) ✅** | 지문 없는 옛 세션에서 기준선 회차 · 같은 옛 세션에서 다시 | 새 세션은 지금 정책의 지문과 기준선 기록을 들고, 남은 항목은 옛 결론이 참고값으로만 붙으며 현재 결론은 빈다. 새 정책에서 빠진 항목은 보고서에만 남는다. 옛 세션은 바이트 그대로 보관되고, scope-baseline 형식 2 영수증이 조직 · 옛 세션 해시 · 정책 지문 · 새 세션 ID를 잇는다. 다시 열면 거절되고 파일이 늘지 않는다 |
| [IC-P42](../pkg/inventory/rulesetround/baseline_test.go) ✅ | 지문이 이미 있는 세션에서 기준선 · 확인한 정책과 다시 세운 세션의 정책이 다름 | `ErrNotBaseline` · `ErrStale`이고 아무것도 쓰지 않는다 |
| **[IC-P43](../pkg/inventory/rulesetround/baseline_test.go) ✅** | 지문 없는 세션에서 규칙 전환 · 다시 세운 세션의 지문이 다른 규칙 전환 | 기준선 잠금 · 승인되지 않은 변경으로 거절한다. **규칙 전환이 정책을 운영자 확인 없이 채택하지 않는다** |
| **[IC-P44](../pkg/inventory/rulesetround/activation_test.go) ✅** | 정책 A로 판정한 세션 · 활성 정책 A · 02 확정 정책 B로 활성화 · 같은 요청 다시 | 활성 정책이 B의 바이트로 바뀌고 A는 보관된다. 새 세션은 B의 지문과 활성화 기록을 들고 옛 결론은 참고값으로만, 빠진 항목은 보고서에만 남는다. scope-activation 형식 2 영수증이 조직 · 옛 세션 해시 · 옛 지문 · 새 지문 · 새 세션 ID를 잇는다. 다시 보내면 거절되고 파일이 늘지 않는다 |
| **[IC-P45](../pkg/inventory/rulesetround/activation_test.go) ✅** | 두 정책이 의미상 같음 · 후보 없음 · 후보가 깨짐 · 활성 정책 파일 없음 | 같으면 `ErrSamePolicy`, 후보가 없거나 깨지면 `ErrNoCandidate`이고 아무것도 쓰지 않는다. 활성 정책 파일이 없으면 규칙 0개 정책에서 활성화하고 보관할 옛 정책은 없다 |
| **[IC-P46](../pkg/inventory/rulesetround/activation_test.go) ✅** | 활성화 영수증을 같은 것 · 다른 조직 · 다른 세션 · 다른 세션 바이트 · 다른 옛 지문 · 다른 새 지문으로 증거 판정 | **정확히 이을 때만 증거다.** 나머지는 모두 아니다. 영수증이 없으면 아니다 |
| **[IC-P47](../pkg/inventory/review/ruleset_round_test.go) ✅** | 정책 회차로 연 세션을 지금 입력에서 다시 세움 | **정책 회차 기록(근거 포함)이 그대로 남는다.** 빠지면 05가 그 회차를 운영자 진술로 열었는지 보이지 못하고, 화면이 세션을 다시 쓰면 기록이 사라진다 |
| **[IC-P48](../pkg/inventory/review/plan_fields_test.go) ✅** | 목표 알고리즘 · 근거 관측 · 활성화 훅 · 설정 산출물 · 되돌림 메모에 NUL을 넣어 계약으로 · NUL 없음 | **계약을 만들지 않고 상류 `sign.CheckPlanText`의 ErrNUL로 거절한다.** 확정(화면 · 명령의 close)이 이 함수를 거치므로 그런 계획은 파일과 영수증이 되지 않는다. NUL이 없으면 그대로다 |
| **[IC-P49](../pkg/inventory/rulesetround/activation_test.go) ✅** | 대조 기록 없이 활성화 · 대조 기록 없이 기준선 · 대조 기록과 함께 활성화 | **정책 회차는 pqcota 이력 대조 기록 없이 열리지 않는다.** 앞의 둘은 `ErrNoUpstreamCheck`이고 아무것도 쓰지 않는다. 마지막은 처리 기록의 basis가 upstream-verified이고 전환 보고서에 `upstream_check`가 실리며, 보고서 형식 판은 1 그대로라 v0.27.x의 해석이 읽는다 |
| **[IC-P50](../pkg/inventory/review/ruleset_round_test.go) ✅** | 형식 판 1 보고서: upstream_check 없음(v0.27.x 이하) · 있음 · 이 판이 모르는 선택 필드까지 · 다른 형식 판 | **형식 판 1은 모르는 필드를 무시하는 계약이다.** 앞의 셋은 읽히고 아는 필드는 그대로이며, upstream_check는 없으면 nil이고 있으면 다시 써도 남는다. 다른 형식 판은 거절한다 |

### E. 통신 엣지 대조와 토폴로지 ([인벤토리 설계 §6](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/inventory/design.md)) 🔶: 엔진·렌더·저장 완료(unit); 라이브 관측은 network-collector([디스커버리 설계 §2.3](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/discovery/design.md))가 공급
| TC | Given → When | Then |
|---|---|---|
| [IC-E1](../pkg/inventory/reconcile/edge_test.go) ✅ | 관측 엣지(TLS/SSH 협상) vs 선언 엣지 | 엣지 3-상태(CONFIRMED/UNDECLARED/UNOBSERVED) + 등급 부착 |
| [IC-E2](../pkg/inventory/reconcile/topology_test.go) ✅ | 토폴로지 렌더 | 색=등급(🟢PQC/🔴취약/⚪불명), 미관측=점선(≠부재, [인벤토리 설계 §6.2](https://github.com/randyinthedev-hash/pqcota/blob/v0.9.1/inventory/design.md) 정직성) |
| [IC-E3](../pkg/inventory/reconcile/edge_test.go) ✅ | 스코프 밖 관측 상대 | off-scope 표기 "등재 판정 요청"(§1.4/§5) |
| [IC-E4](../pkg/inventory/reconcile/edge_test.go) ✅ | 관측 엣지가 들어오는 순서만 다르다 · 출발지가 같은 엣지가 여럿이다 | 결과의 순서가 같다. 출발지 → 도착지 → 포트 → 프로토콜 순이고 선언만 있는 엣지도 한 줄로 섞인다(보고서와 토폴로지 SVG의 엣지 번호가 실행마다 달라지지 않는다) |

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
| **[IC-X8](../tools/checklicenses/main_test.go) ✅** | 리포에 놓인 `.js`·`.css`·`.mjs` | 확장자로 훑고 `.git`·`node_modules`·`testdata`는 뺀다. 배포물이 아니다. **디렉터리 규약을 믿지 않는 이유**는 규약을 어긴 사람이 아니라 모르고 다른 곳에 놓은 사람이 지나가기 때문이다 |
| **[IC-X9](../tools/checklicenses/main_test.go) ✅** | `licenses.txt`에 적히지 않은 웹 자산 | 모듈과 똑같이 막는다. 관문을 넓힌 이유가 여기다. 예전에는 프런트 라이브러리를 받아 넣어도 초록이었다 |
| [IC-X10](../tools/checklicenses/main_test.go) ✅ | 카피레프트 `.js` | 막는다. 프런트에는 GPL 라이브러리가 흔하다 |
| **[IC-X11](../tools/checklicenses/main_test.go) ✅** | 실제로 싣고 있는 htmx | `licenses.txt` 값이 옆의 LICENSE 원문과 맞는다. **파일만 갈아 끼우고 목록을 안 고치는 날**을 잡는다 |

### U. 리뷰 화면 (`pqcaton-ui`) ✅
| TC | Given → When | Then |
|---|---|---|
| [IC-U1](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 세션을 화면으로 | **정책으로 묶어** 보여 준다. 한 줄씩 늘어놓으면 화면이 있어도 수천 대에서 리뷰가 안 끝난다 |
| [IC-U2](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 「저장만」 | 세션 파일에 남되 **확정 계획은 생기지 않는다**. 채우다 만 것을 확정으로 오해하면 감사 기록이 거짓이 된다 |
| **[IC-U3](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 서명 없이 「확정」 | **막고 무엇이 남았는지 화면에 그대로 보인다**. `close`와 같은 관문(`review.Finalize`)를 탄다 |
| [IC-U4](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 필수 결론·서명을 채우고 확정 | 계약 형식 계획이 파일로 나오고 판정이 조직에 묶여 원장에 남는다 |
| **[IC-U5](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 화면으로 채운 세션 파일 | **명령이 그대로 읽고 확정한다**. 기계가 채운 `node`·`runtime`이 왕복에서 사라지지 않는다 |
| [IC-U6](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 바인드 주소 | 루프백을 가려낸다. 루프백이 아니면 화면을 열지 않는다(IC-U54) |
| [IC-U7](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `/save`·`/finalize`에 GET | 405를 낸다. 새로고침으로 확정이 다시 돌지 않게 POST 뒤엔 리다이렉트한다 |
| **[IC-U8](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | `pqcaton-ui session.json -addr ...` | 위치 인자 뒤의 플래그도 먹는다. 표준 flag는 거기서 멈춰 **-addr이 아무 표시 없이 무시된다** |
| [IC-U9](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `-decl` 없이 / 주고 | 없으면 이동 링크도 `/decl`도 없다(404). **없는 것을 눌러 보게 하지 않는다** |
| **[IC-U10](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 선언 화면을 염 | 설명이 **접혀 있다**. 날마다 쓰는 화면이지 읽는 화면이 아니다. 필요한 사람만 「도움말」을 편다 |
| [IC-U11](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 화면에서 선언 저장 | 명령이 그대로 읽고, 다 맞추면 짚을 것이 없다 |
| [IC-U12](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 노드 이름을 비워 저장 | 그 줄이 지워진다. 표에서 줄을 없애는 유일한 방법이다 |
| **[IC-U13](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 탭 순서 | **선언 → 대조 → 판정** 순이다. 절차 순서다. 뒤섞이면 화면이 절차를 가르치지 못한다 |
| **[IC-U24](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 판정 화면의 단계 카드와 조회 탭 | 화면이 쓰는 말과 이어진다. 판정을 **하는** 자리는 「판정(리뷰 큐)」, **보는** 자리는 「인벤토리·판정 이력」이다 |
| [IC-U14](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `/`로 들어옴 | 절차의 첫 자리로 보낸다(선언이 있으면 선언, 없으면 리뷰 큐) |
| [IC-U15](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `-results` 없이 | 대조 탭도 `/survey`도 없다(404) |
| **[IC-U16](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 네 화면을 모두 준 상태의 탭 순서 | **선언 → 스코프 → 대조 → 판정** 순이다. 무엇을 계속 볼지가 정해져야 관측이 적재되므로 스코프가 대조 앞이다 |
| **[IC-U17](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 근거 없이 스코프 확정 | 막고 어느 변경이 막았는지 알려 준다. `pqcaton-scope close`와 같은 관문 |
| [IC-U18](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | 승인 뒤 나온 CSV | **pqcota가 그대로 읽고** 판정이 조직에 묶여 남는다 |
| **[IC-U19](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 「행 추가」에 모르는 표 이름 · 범위 밖 번호 | 막는다(400). 그리면 화면에는 줄이 생기는데 `ApplyDecl`이 읽지 못하는 자리에 놓인다. 사람은 적어 넣고 저장했는데 **아무 일도 일어나지 않는다** |
| [IC-U20](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `/static/htmx.min.js` · `/static/app.css` | 같은 서버가 내준다. 주소가 어긋나면 화면은 뜨는데 모양이 무너지고 「행 추가」가 아무 표시 없이 동작하지 않는다 |
| **[IC-U21](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 계층 CSV만 주고 **세션 파일 없음** | 화면이 세션을 연다. 입력 파일을 이미 가지고도 명령을 한 번 돌려야 열리는 것은 화면을 두는 이유와 어긋난다 |
| **[IC-U22](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 화면에서 고친 규칙 | **계층 CSV에 그대로 쓰이고 pqcota가 읽는다.** 새 exclude가 판정 대상으로 올라온다. 빈 줄은 규칙이 되지 않는다 |
| **[IC-U23](../inventory/cmd/pqcaton-ui/main_test.go) ✅** | 선언과 관측 결과만 주고 **세션 파일 없음** | 리뷰 큐가 열린다 |
| [IC-U25](../inventory/cmd/pqcaton-ui/main_test.go) ✅ | `/ui-next.html` | **404다**(v0.22.0). 새 화면이 기본 주소가 됐으므로 견줄 정적 원본을 바이너리에 싣지 않는다. 공개 사이트의 원본(`site/ui-next.html`)은 그대로 둔다 |

#### 선언 화면의 다음 판 (`/decl-next`)

지금 선언 화면은 칸부터 펼친다. 칸이 많아서 처음 여는 사람은 어디부터 손대야 하는지 알기
어렵고, 정작 급한 것 하나가 그 안에 묻힌다. 급한 것은 **관측에는 있는데 어느 노드에도 붙지
않은 이름**이다. 그 자리를 두면 대조가 통째로 어긋난다.

그래서 **적는 자리 위에 「지금 무엇을 할지」를 얹는다.** 옮기는 동안 옛 화면(`/decl`)을
그대로 두고 새 주소를 하나 더 연다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-U26](../pkg/inventory/ui/ui_test.go) ✅** | 새 선언 화면(`/decl`, v0.21까지 `/decl-next`)을 연다 | 요약과 **편집 폼이 한 화면에** 있다. 폼이 빠지면 무엇을 할지 가리키기만 하고 고칠 자리가 없는 화면이 된다 |
| **[IC-U27](../pkg/inventory/ui/ui_test.go) ✅** | 붙지 않은 관측 이름이 없을 때 · 있을 때 | 없으면 **없다고 적고**, 있으면 그 이름을 그대로 보여 준다. 빈 자리는 「아직 안 불러온 것」으로 읽히고, 이름을 감추면 무엇을 고쳐야 할지 알 수 없다 |
| **[IC-U28](../pkg/inventory/ui/ui_test.go) ✅** | 노드 둘·자산 둘·안 붙은 이름 하나 | 뷰가 센다(`DeclView.Summary`). templ 안에서 세면 같은 계산이 화면마다 흩어지고, 그러면 화면과 요약이 다른 답을 내는 날이 온다 |
| **[IC-U29](../pkg/inventory/ui/ui_test.go) ✅** | 관측이 붙은 노드 · 후보가 남은 노드 · 둘 다 없는 노드 | 배지가 셋으로 나뉜다. 배지가 상태를 잘못 말하면 사람은 **멀쩡한 노드를 고치러 간다** |
| **[IC-U32](../pkg/inventory/ui/ui_test.go) ✅** | 노드가 여럿인 선언 | 카드는 **그 노드의 줄로** 보내고, 「연결 검토」는 붙일 데가 있는 첫 노드로 간다. 폼 머리로 보내면 어느 줄을 고쳐야 하는지 다시 찾아야 하고, 그러면 요약이 가리킨 뜻이 사라진다 |
| **[IC-U33](../pkg/inventory/ui/ui_test.go) ✅** | 절차 카드 다섯 | 카드마다 **지금 상태가 한 줄로** 적힌다. 번호와 이름만 있는 탭으로는 어디부터 볼지 알 수 없다. 아직 세지 않는 화면은 그렇다고 적는다 |
| [IC-U34](../pkg/inventory/ui/ui_test.go) ✅ | 입력 파일을 주지 않은 화면 | 카드를 **빼지 않고 닫는다.** 넷이 늘 보여야 절차가 몇 걸음인지 읽히고, 닫힌 카드는 누를 수 없다(`NavFor`와 같은 선) |
| **[IC-U36](../pkg/inventory/ui/ui_test.go) ✅** | 근거가 있는 변경 둘과 없는 변경 하나 | 뷰가 센다. **근거가 필요한 수를 따로 센다.** 다 승인해 놓고 확정이 막히면 사람은 무엇이 모자란지 화면에서 알 수 없다 |
| **[IC-U38](../pkg/inventory/ui/ui_test.go) ✅** | 세 상태와 재관측 후보가 섞인 결과 | 상태를 그대로 세고 **재관측 후보는 따로 센다.** 확정된 것은 기계가 답을 냈으므로 「사람이 볼 항목」에서 뺀다 |
| **[IC-U39](../pkg/inventory/ui/ui_test.go) ✅** | 재관측 후보가 있을 때 · 없을 때 | 있으면 **판정 안내보다 먼저** 띄운다. 못 본 것을 없는 것으로 확정하면 실재하는 자산이 장부에서 사라진다. 없으면 그 경고를 띄우지 않는다. 늘 뜨는 경고는 아무도 안 읽는다 |
| **[IC-U41](../pkg/inventory/ui/ui_test.go) ✅** | 결론이 빈 정책 하나가 필수 항목 둘을 안고 있음 | 남은 것을 **정책 수가 아니라 필수 항목 수**로 센다. 서명은 승인자와 서명이 **둘 다** 차야 채운 것이다 |
| **[IC-U42](../pkg/inventory/ui/ui_test.go) ✅** | 결론과 서명이 모자랄 때 · 서명만 없을 때 · 둘 다 찼을 때 | 셋을 갈라 적는다. 뭉뚱그리면 사람은 **결론을 다 채워 놓고 왜 여전히 막히는지 모른다** |
| **[IC-U44](../pkg/inventory/ui/ui_test.go) ✅** | 승인이 살아 있는 제외와 만료된 제외 | **살아 있는 승인은 빼고** 센다. 전부 세면 뺄 때마다 재검토 수가 늘어 아무도 그 숫자를 안 본다 |
| [IC-U45](../pkg/inventory/ui/ui_test.go) ✅ | 걸리는 것이 없을 때 · 있을 때 | 없으면 **없다고 적는다.** 이 화면은 아무 때나 들어오는 자리라, 빈 자리를 두면 「아직 안 불러온 것」으로 읽힌다 |
| **[IC-U46](../pkg/inventory/ui/ui_test.go) ✅** | 조회 화면에서 절차 카드를 셈 | 넷뿐이다. 조회는 단계가 아니라 아무 때나 들어오는 자리라, 번호를 달면 ④ 다음에 하는 일로 읽힌다 |
| **[IC-U47](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | 루프백 이름(`127.0.0.1` · `localhost` · `[::1]`, 포트는 아무것)으로 온 읽기 요청 | 지나간다. **포트는 보지 않는다.** SSH로 다른 포트에 넘겨 들어오면 Host의 포트가 바뀌는데, 그때 막히면 원격에서 쓰는 길이 막힌다 |
| **[IC-U48](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | 다른 이름(`evil.example`, LAN 주소, 빈 Host)으로 온 GET과 POST | **둘 다 막는다**(403). DNS rebinding을 막는 자리라 읽기 요청도 막아야 리뷰 큐가 읽히지 않는다. 막힌 요청에는 쿠키를 내주지 않는다 |
| **[IC-U49](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | 같은 출처에서 같은 토큰을 들고 온 POST | 여섯 주소(선언 저장 · 규칙 저장 · 스코프 저장 · 스코프 확정 · 판정 저장 · 판정 확정) 모두 처리기에 닿는다. 막는 것만 재면 전부 막아도 통과한다 |
| **[IC-U50](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | 다른 이름 · 다른 포트 · https · 없음 · `null` · 빈 값 · 둘 · 경로나 사용자 정보나 질의가 붙은 Origin | 모두 막고 처리기에 닿지 않는다. 같은 기계의 다른 포트도 다른 출처다. 쿠키와 SameSite는 포트를 가르지 않으므로 그 몫을 Origin이 한다. **대소문자만 다른 호스트 이름은 같은 출처로 받는다.** 브라우저는 Origin을 소문자로 보내므로, 막으면 `LOCALHOST`로 연 화면에서 저장이 늘 막힌다 |
| **[IC-U51](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | 쿠키 없음 · 모양이 틀린 쿠키 · 필드 없음 · 빈 필드 · 다른 토큰 · 한 글자 바뀐 토큰 · 필드 둘 | 모두 막고 처리기에 닿지 않는다. 빈 값끼리 같다고 지나가면 토큰을 채우지 않은 실수가 그대로 통과한다 |
| **[IC-U52](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | 라우터에 등록된 POST 전부 | 토큰 없이 보내면 모두 막힌다. 보호 목록을 사람이 따로 적으면 새 주소를 더한 날 넣는 것을 잊는다. 그래서 라우터를 훑어 모은다 |
| **[IC-U53](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | 막힌 요청(Host · Origin · 쿠키 · 토큰) | **아무것도 바꾸지 않는다.** 가짜 처리기는 한 번도 불리지 않고, 실제 일곱 주소에서는 막힌 뒤 파일이 한 바이트도 바뀌지 않는다. 403을 냈는데 처리기가 먼저 돌았다면 막은 것이 아니다 |
| **[IC-U54](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | `-addr`가 루프백이 아님(`:8765` · `0.0.0.0` · `[::]` · LAN 주소 · 이름) | **화면을 열지 않는다.** 이 화면에는 인증이 없다. 거부하면서 다른 기계에서 쓰는 길(SSH 포트 포워딩)을 함께 알린다 |
| **[IC-U55](../inventory/cmd/pqcaton-ui/guard_test.go) ✅** | 화면 · 새 화면 · 조각 · 막힌 요청 · 정적 CSS와 JS의 응답 | 보안 머리글(프레임 금지 · 형식 추측 금지 · 리퍼러는 같은 출처에만)이 모두에 붙는다. 리퍼러를 아예 막으면(`no-referrer`) 브라우저가 양식 POST에 `Origin: null`을 실어 저장이 모두 막힌다(v0.23.0에서 고침). 저장 금지(`no-store`)는 정적 파일에만 빠진다. 화면과 오류에는 리뷰 큐와 토큰이 담긴다 |
| **[IC-U56](../inventory/cmd/pqcaton-ui/save_test.go) ✅** | 화면을 연 뒤에 파일이 바뀐 상태에서 리뷰 · 스코프 · 선언 · 접근 정보 저장 | **409이고 쓰지 않는다.** rev는 저장소가 저장 잠금 안에서 견준다(IC-W30). 거절 페이지는 역할과 파일 이름만 보이고 전체 경로와 오류 원문은 싣지 않는다 |
| [IC-U57](../inventory/cmd/pqcaton-ui/save_test.go) ✅ | 확정된 세션을 고쳐 저장 · 다른 내용으로 다시 확정 | 409이고 새 세션을 열라고 알린다. 계획과 세션은 바뀌지 않는다 |
| **[IC-U58](../inventory/cmd/pqcaton-ui/save_test.go) ✅** | 같은 확정을 다시 누름 | 「이미 확정됨」으로 화면에 돌아간다. 판정 원장에는 한 번만 쌓이고, 확정 전의 계획은 `plan.json.prev`로 남는다. 알림에는 파일 이름만 적는다 |
| [IC-U59](../inventory/cmd/pqcaton-ui/save_test.go) ✅ | 저장을 지원하지 않는 플랫폼(보기 전용) | 화면은 열리고 저장은 503이다 |
| [IC-U60](../inventory/cmd/pqcaton-ui/save_test.go) ✅ | 다른 저장이 대상 디렉토리를 쥐고 있음 | 정해진 시간까지 기다리다 409이고 쓰지 않는다 |
| [IC-U61](../inventory/cmd/pqcaton-ui/save_test.go) ✅ | 저장 기록과 파일이 어긋남 | 503이고 운영자 안내서의 「복구」 절을 안내한다(v0.23.0까지는 `-recover-status`를 적었다). 밖에서 바뀐 파일을 덮어쓰지 않는다 |
| **[IC-U62](../inventory/cmd/pqcaton-ui/save_test.go) ✅** | 같은 상태 디렉토리로 서버 둘 · 기록에 없는 준비 파일 · 계층만 주고 시작 | 둘째 서버는 열리지 않는다. 기록에 없는 준비 파일은 지우지 않고 열지 않는다. 입력에서 세운 세션은 시작할 때 파일로 남겨 세션 ID가 선다 |
| [IC-U63](../inventory/cmd/pqcaton-ui/save_test.go) ✅ | 리뷰 · 스코프 · 선언 화면 | 양식이 지금 파일로 잰 rev를 숨은 필드로 싣는다. 세션이 바뀌면 rev도 바뀐다 |
| **[IC-U64](../inventory/cmd/pqcaton-ui/confirm_test.go) ✅** | 확정 버튼(확인 요청) | **아무것도 바꾸지 않는다.** 확인 페이지는 무엇이 확정되는지(조치 수 · 판정 수 · 역할과 파일 이름)를 보이고, 저장하지 않은 편집과 서명된 확인 토큰을 숨은 필드로 싣는다. 확정 전에 따로 저장할 필요가 없다 |
| **[IC-U65](../inventory/cmd/pqcaton-ui/confirm_test.go) ✅** | 확인 토큰 없음 · 토큰 · 지문 · rev · 세션 · 만료 시각 가운데 하나를 바꿈 | 403이고 아무것도 쓰지 않는다. 서명은 확정 종류 · 세션 ID · rev · 전체 payload 지문 · 만료 시각을 덮는다 |
| [IC-U66](../inventory/cmd/pqcaton-ui/confirm_test.go) ✅ | 확인 뒤 10분이 지남 · 서버를 다시 띄움 | 만료는 409이고 다시 미리 보라고 안내한다. 다시 띄우면 서명 키가 바뀌어 403이다 |
| **[IC-U67](../inventory/cmd/pqcaton-ui/confirm_test.go) ✅** | 확인한 뒤 파일이 바뀜 | 409이고 쓰지 않는다. 토큰이 맞아도 저장소가 잠금 안에서 rev와 지문을 다시 견준다 |
| [IC-U68](../inventory/cmd/pqcaton-ui/confirm_test.go) ✅ | 확인 페이지에서 「저장만」 | 확정하지 않고 실어 온 편집을 저장한다. 계획과 영수증은 생기지 않는다 |
| [IC-U69](../inventory/cmd/pqcaton-ui/confirm_test.go) ✅ | 오래된 화면에서 리뷰 · 스코프 확정 버튼 | 409이고 확인 페이지를 그리지 않는다(확인 토큰을 내주지 않는다) |
| **[IC-U70](../inventory/cmd/pqcaton-ui/startup_test.go) ✅** | 복구 필요가 남은 채로 화면을 시작 | **일반 화면도 503이다.** 정적 파일만 나가고 화면 · 조각 · 저장은 모두 503 복구 페이지로 가며 운영자 안내서의 「복구」 절을 안내한다. 실행 잠금은 쥔 채라 명령도 막힌다 |
| **[IC-U71](../inventory/cmd/pqcaton-ui/startup_test.go) ✅** | 입력에서 세운 세션의 준비 파일을 쓰는 단계에서 실패 | 서버가 뜨지 않는다. 그대로 열면 읽을 때마다 세션 ID가 새로 난다. 실행 잠금을 놓고 세션 파일도 준비 파일도 남기지 않는다 |
| **[IC-U72](../inventory/cmd/pqcaton-ui/snapshot_test.go) ✅** | 화면 모델을 만든 직후에 다른 탭이나 손 편집이 세션을 바꿈 | **화면에 그린 내용과 양식의 rev는 같은 시점이다.** 모델을 만들기 전과 뒤의 rev가 같을 때만 짝짓고, 다르면 다시 만든다. 확인 페이지도 같아서, 미리 보는 사이에 바뀌면 폼의 rev와 맞지 않아 409다 |
| **[IC-U73](../inventory/cmd/pqcaton-ui/policy_test.go) ✅** | 서버가 떠 있는 동안 자산 스코프 정책(`-scope-assets`)을 바꿈 | **화면 모델 · rev · 확인 결과가 모두 새 정책을 쓴다.** 정책은 시작할 때 읽을 수 있는지만 보고, 쓸 때마다 읽는다. 선언 화면의 rev도 정책 파일을 덮는다 |

#### 화면 한 벌 (v0.22.0 · v0.23.0)

v0.22.0에서 새 화면이 기본 주소가 됐고, 옛 화면은 한 판 동안 `/classic` 아래에 남았다가 v0.23.0에서
없어졌다. v0.21의 `-next` 주소와 v0.22의 `/classic/*` 주소는 북마크가 죽지 않게 301로 넘긴다.

v0.23.0에서 옛 화면을 없애며 폐기한 케이스다. 번호는 다시 쓰지 않는다.

- IC-U30: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 IC-U74
- IC-U31: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 IC-U76
- IC-U35: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 IC-U74
- IC-U37: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 없음(화면이 한 벌이라 견줄 짝이 없다)
- IC-U40: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 없음(화면이 한 벌이라 견줄 짝이 없다)
- IC-U43: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 없음(화면이 한 벌이라 견줄 짝이 없다)
- IC-U75: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 IC-U76
- IC-U79: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 IC-U103
- IC-U80: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 IC-U103
- IC-U81: v0.23.0에서 옛 화면 제거로 폐기, 대체 케이스 IC-U103

| TC | Given → When | Then |
|---|---|---|
| **[IC-U74](../inventory/cmd/pqcaton-ui/screens_test.go) ✅** | 기본 주소 여섯(`/decl` `/scope` `/observe` `/survey` `/review` `/inventory`)을 연다 | 화면 껍데기이고 브라우저 제목은 그 화면의 이름이다. 옛 이동 링크도, 옛 화면으로 가는 링크도 없다 |
| **[IC-U76](../inventory/cmd/pqcaton-ui/screens_test.go) ✅** | 옛 주소 열 개(`-next` 다섯 · `/classic/*` 다섯)를 질의 문자열과 함께 거듭 연다 | 301로 기본 주소에 넘기고 **질의 문자열을 그대로 붙인다.** `?lang=en`으로 저장한 북마크가 말을 잃지 않고, 거듭 받아도 질의가 쌓이지 않는다. 옛 화면 자체는 없다 |
| [IC-U77](../inventory/cmd/pqcaton-ui/screens_test.go) ✅ | 확인 페이지 · 409 거절 · 복구 503 | 셋 다 새 껍데기다. 복구 503은 입력을 읽지 않으므로 단계 카드를 그리지 않는다 |
| **[IC-U78](../inventory/cmd/pqcaton-ui/screens_test.go) ✅** | 선언의 「다시 불러오기」 · 조회의 검색 양식 | 그 화면의 기본 주소를 가리킨다 |
| **[IC-U103](../inventory/cmd/pqcaton-ui/return_test.go) ✅** | 선언 · 스코프 · 판정에서 저장 · 확정 · 오래된 확인, 손으로 실어 보낸 `return_to`(바깥 주소 · 옛 주소 · 다른 화면) | 양식은 CSRF · rev만 싣고 `return_to`를 싣지 않는다. 저장 · 확정 뒤에는 그 화면으로 돌아오고, 실려 온 `return_to`는 무시한다. 확인 페이지와 거절 페이지의 「돌아가기」도 그 화면을 가리킨다 |
| **[IC-U82](../inventory/cmd/pqcaton-ui/cards_test.go) ✅** | 화면 여섯을 차례로 연다 | **단계 카드 다섯이 모두 채워지고 어느 화면에서 보든 같다.** 요청 하나가 입력을 한 번 읽어 본문과 카드가 함께 쓴다. 스코프와 판정 카드에는 확정 상태(미확정)가 붙는다 |
| **[IC-U83](../inventory/cmd/pqcaton-ui/cards_test.go) ✅** | 확정 전 · 확정 뒤 · 다른 요청이 잠금을 쥠 · 계획을 손으로 고침 | 판정 카드가 미확정 · 확정됨 · 확인할 수 없음 · 복구 필요다. 잠금 대기를 넘어도 화면은 선다 |
| [IC-U84](../inventory/cmd/pqcaton-ui/cards_test.go) ✅ | 판정 화면을 여는데 스코프 세션이 깨짐 | 스코프 카드만 「확인할 수 없음」이고 화면과 다른 카드는 그대로다 |
| **[IC-U85](../inventory/cmd/pqcaton-ui/cards_test.go) ✅** | 모델을 만든 직후에 카드만 읽는 파일(원장)이 바뀜 | 모델을 다시 만든다. 양식의 rev는 그 화면 입력만 덮어 지금 파일의 rev와 같다. 옛 화면은 카드가 없어 그 파일을 재지 않는다 |
| **[IC-U86](../inventory/cmd/pqcaton-ui/cards_test.go) ✅** | 스냅샷이 짝을 지은 뒤에 원장을 망가뜨림 | **조회 본문도 스냅샷 안에서 만든다.** 화면은 이미 만든 본문을 그리고 200이다. 본문이 정책 · 원장을 묶음 밖에서 다시 읽으면 카드와 다른 시점을 보인다 |
| **[IC-U87](../inventory/cmd/pqcaton-ui/cards_test.go) ✅** | 확정 뒤, 모델을 만든 직후에 영수증만 사라짐 | **확정 상태를 정하는 영수증과 기록도 스냅샷이 잰다.** 모델을 다시 만들고 카드는 「미확정」이다. 출력이 그대로여도 영수증이 바뀌면 다시 잰다 |

#### 오류 표시 (v0.23.0)

브라우저로 가는 오류는 모두 새 껍데기의 오류 페이지다. 화면에는 종류마다 정해 둔 그 말의 문구와 역할 · 파일 이름만
보이고, 오류 원문과 전체 경로는 표준오류에 참조 번호와 함께 남는다. 저장 · 확정 뒤 알림은 주소에 코드로만 실린다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-U88](../inventory/cmd/pqcaton-ui/errors_test.go) ✅** | 없는 주소 · 받지 않는 방식 · 잘못된 요청 · 필요한 파일 없음 · 입력을 읽지 못함 · 요청 보호 · 처리 중 panic을 두 말로 | 모두 새 껍데기의 오류 페이지(`role="alert"`)다. 상태 코드가 맞고, 경로 · 오류 원문 · 플래그 이름이 없고, 영어 페이지에 한글이 없다. 원인을 화면 말로 옮길 수 없는 것만 참조 번호를 보인다. 페이지마다 운영자 안내서의 오류 표가 쓰는 오류 코드가 있다 |
| **[IC-U89](../inventory/cmd/pqcaton-ui/errors_test.go) ✅** | 주소의 `msg` · `problem`에 임의의 글 · 경로 · 스크립트 · 맞지 않는 인자 · 확정 관문이 막은 까닭 | 임의의 값은 보이지 않고 「알 수 없는 알림」이다. 관문의 까닭은 코드로 실려 가서 그 말의 문장으로 그려진다 |
| [IC-U90](../inventory/cmd/pqcaton-ui/errors_test.go) ✅ | 화면 서버의 시험이 아닌 소스 | 평문 `http.Error`가 없다 |
| **[IC-U91](../inventory/cmd/pqcaton-ui/errors_test.go) ✅** | 저장소 센티널(안전하지 않은 파일 · 상태 불일치 · 손상 · 진행 중 · 실행 잠금 · 대상 잠금 · 오래된 화면 · 복구 필요)과 모르는 오류 | 값으로 거절 종류와 상태를 가린다. 거절 페이지는 두 말 모두 경로를 싣지 않는다. 모르는 오류는 참조 번호를 붙인 내부 오류다 |
| **[IC-U92](../inventory/cmd/pqcaton-ui/mode_test.go) ✅** | 저장하는 서버 · 저장을 지원하지 않는 플랫폼 · 복구가 남은 서버를 두 말로 | 화면 머리의 모드가 「저장 가능」 · 「보기 전용」 · 「복구 필요」로 실제 동작과 같다. 「읽기·검토 모드」 · 「원본은 보존」 문구는 어느 화면에도 없고, 보기 전용 거절은 Linux와 Intel Mac(검증함), Apple Silicon Mac(검증하지 않음)을 두 말로 적는다 |

#### 작업 폴더와 시작 (v0.23.0)

인자 없이 띄우면 홈의 `pqcaton` 작업 폴더를 쓴다. 파일 위치는 정해져 있고, 경로 플래그와 섞지 않는다. 포트를 연
뒤에 브라우저를 열고, 이미 떠 있으면 그 화면을 연다. 입력이 깨졌으면 진단 전용으로 열고, 선언이 없으면 화면에서
만든다.

| TC | Given → When | Then |
|---|---|---|
| [IC-U93](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅ | 인자 없이 시작 · 계층 CSV와 확정 정책 파일을 둔 작업 폴더 | 홈의 `pqcaton`과 `results/` · `layers/`를 0700으로 만든다. 모든 파일이 정해진 이름에 있고, 계층은 `layers/`의 CSV만 이름 순으로 겹치고, 확정 정책 파일이 있으면 그것이 기준이다 |
| [IC-U94](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅ | 작업 폴더와 경로 플래그 · 세션 파일을 함께 줌 | 사용법 오류로 시작하지 않는다. 플래그 뒤에 세션 파일을 두어도 작업 폴더로 뜨지 않고 사용법 오류다. 동작 플래그(`-addr` · `-no-browser` · `-org`)는 함께 쓸 수 있고, 세션 파일과 경로 플래그(명시 모드)는 그대로다 |
| **[IC-U95](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 작업 폴더로 시작 · `-no-browser` · 브라우저 명령이 없음 | 브라우저를 부를 때 그 주소에 이미 접속된다. `-no-browser`면 열지 않는다. 열지 못하면 경고만 한다 |
| **[IC-U96](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 같은 작업 폴더로 이미 떠 있는데 다시 실행 | 두 번째 서버를 띄우지 않고 적어 둔 주소를 연다. 그 주소의 서버가 같은 상태 디렉토리의 ID를 답할 때만 열고, 다른 화면이 답하거나 루프백이 아니면 열지 않고 실패한다 |
| **[IC-U97](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 작업 폴더의 선언 파일이 깨짐 | 서버는 뜨고, 화면과 저장 요청 모두 503 진단 페이지다. 「진단 전용」 모드와 파일 이름, 표준오류와 같은 참조 번호를 보이고 경로는 보이지 않는다 |
| **[IC-U98](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 선언이 없는 작업 폴더 | GET은 만들기 폼만 보이고 아무것도 만들지 않는다. 대조 · 판정은 닫혀 있고 02 카드는 「아직 계층이 없습니다」다. POST가 선언과 리뷰 세션을 만들고, 다시 보내면 덮어쓰지 않고 409다 |
| **[IC-U106](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 떠 있는 화면에 Ctrl+C(또는 SIGTERM)를 보냄 | 적어 둔 주소(`screen.addr`)를 지우고 0으로 끝난다. 실행 잠금도 풀려 같은 작업 폴더로 다시 띄울 수 있다 |
| **[IC-U111](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 실행기(`-open`): 낡은 주소만 있고 화면 없음 · 화면이 늦게 뜸 · 이미 떠 있음 · `-no-browser` | 낡은 주소는 열지 않고 기다리다 실패한다. 늦게 뜨면 그 화면이 같은 상태 ID를 답한 뒤에 새 주소를 연다. 떠 있으면 곧바로 연다. `-no-browser`면 주소만 찍는다. 새 화면을 띄우지 않는다 |
| **[IC-U99](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 선언에 형식이 틀린 IP와 포트 0인 엣지가 있음 | 01 화면이 어느 노드 · 엣지의 어느 칸이, 무엇이, 그대로 두면 어떻게 되는지를 두 말로 보인다. 01 카드에 그 수가 오르고, 판정 화면의 경고는 `pqcaton-report` 대신 01 화면을 가리킨다 |
| **[IC-U100](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 관측 결과 폴더가 빔 → 결과 파일을 넣고 다시 불러옴 | 대조 · 판정 화면이 두 말로 그 폴더의 경로를 보인다. 파일을 넣으면 안내가 사라지고, GET은 리뷰 세션을 다시 쓰지 않는다 |
| **[IC-U101](../inventory/cmd/pqcaton-ui/workspace_test.go) ✅** | 계층이 없는 작업 폴더 · 명시 모드 | GET은 만들기 폼만 보이고 아무것도 만들지 않는다(02 카드 「아직 계층이 없습니다」). POST가 `layers/<이름>.csv`와 스코프 세션을 만들고 화면은 규칙 편집이 된다. 같은 이름은 409로 덮어쓰지 않고, 틀린 이름은 400이다. 계층이 생기기 전의 스코프 저장 · 확정과 명시 모드의 만들기는 404다 |
| **[IC-U102](../inventory/cmd/pqcaton-ui/startup_test.go) ✅** | 복구 필요 상태에서 화면을 열고 진단 파일을 받음 · 복구 상태가 아닌 서버 | 화면은 503이고 남은 기록마다 파일 이름 · 하려던 일 · 지금 상태(밖에서 바뀜)를 두 말로 보인다. 전체 경로 · 해시 · 고치는 양식은 없다. 진단 파일은 `-recover-status -diagnostics`와 같은 JSON(전체 경로 포함)이고 내려받아도 파일은 그대로다. 복구 상태가 아니면 그 주소는 404다 |

### T. 말의 경계 (`tools/checktext`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-T1](../tools/checktext/main_test.go) ✅** | 한국어 주석과 한국어 문자열이 함께 있는 파일 | **문자열만 잡고 주석은 건드리지 않는다**. 주석까지 막으면 이 리포가 판단 근거를 적어 두는 방식이 통째로 막힌다 |
| **[IC-T2](../tools/checktext/main_test.go) ✅** | URL 안에 `//`가 있는 문자열 · 여러 줄 백틱 문자열 | 둘 다 잡는다. **정규식이 놓친 실제 세 자리**를 케이스로 남겨 둔 것이다. 그래서 `go/ast`로 본다 |
| [IC-T3](../tools/checktext/main_test.go) ✅ | 봐주는 파일(카탈로그·토글·테스트·생성물)과 그 옆 파일 | 목록에 적힌 것만 넘어간다. 규약으로 두면 새 파일이 슬그머니 예외가 된다 |

#### 터미널 없이 확정까지 (v0.23.0)

화면을 실제 HTTP로만 쓴다(쿠키 · 요청 보호 토큰 · 같은 출처). 관측 결과를 결과 폴더에 넣는 것만 pqcota 수집의
자리라 화면 밖이다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-U104](../inventory/cmd/pqcaton-ui/e2e_test.go) ✅** | 빈 작업 폴더에서 선언 만들기 → 노드 저장 → 결과 파일 도착 → 판정 · 서명 → 확인 → 확정, 두 말 | 05 카드가 「확정됨」이다. 계획 파일은 pqcota가 읽는 계약 형식(FinalizedPlan, IN_REVIEW)이고, 판정 원장은 선언의 조직에 묶인다. 영어 화면에 한글이 없다 |
| **[IC-U105](../inventory/cmd/pqcaton-ui/e2e_test.go) ✅** | 선언이 이미 있는 작업 폴더로 `-org` 없이 시작해 확정 · `-org acme`로 시작해 다른 조직의 선언 만들기 | 원장이 선언의 조직에 묶인다. `-org`를 준 서버는 그 조직의 선언만 만든다(다르면 400). 명시 모드는 `-org` 그대로다 |
| **[IC-U109](../inventory/cmd/pqcaton-ui/e2e_test.go) ✅** | 확정한 판정 화면 · 「새 검토 시작」 · 옛 확정 요청 다시 보내기 | 확정한 화면은 칸이 잠기고(`fieldset disabled`) 저장 · 확정 단추가 없으며 새 회차 단추(요청 보호 · rev)가 있다. 새 회차는 새 ID이고 결론을 옮기고 승인 서명을 비우며 05 카드가 「미확정」이 된다. 옛 확정을 다시 보내도 503(복구 필요)이 아니고 세션을 다시 쓰지 않는다 |
| **[IC-U110](../inventory/cmd/pqcaton-ui/e2e_test.go) ✅** | 같은 작업 폴더에서 판정 · 스코프 모두 확정 → 새 회차 → (재시작) → 확정 → 새 회차 → 확정 | 회차마다 새 세션 ID이고, 확정 뒤 카드는 「확정됨」, 새 회차 뒤에는 「미확정」이다. 원장에 세 판정 세션이 모두 남고 계획은 마지막 세션의 것이다. 재시작한 화면은 「저장 가능」이고 남은 기록이 없다 |

#### 운영자 안내서 (v0.23.0)

[운영자 안내서](operator-guide.md)와 [영어판](operator-guide.en.md)의 명령은 시험이 실제로 돌린다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-U107](../inventory/cmd/pqcaton-ui/guide_test.go) ✅** | 두 판의 명령 블록과 인라인 코드에 적힌 `pqcaton-ui`의 플래그 | 모두 `-h`가 찍는 목록에 있다 |
| **[IC-U108](../inventory/cmd/pqcaton-ui/guide_run_test.go) ✅** | 두 판에서 실행 표시가 붙은 명령을 빈 홈 폴더에서 차례대로 돌림 | 띄우기 · 떠 있는 화면 다시 열기 · 끝내기 · 루프백이 아닌 주소 거부 · 끈 상태의 백업과 복원 · 저장 도중 멈추고 밖에서 바뀐 파일의 복구가 안내서에 적힌 명령으로 끝난다. 멈춘 상태는 시험 도구(`testdata/guidetool`)가 저장소의 실패 주입으로 만든다 |
| **[IC-U112](../inventory/cmd/pqcaton-ui/guide_test.go) ✅** | 두 판의 기준판 문장 · `go install`의 판 · `raw.githubusercontent.com` 다운로드 주소의 판 | 모두 릴리스 노트 맨 위 절의 판과 같다. 새 판을 내며 안내서를 그대로 두면 낡은 판을 설치하게 되는 것을 막는다 |
| **[IC-U113](../inventory/cmd/pqcaton-ui/observe_test.go) ✅** | 03 관측 화면에서 비밀번호가 있는 줄의 노드 ID를 고치고, 다른 줄을 고치고, 새 WinRM 줄을 더해 저장 | 화면은 비밀번호를 「있음」으로만 보이고 값은 싣지 않는다. 비밀번호는 고친 노드 ID를 따라가고 새 줄에는 없다. 저장한 `hosts.csv`는 상류 `ParseHosts`가 그대로 읽고 소유자 전용(0600)이다 |
| **[IC-U114](../inventory/cmd/pqcaton-ui/observe_test.go) ✅** | WinRM을 Linux 노드에 고르고 같은 노드 ID를 두 번 적어 저장, 두 말 | 422이고 파일은 한 바이트도 바뀌지 않는다. 화면에는 적은 값이 남고 어느 노드의 무엇이 틀렸는지 적힌다. 리다이렉트하면 적던 것이 날아간다 |
| **[IC-U115](../inventory/cmd/pqcaton-ui/observe_test.go) ✅** | 명시 모드와 `hosts.csv`가 없는 작업 폴더에서 `/observe` | 명시 모드는 404이고 03 카드가 닫혀 있다. 작업 폴더 모드는 열리고 03 카드가 「아직 접근 정보가 없습니다」다 |
| **[IC-U116](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 「관측 시작」(가짜 ansible-playbook), 이전 결과가 0755 폴더에 있음 | 소유자 전용 인벤토리와 내장 플레이북을 실행 자리(results/ 옆 `observations/<id>/`, 상태 폴더 밖)에 쓰고 정해진 인자로 부른다. 모든 노드의 결과가 오고 0으로 끝나면 받은 세대와 results/를 **한 번에 맞바꾸고**, 이전 결과는 섞이지 않고 `previous/`로 간다. 인벤토리는 지우고 기록과 로그는 남는다. 로그에 찍힌 비밀번호는 화면과 로그 파일에서 가려진다 |
| **[IC-U117](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | ansible-playbook과 수집기 폴더가 없는 서버, 두 말 | 점검 목록이 빠진 것과 할 일을 적고 시작 양식을 내지 않는다. 주소로 직접 보내도 409(`run-not-ready`)이고 아무것도 실행하지 않는다 |
| **[IC-U118](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 돌고 있는 관측에 다시 시작 · 중지 · 범위 밖 관측 구간 | 다시 시작은 409(`run-busy`)다. 중지는 프로세스 묶음을 끝내고 받은 것은 `partial/`로 격리하며 results/는 그대로다. 노드 정리 플레이북을 한 번 더 돌린다. 도는 동안 상태 조각은 2초마다 다시 받고 중지 단추를 낸다. 관측 구간 4초는 400이다 |
| **[IC-U119](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | ansible-playbook이 종료 코드 2로 끝남 · 0으로 끝났지만 한 노드의 네트워크 결과가 빠짐 | 「실패」 · 「불완전」이고 종료 코드나 빠진 노드가 보이며 대조로 가는 길을 내지 않는다. results/는 그대로이고 받은 것은 `partial/`로 격리하며 노드 정리를 돌리고 인벤토리는 지운다 |
| **[IC-U120](../inventory/cmd/pqcaton-ui/approve_test.go) ✅** | 06 화면에서 승인자 이름 · 검토 확인과 함께 서명(작업 폴더 밖의 0600 키) | 서명 양식은 먼저 확인 페이지로 가고, 확인 페이지는 계획 · 계획 파일 해시 · 승인자 · 키 지문을 보이며 아무것도 쓰지 않는다. 확인을 거친 서명은 계획 파일 옆 `plan.signed.json`에 FINALIZED 계획을 쓰고 `plan.json`은 그대로다. 상류 `VerifyApprovals`가 그 승인자의 공개 키로 통과시키고 `Executable`도 지난다. 서명한 뒤에만 확인 키 줄이 보이고 06 카드는 「플레이북을 만들 수 있습니다」다. 다시 서명하면 409(`signed-exists`)다 |
| **[IC-U121](../inventory/cmd/pqcaton-ui/approve_test.go) ✅** | 키 없음 · 0644 · 심볼릭 링크 · 하드 링크가 더 있음 · 계획 폴더 안(그 폴더를 가리키는 링크를 거쳐도) · 개인 키가 아님, 두 말 · 콜론이 든 승인자 · 검토 확인 없음 | 화면은 까닭과 할 일을 적고 서명 양식을 내지 않는다. 주소로 직접 보내도 409(`sign-not-ready`)이고 아무것도 쓰지 않는다 |
| **[IC-U122](../inventory/cmd/pqcaton-ui/approve_test.go) ✅** | 서명 뒤 L3로 생성 · 그 뒤 계획이 바뀜 · 모르는 수준 | `plan.provision.yml` · `plan.rollback.yml`이 상류 생성기의 출력과 바이트까지 같고, 생성기가 짚은 빈 곳의 수가 06 카드에 오른다. 계획이 바뀌면 카드가 「서명이 지금 계획과 맞지 않습니다」이고 생성은 409(`generate-not-ready`)다. 모르는 수준은 400이다 |
| **[IC-U123](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 노드 ID `cmdb://web-1`, 공백 · 작은따옴표 · `{{ }}`가 든 이름 · 계정 · 키 경로 · 줄바꿈이나 탭이 든 노드 ID | 인벤토리의 호스트 이름은 별칭(n1)이고 값은 모두 `!unsafe` 문자열이라 한 줄을 벗어나거나 템플릿으로 풀리지 않는다. 제어 문자는 화면이 저장하지 않고(422), 손으로 넣은 파일이면 실행 전 점검이 막는다(409) |
| **[IC-U124](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 관측 도중 서버 종료 · 실행 자리를 남긴 비정상 종료 뒤 다시 시작 | 종료는 관측을 멈추고 인벤토리가 지워진 뒤에 돌아온다. 다시 시작하면 남은 인벤토리를 지우고 받던 것을 `partial/`로 옮기고 「중단됨」으로 적으며 03 화면이 그것을 보인다. results/는 그대로다 |
| **[IC-U125](../inventory/cmd/pqcaton-ui/approve_test.go) ✅** | 확인 뒤 승인자를 바꿔 보냄 · 확인 뒤 키 파일이 다른 키로 바뀜 · 확인 없이 서명 주소로 보냄 | 앞의 둘은 409(오래된 화면), 마지막은 403이고 어느 경우에도 서명한 계획 파일이 생기지 않는다. 확인 토큰은 계획 파일 해시 · 키 지문 · 승인자 · 만료 시각을 묶고, 서명은 잠금 안에서 그것을 다시 읽어 견준다 |
| **[IC-U126](../inventory/cmd/pqcaton-ui/approve_test.go) ✅** | 다른 키로 서명한 `plan.signed.json`이 있는 상태에서 서명 · 보관 · 다시 서명 · 맞는 서명의 보관 | 서명은 409(`signed-exists`)이고 파일은 바이트까지 그대로다. 보관은 지우지 않고 이름에 시각을 붙여 옆에 두며(내용 그대로), 그 뒤에 새로 서명할 수 있다. 맞는 서명은 보관하지 않는다(409) |
| **[IC-U127](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 받은 결과에 수집기 오류 문구 · 다른 노드의 결과 · 다른 수집기의 결과가 섞임 | 상류 디코더로 모두 읽어 하나라도 어긋나면 「불완전」이고, 어느 노드의 어느 결과가 왜 맞지 않는지 보인다. results/는 그대로이고 받은 것은 `partial/`로 간다 |
| **[IC-U128](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 맞바꾸기 실패 · 맞바꾼 뒤 이전 결과 옮기기 실패 · 맞바꾼 직후 화면이 끊김 | 맞바꾼 순간이 커밋이다. 맞바꾸기 실패는 지금 결과를 그대로 둔다. 그 뒤의 실패는 실행을 성공으로 두고 이전 결과의 보관만 「정리 필요」로 남겨 다음 시작이 마친다. 끊긴 자리도 다음 시작이 results/의 기록으로 알아보고 마친다 |
| **[IC-U129](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 감독이 살아 있는 중단 · 감독이 죽고 표지를 가진 프로세스가 남은 중단 · PID가 다른 프로세스에 다시 쓰인 기록 · 인벤토리가 없는 기록에서 다시 시작 | 살아 있는 감독은 마무리를 기다렸다가 끝낸다. 이 실행의 표지를 가진 프로세스를 모두 끝내고, 남은 인벤토리로 노드 정리를 돌리고, 인벤토리를 지우고, `partial/`로 옮기고, 「중단됨」으로 적는다. 시작 시각이 다른 프로세스와 다른 실행의 프로세스는 건드리지 않는다. 노드에 닿지 못했으면 03 화면이 남았을 스테이징 폴더를 실행 ID와 함께 적는다 |
| **[IC-U130](../inventory/cmd/pqcaton-ui/approve_test.go) ✅** | 「지난 서명 보관」이 준비 · 기록 · 적용 도중 멈춤 · 보관 이름이 이미 있음 · 다른 요청이 폴더를 쥠 | 다시 열어 복구하면 세 파일이 모두 보관됐거나 모두 그대로다. 이름이 있으면 409이고 아무것도 옮기지 않는다. 폴더를 쥐고 있으면 보관도 생성도 409(`busy`)이고 아무것도 바뀌지 않는다 |
| **[IC-U131](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 관측 도중 화면이 사라짐(감독과 이은 표준 입력이 닫힘) | 감독은 자기를 기록한 뒤에야 Ansible을 띄웠다. 화면이 사라지면 감독이 이 실행의 프로세스를 모두 끝내고, 노드를 정리하고, 인벤토리를 지우고, `partial/`로 옮기고, 「중단됨」으로 적은 뒤 끝난다. 표지를 가진 프로세스가 남지 않고 results/는 그대로다 |
| **[IC-U132](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 다시 시작할 때 이 실행의 프로세스를 끝내지 못함 | 노드 정리를 겹쳐 돌리지 않고, 인벤토리를 지우지 않고, 받던 것을 옮기지 않고, 「회수 필요」로 적는다. 03 화면이 PID와 스테이징 폴더를 보이고 새 관측은 409(`run-not-ready`)다. 프로세스가 사라진 뒤 다시 시작하면 남은 것을 회수한다 |
| **[IC-U133](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 세션을 떼어 나간 자식을 남기는 관측의 정상 종료와 중지 | Ansible은 `ControlMaster=no`로 돌고 실행 표지를 물려받는다. 끝난 뒤 표지를 가진 프로세스가 하나도 없다 |
| **[IC-U134](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 프로세스 목록이나 이 사용자 프로세스의 환경을 읽지 못함(관측을 마칠 때 · 다시 시작할 때) | 「확인할 수 없음」은 「남은 것 없음」이 아니다. 노드 정리 · 인벤토리 삭제 · 결과 이동을 하지 않고 「회수 필요」로 닫으며, 화면이 그 까닭을 보이고 새 관측은 409다. 덤프를 막아 환경을 읽을 수 없는 같은 사용자의 프로세스는 부모가 이 실행의 것이거나 같은 묶음에 지금 표지 프로세스가 있을 때 이 실행의 것으로 본다 |
| **[IC-U135](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 노드 정리 플레이북이 세션을 떼어 나간 자식을 남기고 0으로 끝남 · 그 자식을 끝내지 못함 | 정리 뒤에도 실행 표지로 찾아 끝낸 다음에야 인벤토리를 지운다. 끝내지 못하면 인벤토리를 지우지 않고 「회수 필요」로 닫는다 |
| **[IC-U136](../inventory/cmd/pqcaton-ui/observerun_test.go) ✅** | 찾은 뒤 PID가 시작 시각이 다른 프로세스에 다시 쓰임 | 신호를 보내기 직전에 같은 프로세스인지 다시 보고(Linux는 pidfd를 연 뒤), 시작 시각이 다르면 보내지 않는다. 찾은 그 프로세스에는 보낸다 |
| **[IC-U137](../inventory/cmd/pqcaton-ui/reuse_linux_test.go) ✅** | 지난 감독의 묶음 번호가 다른 묶음에 다시 쓰이고, 그 묶음의 같은 사용자 프로세스가 덤프 금지라 환경을 읽을 수 없음 · 표지 프로세스가 있는 묶음의 덤프 금지 구성원 | 기록해 둔 묶음 번호로는 아무것도 끝내지 않아 재사용된 묶음의 프로세스를 끝내지 않는다. 증명되지 않는 프로세스가 기록된 묶음에 남았으므로 없다고 치지도 않고 「회수 필요」로 멈춘다. 묶음은 지금 표지 프로세스가 있을 때만 이 실행의 것이며, 그때는 덤프 금지 구성원도 찾는다. 감독도 표지를 갖고 뜬다 |
| **[IC-U138](../inventory/cmd/pqcaton-ui/procargs_darwin_test.go) ✅** | macOS에서 Apple 플랫폼 바이너리(`/bin/sleep`)에 표지를 주고 띄움 · 그 바이너리가 표지 프로세스의 묶음에 듦 | `kern.procargs2`가 환경을 빼고 돌려주므로 「표지 없음」이 아니라 「읽을 수 없음」이다. 홀로 있으면 증명되지 않아 기록된 묶음이면 의심 프로세스로 잡히고, 환경이 읽히는 표지 프로세스의 묶음에 들면 이 실행의 것으로 찾힌다 |
| **[IC-U139](../inventory/cmd/pqcaton-ui/ruleset_test.go) ✅** | 상류 규칙 판이 다른 세션 파일로 화면을 열고 저장 · 확정 · 새 회차를 보냄 | 셋 다 409 `ruleset-changed`이고 **세션 파일은 바이트 그대로다** |
| **[IC-U140](../inventory/cmd/pqcaton-ui/ruleset_test.go) ✅** | 결과가 있는 화면에서 규칙 판이 빈 옛 세션 파일로 저장 · 세션 파일을 지우고 저장 | 옛 파일은 409 `ruleset-changed`이고 **바이트 그대로이며 오늘의 규칙 판이 찍히지 않는다.** 파일이 없으면 지금 규칙 판의 새 세션을 쓴다 |
| **[IC-U141](../inventory/cmd/pqcaton-ui/ruleset_test.go) ✅** | 다른 상류 규칙 판의 세션으로 판정 화면 · 「지금 규칙으로 새 회차 시작」 · 같은 단추를 다시 · 「다시 채택」 | 단추가 「남은 일」 상자(완료 모양이 아니다)로 보이고 05 카드는 「규칙 전환 회차가 필요함」이다. 누르면 옛 파일이 **바이트 그대로 보관**되고 그 자리에 지금 규칙의 새 세션(새 ID)과 보고서가 생긴다. 옛 결론은 참고값에만 있고 현재 칸은 빈다. 다시 눌러도 파일이 늘지 않는다. 화면이 보고서와 지난 판정을 보이고, 다시 채택은 그 항목만 옮긴다 |
| **[IC-U142](../inventory/cmd/pqcaton-ui/approve_test.go) ✅** | 다른 상류 규칙 판의 계획으로 06 화면 · 서명 확인 · 이미 서명된 그런 계획과 플레이북으로 화면 · 생성 · 보관 | 서명 양식이 없고 사유가 보이며 서명 확인은 409 `plan-ruleset`이다. 서명된 것이 있으면 생성 양식이 없고 생성은 409이며 **있는 플레이북은 바이트 그대로다.** 지금 계획만 지금 판으로 바꾸면 화면이 **서명된 계획 쪽이 맞지 않는다고 밝힌다.** 보관은 된다 |
| **[IC-U143](../inventory/cmd/pqcaton-ui/jca_test.go) ✅** | JCA 결과가 서비스 목록 없이 · 있게 · JCA 결과 없이 놓인 작업 폴더로 03 관측과 04 대조를 두 말로 | 첫째만 **두 화면 모두 노드 이름과 정해진 문구로** 경고하고, 말마다 그 말의 문구다. 나머지 둘은 경고가 없다. 결과가 화면의 관측에서 오지 않았어도 같다 |
| **[IC-U144](../inventory/cmd/pqcaton-ui/guide_test.go) ✅** | 판별 표(`docs/compatibility.md`)의 맨 위 줄 · 릴리스 노트 맨 위 절 · `go.mod` | 맨 위 줄의 pqcaton 판(범위면 끝 판)이 노트 맨 위의 판과 같고, `go.mod`의 pqcota 계열 모듈이 모두 같은 판이며 **고정한 pqcota 판이 그 판과 같다.** README(맨 위 소개 · 사전 준비 · 맞는 조합)와 데모 안내가 적은 판도 그 판이고, v0.25.0 뒤의 노트 절은 「받는 쪽이 달라지는 것」 첫 항목에 `함께 쓰는 pqcota: vX`를 적는다 |
| **[IC-U149](../inventory/cmd/pqcaton-ui/plan_scope_test.go) ✅** | 확인 페이지를 연 뒤 계획의 영수증이 사라짐 · 정책 A에서 서명 · 생성한 뒤 정책이 B로 바뀜 | 서명은 **잠금 안의 재검사**가 409 `plan-scope`로 막는다. 활성화 뒤에는 생성 양식이 없고 생성이 409이며, 계획 · 서명 · 플레이북이 바이트 그대로다. 보관은 되고 세 파일을 지우지 않고 옮긴다 |
| **[IC-U151](../inventory/cmd/pqcaton-ui/plan_scope_test.go) ✅** | 형식 1 영수증의 계획을 서명 전 · 서명하고 플레이북까지 만든 뒤 | 서명 양식 · 생성 양식이 없고 서명 확인 · 생성이 409다. 이미 만든 플레이북은 바이트 그대로다. 화면이 「현재 정책에서 05의 새 판정 회차를 열어 다시 확정」을 안내한다 |
| **[IC-U152](../demo/e2e/e2e_test.go) ✅** | 데모 종단(`make e2e-demo`): pqcota 재적재 전에 화면과 명령으로 활성화 · `-confirm-ingest` · PQCOTA_DSN 없이 · 손으로 고친 후보 · apply-scope(재적재만) · 작업 폴더 복사본에서 명령으로 활성화 · 작업 폴더에서 화면으로 활성화 · 새 회차 확정 뒤 06 서명 · 생성 · v0.25.0 태그로 빌드한 옛 바이너리로 작업 폴더 복사본 열기 | 재적재 전의 활성화는 화면이 409 `upstream-missing`(**관리 자산이 0개가 된 노드도 대조 대상이라 반드시 거절**), 명령도 거절하고, `-confirm-ingest` · DSN 없음 · 손으로 고친 후보도 거절한다. 어느 경우에도 정책 · 세션 · 계획 · 처리 기록 바이트와 pqcota 스냅샷 수가 그대로이고, apply-scope는 작업 폴더를 바꾸지 않는다. 두 길 모두 성공하면 활성 정책 · 새 세션 · 활성화 처리 기록의 지문이 같고 basis가 upstream-verified이며, 보고서에 `upstream_check`가 있고 DSN이 기록에 없다. 06은 옛 계획을 막고, 새 회차의 계획은 06 화면 요청으로 서명(FINALIZED · 승인 하나)되고 플레이북 둘이 만들어진다. 05는 지난 결론을 참고값으로만, 회차의 근거를 pqcota 이력 대조로 보인다. **옛 바이너리는 상태 문서가 형식 2라 화면이 시작하지 않고**(05 · 06의 어떤 양식도 없다) 상태를 쓰는 명령도 멈추며, 파일 · 처리 기록 · 트랜잭션 기록 · 준비 파일이 그대로다. 환경이 없거나 상류 다섯 리포가 go.mod가 고정한 판의 태그에 있지 않으면 건너뛰지 않고 실패한다. 상류 데모는 **이번 실행만의 dind 컨테이너 안의 데몬**에 띄우므로, 호스트에 같은 이름의 pqcota 데모가 떠 있어도 부딪히지 않고 건드리지 않는다. 끝나면(실패해도) 그 dind 컨테이너와 볼륨이 호스트에 남지 않는다. 안쪽에서도 정리는 up.sh보다 먼저 걸고 이번 실행의 생성 파일 라벨이 붙은 자원만 지운다 |
| **[IC-U153](../inventory/cmd/pqcaton-ui/scope_session_test.go) ✅** | 정책 기록이 없는 옛 세션 · 정책 A로 세운 세션을 다른 정책으로 · 정책 없이 · 둘 다 정책 없음으로 화면에서 확정 | 첫째는 409 `scope-baseline`, 둘째와 셋째는 409 `scope-unactivated`이고 어떤 파일도 바뀌지 않는다. 마지막은 확인 페이지가 열린다 |
| **[IC-U154](../inventory/cmd/pqcaton-ui/scope_lock_test.go) ✅** | 정책 기록이 없는 옛 세션으로 05 화면 · 저장 · pqcota 적재 전 기준선(확인 칸을 보내도) · 적재 뒤 기준선 · 같은 요청 다시 | 기준선 상자가 보이고 운영자 확인 칸은 없으며 05 카드가 「정책 기준선을 채택해야 합니다」다. 저장은 409 `scope-baseline`, 적재 전 기준선은 409 `upstream-missing`이고 파일은 그대로다. 적재 뒤에는 새 세션(새 ID · 지금 정책의 지문 · 현재 결론은 빔)이 생기고 옛 세션은 바이트 그대로 보관되며 basis가 upstream-verified인 scope-baseline 형식 2 처리 기록이 남는다. 다시 보내면 거절되고 파일이 늘지 않는다 |
| **[IC-U155](../inventory/cmd/pqcaton-ui/scope_lock_test.go) ✅** | 정책 A로 저장한 뒤 `scope-assets.csv`를 손으로 B로 바꿈 · 다시 A로 복원 | 05가 잠기고 회차 단추가 없으며 이전 정책 복원과 정식 활성화만 안내한다. 저장은 409 `scope-unactivated`이고 파일은 그대로다. 복원하면 다시 저장된다 |
| [IC-U156](../inventory/cmd/pqcaton-ui/scope_lock_test.go) ✅ | note · 주석만 바꾼 정책 | 잠기지 않고 저장된다 |
| **[IC-U157](../inventory/cmd/pqcaton-ui/scope_activate_test.go) ✅** | 02 확정 정책이 활성 정책과 다르고 pqcota가 새 정책으로 적재된 작업 폴더로 05 화면 · 확인 페이지 · 활성화 · 활성화 전 계획 서명 · 같은 활성화 다시 · 손으로 고친 후보 | 05에 활성화 상자가 있다. 확인 페이지는 두 지문 · 바뀌는 파일 · 되돌리기 제한과 백업 위치를 적고 아무것도 쓰지 않는다. 활성화하면 활성 정책이 바뀌고 옛 정책과 옛 세션이 보관되며 새 세션은 새 정책의 지문을 든다. 활성화 전 계획은 06에서 서명되지 않는다. 같은 활성화는 409 `scope-same`, 손으로 고친 후보는 409 `scope-not-finalized`다 |
| **[IC-U158](../inventory/cmd/pqcaton-ui/scope_activate_test.go) ✅** | 02 확정 정책이 활성 정책과 다른 작업 폴더로 04 화면 · 활성화 기록 없이 바뀐 정책으로 잠긴 세션 · 확정 뒤 손으로 고친 후보 · 활성화한 뒤 | 05와 같은 판정으로 04에 **아직 활성화되지 않았다는 안내**(두 지문, 이 대조가 활성 정책으로 한 것이라는 문장)와 05의 활성화 상자로 가는 링크가 있고, 활성화 양식은 없다. 05에 그 링크가 가리키는 상자가 있다. 잠긴 세션 · 손으로 고친 후보 · 활성화한 뒤에는 알리지 않는다 |
| **[IC-U159](../inventory/cmd/pqcaton-ui/scope_activate_test.go) ✅** | 02 확정 알림이 붙은 02 화면(활성화 대기 · 알림 없음 · 05 잠김 · 후보가 활성 정책과 같음) · 05 · 04 | **확정 알림과 활성화 대기가 모두 있을 때만** 02에 다음 단계와 두 링크(먼저 `/survey`, 그다음 `/review#scope-activate`)가 있다. 05의 활성화 상자는 상류 재적재 · 재적재 확인 · 정책 활성화를 그 순서로 보이고 명령을 싣는다. 04의 단추는 「05에서 활성화 절차 보기」다 |
| **[IC-U160](../inventory/cmd/pqcaton-ui/nul_test.go) ✅** | NUL 없는 계획을 v0.10.6으로 만든 정규형 해시 · 승인과 견줌 | **정규형이 v0.10.6과 바이트까지 같고**, 같은 키로 다시 서명하면 같은 서명이며, v0.10.6의 승인이 지금도 승인으로 남는다 |
| **[IC-U161](../inventory/cmd/pqcaton-ui/nul_test.go) ✅** | v0.10.6에서 정규형이 충돌하던 계획 한 쌍(provider_choice와 config_artifact 사이로 NUL을 옮김) | 두 계획의 정규형은 지금도 같다. **서명 · 승인 확인 · 실행 근거 판정이 모두 거절한다**: 서명을 만들지 않고, v0.10.6에서 A에 만든 승인이 A에도 B에도 승인으로 남지 않으며, 승인과 확정 시각을 갖춰도 Executable이 ErrNUL이다 |
| **[IC-U162](../inventory/cmd/pqcaton-ui/nul_test.go) ✅** | 옛 판에서 확정된 NUL 계획으로 06 화면 · 서명 확인 · 서명(확인 양식을 갖춰 잠금 안까지) · 생성 | **네 자리 모두 `plan-text`다.** 화면은 서명 · 생성 양식이 없고 새 판정 회차로 고치라는 까닭을 두 말로 보인다. 나머지는 409이고 작업 폴더는 바이트 그대로다 |
| **[IC-U163](../inventory/cmd/pqcaton-ui/nul_test.go) ✅** | 지금 계획은 깨끗하고 서명한 계획에 NUL | 화면이 서명한 계획의 문제로 까닭을 보이고 생성 양식이 없으며 보관은 할 수 있다. 생성은 409 `plan-text`이고 파일은 그대로다 |
| **[IC-U164](../inventory/cmd/pqcaton-ui/nul_test.go) ✅** | 되돌림 메모에 NUL을 넣고 05 확정 요청 | **확인 페이지부터 409 `session-text`이고 확정하지 않는다.** 세션 · 계획 · 영수증은 바이트 그대로이고, 거절문은 확정 전이라 그 자리에서 고치라고 안내한다 |
| **[IC-U165](../inventory/cmd/pqcaton-ui/collectors_test.go) ✅** | 수집기가 모두 고정 판으로 확인됨(의존에도 replace 없음) | **통과하고 파일마다 판과 출처를 보인다**(「선언된 판 · 실제 소스: 태그 커밋 · 확인됨」) |
| **[IC-U166](../inventory/cmd/pqcaton-ui/collectors_test.go) ✅** | 옛 판 수집기, 또는 수집기끼리 판이 섞임 | **시작하지 않고(409 `run-not-ready`), `-allow-unverified-collectors`로도 넘기지 못한다**. 옛 `pqcota-jvmscan`이 적재 요청을 통째로 막던 사고가 여기 걸린다 |
| **[IC-U167](../inventory/cmd/pqcaton-ui/collectors_test.go) ✅** | 판을 확인할 수 없는 수집기(상류 데모처럼 replace로 빌드, 태그 체크아웃이어도 의존이 replace) | **기본으로 시작하지 않는다**. 「실제 소스: 로컬 replace(devel) · 확인 불가」나 「수집기 소스 v0.10.7 확인 · replace 의존 소스 확인 불가」로 보인다. `-allow-unverified-collectors`로 띄운 화면만 허용하고, 허용한 사실과 파일 목록을 화면과 표준 오류에 남긴다. 작업 폴더에는 남기지 않는다 |
| **[IC-U168](../inventory/cmd/pqcaton-ui/collectors_test.go) ✅** | `collector.jar`에 판 정보가 없음 / 다른 판 | 판이 없으면 **경고만 하고 시작은 된다**(override를 요구하지 않는다). 판이 있는데 다르면 막는다 |
| [IC-U169](../inventory/cmd/pqcaton-ui/collectors_test.go) ✅ | `-allow-unverified-collectors`를 주거나 주지 않고 시작 | 기본은 꺼져 있고, 주면 서버에 그대로 닿는다 |
| **[IC-U170](../inventory/cmd/pqcaton-ui/collectors_test.go) ✅** | 화면이 `pqcota-discovery`를 링크하지 않음 | **그래도 고정 판을 안다**. discovery가 있으면 그 판, 없으면 `pqcota-common`의 판을 쓴다. discovery만 찾던 때는 데모에서 모든 수집기가 「고정한 판을 알 수 없음」이 됐다 |
| **[IC-U171](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 기본 위치에 다른 키가 있는데 `-approval-key`를 줌(`-no-default-approval-key`를 함께 줘도) | **명시한 키로 서명한다**. 명시 경로가 언제나 우선하고, 후자는 기본 검색만 끈다 |
| [IC-U172](../inventory/cmd/pqcaton-ui/keys_test.go) ✅ | 기본 위치에 키가 있는데 `-no-default-approval-key`로 시작 | 기본 키 · 확인용 공개 키를 쓰지 않는다. 06이 「자동 검색을 껐습니다」를 보이고 서명 양식을 내지 않는다 |
| [IC-U173](../inventory/cmd/pqcaton-ui/keys_test.go) ✅ | 기본 위치에 키가 없음 | **정상 시작하고 새 서명만 하지 않는다**. 표준 오류에 찾은 자리를 한 줄 남긴다 |
| **[IC-U174](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 기본 위치에 정상 키 | 그 키로 서명한다. 06에는 「사용자 설정 폴더의 pqcaton/approval.key」와 지문만 보이고, **화면 · 표준 오류 · 서명한 계획 어디에도 개인 키가 없다**. 표준 오류에는 전체 경로가 남는다 |
| **[IC-U175](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 기본 위치의 키가 다른 사용자도 읽을 수 있거나(0644) 심볼릭 링크 | **정상 시작하되 06과 표준 오류에 분명히 알리고 서명을 막는다**(409) |
| **[IC-U176](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 명시한 `-approval-key` · `-approval-verify-key`가 잘못됨 | **화면을 시작하지 않는다**(종료 1). 화면 주소도 남기지 않는다. 알리지 않고 다른 모드로 열지 않는다 |
| **[IC-U177](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 개인 키 없이 이미 서명된 계획 | 확인용 공개 키(`approval.pub`)가 있으면 **확인하고 플레이북을 만든다**. 없으면 확인하지 못한 것으로 두고 그 파일을 두라고 안내한다. 새 서명은 어느 경우에도 하지 않는다 |
| [IC-U178](../inventory/cmd/pqcaton-ui/keys_test.go) ✅ | 기본 위치의 개인 키와 확인용 공개 키가 서로 다른 키 / 둘 다 명시했는데 다른 키 | 기본 짝이면 서명도 확인도 하지 않고 06과 표준 오류가 알린다. 둘 다 명시했으면 **화면을 시작하지 않는다** |
| [IC-U179](../inventory/cmd/pqcaton-ui/keys_test.go) ✅ | 확인용 공개 키 파일이 다른 사용자도 읽기만 됨 / 고칠 수 있음 / 개인 키 줄을 담음 | 읽기만 되면 받는다. 고칠 수 있거나 개인 키를 담으면 받지 않는다 |
| **[IC-U180](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 명시한 개인 키, 기본 위치에 다른 키의 `approval.pub` | **기본 공개 키를 읽지 않는다**. 명시한 키에서 공개 키를 뽑아 서명 · 확인 · 생성한다 |
| **[IC-U181](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 명시한 공개 키만, 기본 위치에 개인 키 | 기본 개인 키를 읽지 않는다. 06이 「확인용 공개 키만 지정했습니다」를 보이고, 서명은 하지 않고 생성은 한다 |
| **[IC-U182](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 잘못된 기본 개인 키(0644)와 정상 기본 공개 키 | **서명은 막고 확인 · 생성은 한다** |
| **[IC-U183](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 정상 기본 개인 키와 다른 사용자가 고칠 수 있는 기본 공개 키 | **구성 오류로 서명도 생성도 막는다**. 06과 표준 오류가 알린다 |
| **[IC-U184](../inventory/cmd/pqcaton-ui/procinfo_unix_test.go) ✅** | 「회수 필요」로 남긴 프로세스가 살아 있음(비밀 인자 · 비밀 환경 변수를 가짐) | **PID · 부모 PID · 묶음 · 사용자 · 실행 파일 · 시작 시각을 보이고, 인자는 가린 값만, 환경 변수는 싣지 않는다** |
| **[IC-U185](../inventory/cmd/pqcaton-ui/procinfo_unix_test.go) ✅** | 남긴 프로세스가 이미 종료됨 | 오류가 아니라 「이미 종료됨」으로 보인다 |
| **[IC-U186](../inventory/cmd/pqcaton-ui/procinfo_unix_test.go) ✅** | PID는 같은데 시작 시각이 다름(PID 재사용) | **「다른 프로세스로 바뀜」이고 세부를 싣지 않는다**. 회수 대상으로 안내하지 않는다 |
| **[IC-U187](../inventory/cmd/pqcaton-ui/procinfo_unix_test.go) ✅** | 조회 권한 없음 · 다른 사용자의 프로세스(조회기 주입) / 시작 시각 없는 기록 / 조회 실패 | 「조회 권한 없음」은 **실행 파일과 인자를 모델에 싣지 않는다**. 「같은 프로세스인지 확인할 수 없음」 · 「조회 실패」로 보인다. 이 기계의 PID 1이 다른 사용자의 것이면 실물로도 잰다(보조) |
| **[IC-U188](../inventory/cmd/pqcaton-ui/procinfo_unix_test.go) ✅** | 비밀이 든 인자 | DSN 사용자 정보 · password= · token= 꼴 · …_KEY= 꼴 · 값을 받는 비밀 플래그의 다음 인자 · PEM 개인 키를 가리고 나머지는 둔다 |
| **[IC-U189](../inventory/cmd/pqcaton-ui/procinfo_unix_test.go) ✅** | 03 화면에서 남은 프로세스 보기와 「다시 확인」 | 가린 인자만 HTML에 싣고 **끝내는 단추가 없다**. 두 번 불러도 작업 폴더가 한 바이트도 바뀌지 않는다 |
| [IC-U190](../inventory/cmd/pqcaton-ui/procinfo_darwin_test.go) ✅ | macOS의 kern.procargs2 응답 | 실행 파일과 인자 argc개만 읽고, 뒤에 붙은 환경은 돌려주지 않는다(실제 조회는 최종 검증에서) |
| **[IC-U191](../inventory/cmd/pqcaton-ui/procinfo_unix_test.go) ✅** | 시작 시각을 남기지 않은 이전 판의 기록(PID만 있음) | 사라지지 않고 **「같은 프로세스인지 확인할 수 없음」**으로 보인다. 살아 있는 PID여도 세부와 인자를 싣지 않는다 |
| **[IC-U192](../inventory/cmd/pqcaton-ui/keys_test.go) ✅** | 실제 `os.UserConfigDir()`의 기본 위치가 공백이 든 경로(macOS의 `Application Support` 같은 것) | **개인 키와 공개 키를 읽고 서명한다**. macOS는 `~/Library/Application Support/pqcaton/`, Linux는 `$XDG_CONFIG_HOME/pqcaton/`이다. 명시 키 우선과 `-no-default-approval-key`도 같은 자리에서 맞다 |
| [IC-U193](../inventory/cmd/pqcaton-ui/planfields_test.go) ✅ | 새 필드가 없는 계획(상류 v0.11.0 모듈) | 지금처럼 서명하고 플레이북을 만든다 |
| **[IC-U194](../inventory/cmd/pqcaton-ui/planfields_test.go) ✅** | 소스 근거나 승인 가이드가 든 계획(이 작업 폴더에서 확정해 영수증과 맞음) | **화면 상태 · 서명 확인 · 실제 서명(잠금 안) · 생성(잠금 안)의 네 자리에서 409 `plan-fields`이고 승인 서명이 붙지 않는다**. 화면은 서명 · 생성 양식을 내지 않고 「계획이 손상된 것은 아니며 pqcaton이 아직 그 내용을 검토 · 생성하지 않는다」는 한계를 보인다. 작업 폴더는 바이트 그대로다 |
| **[IC-U195](../inventory/cmd/pqcaton-ui/planfields_test.go) ✅** | 서명한 계획(plan.signed.json)에 승인 가이드 | 생성하지 않고(409 `plan-fields`) 보관만 허용한다 |
| **[IC-U196](../inventory/cmd/pqcaton-ui/upstream_test.go) ✅** | `PQCOTA_DSN` 없음 · 닿지 않는 DSN(사용자 · 비밀번호 포함) · 필수 조회가 실패하는 이력 · 빈 메모리 이력으로 화면 시작 | **화면은 pqcota 이력 없이 시작하지 않는다.** 앞의 셋은 종료 코드 1이고 포트를 열지 않으며 주소 파일을 쓰지 않는다. 표준오류에 DSN의 사용자 · 비밀번호 · 호스트가 없다. 빈 이력이면 뜬다 |
| **[IC-U197](../inventory/cmd/pqcaton-ui/upstream_test.go) ✅** | 05 화면 · pqcota가 옛 정책으로만 적재됨(진술 칸을 보내도) · 닿지 못함 · 새 정책으로 적재한 뒤 확인 페이지 · 그 뒤 옛 정책으로 다시 적재하고 활성화 | **05의 활성화는 pqcota 이력 대조를 거쳐야 한다.** 상자에 진술 칸이 없고 대조 단계를 적는다. 차례로 409 `upstream-missing` · 503 `upstream-unreachable` · 「확인 시점에 pqcota 이력과 일치했습니다」와 노드 수 · 409 `upstream-missing`(실행은 처음부터 다시 대조한다)이고, 거절은 모두 작업 폴더를 그대로 둔다 |
| **[IC-U198](../inventory/cmd/pqcaton-ui/upstream_test.go) ✅** | 대조 도중 결과 폴더가 바뀌는 활성화 · 성공한 활성화 | 앞은 잠금 안에서 409 `stale`이고 처리 기록을 쓰지 않는다. 뒤는 새 세션의 근거와 처리 기록의 basis가 upstream-verified, 보고서에 `upstream_check`(확인 시각 · 조직 · 노드별 지문)가 있고 05가 「확인 시점에 pqcota 이력과 일치했습니다」를 보인다 |
| **[IC-U199](../inventory/cmd/pqcaton-ui/upstream_test.go) ✅** | A로 적재한 뒤 B로 다시 적재한 pqcota 이력에서 A로 되돌리는 활성화(진술 칸을 보내도) | **이전 정책 복귀는 막는다.** 409 `upstream-revert`와 「이전 정책 복귀를 막습니다」이고 아무것도 쓰지 않는다 |
| **[IC-U200](../inventory/cmd/pqcaton-ui/upstream_test.go) ✅** | PQCOTA_DSN이 환경에 있음 · 없고 pqcota.env가 정상 · 셸 문법이 든 값 · 파일 없음 · 0640 · 0604 · 0620 · 0660 · 다른 변수 · export · 따옴표 · 두 번 · 빈 값 · 줄 없음 · 앞뒤 공백 · 값 안의 `=` · 심볼릭 링크 · 링크한 부모로 작업 폴더 안 · 하드 링크 · 폴더, 화면 시작, 화면 환경에 DSN을 둔 채 관측(성공 · 실패 뒤 정리) | **pqcota.env는 데이터로만, 안전한 파일에서만 읽는다.** 환경 변수가 먼저다. 정상 파일은 값 그대로(셸로 평가하지 않고, 첫 `=`에서만 나눠 값 안의 `=`가 남는다)이고, 나머지는 모두 거절하며(작업 폴더 안은 실제 경로로 본다) 오류에 값이 없다. 파일로 화면이 뜨고, 권한이 열린 파일이면 이력을 열기 전에 시작하지 않는다. 실제로 띄운 감독 · Ansible(관측과 정리)의 환경에 PQCOTA_DSN이 없고 표준오류 · 관측 로그에도 값이 없다 |
| **[IC-U201](../inventory/cmd/pqcaton-ui/upstream_test.go) ✅** | `-upstream-org`에 다른 조직 · 같은 조직으로 화면 시작, 다른 조직의 판정 세션 · 다른 조직의 이력으로 05 활성화 | **pqcota 이력의 조직은 판정 조직이다.** 다른 조직이면 이력을 열지 않고 시작하지 않으며, 같은 값은 받는다. 05는 조회하지 않고 409 `upstream-org`이며 아무것도 쓰지 않는다 |

### K. 문체의 경계 (`tools/checkprose`): 엔진은 상류에

문서와 화면 문구의 한국어를 잰다. **한 번 없앤 말이 다시 들어오지 않게** 막는 것이 전부다.
지금 있는 것은 `baseline.tsv`에 파일마다 적어 두고 **늘어나는 것만** 막는다.

**엔진과 그 케이스(IC-K1~18)는 2026-09-17에 상류 pqcota `tools/checkprose`로 옮겼다.** 코드를
리포마다 복사해 두면 HTML 주석 덮기 같은 공통 수정을 세 곳에 같은 커밋으로 맞춰야 했고, 어느
쪽이 원본인지가 라이선스 문제가 됐다. 이 리포에는 설정만 둔다: `rules.tsv`(무엇을 막나. `게이트`·
`머신`처럼 이 리포만의 낱말은 여기서 정한다) · `notices.tsv`(알림) · `overlap.txt`(잘못 잡는 말) ·
`files.txt`(마크다운 밖에서 볼 화면 문구 파일) · `baseline.tsv`(기준선). Makefile이 상류의 판을 고정해
이 설정으로 돌린다(`PROSE_CHECKER`). 케이스 관문(IC-M8)은 이 절에 번호가 없으므로 아무것도 재지
않는다.

### B. 두 말의 짝 (`tools/checklang`) ✅

공개 사이트 두 장(`site/*.html`)은 문구를 두 벌 적어 두고 CSS 두 줄로 한쪽을 숨긴다. 이
방식은 **한쪽 말로 볼 때는 멀쩡해 보이는 고장**을 만든다. `<span lang="ko">`를 닫지 않은
자리가 아홉 곳 있었는데, 닫는 태그가 없으니 파서가 뒤따르는 `<span lang="en">`을 그 안으로
넣었고, 영어로 열면 바깥 한국어가 숨겨질 때 안쪽 영어까지 함께 사라져 표 머리글 여섯과 표
셀 셋이 통째로 빈칸이 됐다. 한국어로 보면 아홉 자리가 다 멀쩡했다.

**위의 관문 넷이 그것을 하나도 잡지 못했다.** 사람이 사이트를 영어로 열어 본 날에야 드러났다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-B1](../tools/checklang/main_test.go) ✅** | `</span>`이 빠져 한국어가 영어를 품은 자리 | 잡는다. **2026-08-27에 실제로 난 고장의 모양이다.** 한국어로 보면 멀쩡해서 사람 눈으로는 걸러지지 않던 자리다 |
| [IC-B2](../tools/checklang/main_test.go) ✅ | 한 말만 적힌 자리 | 잡는다. 다른 말로 열면 그 자리가 빈다 |
| **[IC-B3](../tools/checklang/main_test.go) ✅** | 나란히 적힌 올바른 짝 | 통과시킨다. 두 장의 요소 오백두 개가 모두 이 모양이라, 오탐을 내면 관문을 아무도 켜지 않는다 |
| [IC-B4](../tools/checklang/main_test.go) ✅ | `<html lang="ko">` | 세지 않는다. 문서 전체의 초깃값이라 짝이 없는 것이 맞다. 스크립트가 막혀도 한국어가 보이게 하는 자리다 |
| **[IC-B5](../tools/checklang/main_test.go) ✅** | `lang="kr"` 같은 오타 | 잡는다. CSS 두 줄은 `ko`·`en`에만 걸리므로 그 값은 **두 말에서 모두 보인다**. 숨겨지는 고장보다 알아채기 어렵다 |
| **[IC-B6](../tools/checklang/main_test.go) ✅** | 중첩된 자리 | **바깥만 짚는다.** 안쪽까지 세면 아홉 자리가 열여덟 줄로 나와 무엇을 고쳐야 하는지 흐려진다. 바깥의 닫는 태그를 넣으면 둘 다 풀린다 |
| **[IC-B7](../tools/checklang/main_test.go) ✅** | 「정의」처럼 짧고 흔한 낱말이 앞쪽에도 있는 원본 | 앞쪽이 아니라 **실제 자리의 줄**을 짚는다. 품은 글자를 원본에서 되찾는 첫 판은 437을 357로, 438을 242로 짚었다. 그래서 토크나이저로 태그가 나온 줄을 센다 |
| [IC-B8](../tools/checklang/main_test.go) ✅ | 원본의 태그 순서와 트리 순서가 어긋남 | 줄을 통째로 버리고 파일 이름만 낸다. **어긋난 줄은 없는 줄보다 나쁘다** |
| [IC-B9](../tools/checklang/main_test.go) ✅ | `site/`에 장이 하나도 없음 | 통과가 아니라 오류다. 경로가 어긋난 날 아무것도 재지 않으면서 통과 표시만 내면, 막을 것을 못 막는 관문이 된다 |

> **글자를 세는 검사로는 안 됩니다.** 닫는 태그가 빠져도 글자는 그대로 남아 있어서, 문자열을
> 세는 방식은 아홉 자리를 모두 통과시킵니다. 고장이 브라우저가 그 HTML을 **어떻게 읽는가**에서
> 생기므로 같은 규칙으로 읽는 파서(`golang.org/x/net/html`)라야 보입니다. `tools/checktext`가
> `go/ast`를 쓰는 것과 같은 이유입니다(IC-T2).

### M. 번호와 테스트의 대응 (`tools/checkcases`) ✅

이 문서 첫머리가 **「케이스 번호가 곧 테스트 파일 링크입니다」**라고 약속한다. 그 약속을
사람이 지키게 두었더니 백일흔넷 가운데 링크가 하나도 없었고, 두 주 동안 아무도 몰랐다.
그래서 **약속을 지키는 일을 기계에 맡긴다.** 링크는 `-write`가 만든다.

**케이스 표가 둘이다.** 인벤토리는 이 문서에, 러너는
[`saas/runner/README.md`](../saas/runner/README.md)에 있다. 코드가 거기 있으니 케이스도 거기
있어야 한다. 관문이 둘을 함께 읽고, 문서마다 맡는 접두어(`IC` · `RUN`)만 그 문서에서 찾는다.

**문서와 접두어의 목록은 코드가 아니라 [`tools/checkcases/docs.tsv`](../tools/checkcases/docs.tsv)에
있다.** 엔진은 리포마다 같고 다른 것은 이 목록뿐이기 때문이다. 문서마다 접두어를 둘로 나눠 적는다.
`owned`는 이 리포의 테스트가 재는 것이고, `external`은 다른 리포가 재므로 여기서는 테스트를
요구하지 않는 것이다. 경로는 리포 루트 기준이다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-M1](../tools/checkcases/main_test.go) ✅** | 테스트가 `// IC-R1·R2·R3`로 번호를 축약해 적음 | 셋으로 펴서 읽는다. 못 펴면 「✅ 인데 테스트가 없다」가 거짓으로 뜬다. 이 도구를 만들기 전에 손으로 세다가 실제로 여섯 건을 그렇게 잘못 셌다 |
| [IC-M2](../tools/checkcases/main_test.go) ✅ | 번호 칸의 두 모양(굵은 것·안 굵은 것)과 이미 링크가 붙은 것 | 셋 다 읽는다. 하나라도 못 읽으면 그 케이스만 관문 밖이 된다 |
| **[IC-M3](../tools/checkcases/main_test.go) ✅** | `-write`로 링크를 찍음 | **굵게와 상태 표시를 그대로 둔다.** 모양이 달라지면 사람이 diff를 못 읽고, 못 읽으면 다음부터 안 돌린다 |
| [IC-M4](../tools/checkcases/main_test.go) ✅ | 찍은 문서를 다시 찍음 | 달라지지 않는다. 링크가 겹쳐 쌓이면 문서가 망가진다 |
| [IC-M5](../tools/checkcases/main_test.go) ✅ | 한 번호를 두 파일이 잼(엣지판·본판) | 둘 다 잡고 정렬해 첫 파일로 링크한다. 어긋남으로 세지 않는다 |
| **[IC-M6](../tools/checkcases/main_test.go) ✅** | **함께 나가는 문서와 테스트** | 실제로 맞는다. 위 다섯은 만들어 낸 입력으로 재는 것이라, 진짜 리포를 재는 이 케이스가 이 도구의 존재 이유다 |
| **[IC-M7](../tools/checkcases/main_test.go) ✅** | 픽스처로 케이스 표 한 줄을 문자열에 담은 테스트 파일 | **주석의 번호만 잡는다.** 파일 전체를 정규식으로 훑었더니 그 문자열 때문에 미구현 케이스 하나가 이 도구의 테스트 파일로 링크됐다. `IC-T2`와 같은 답을 쓴다 |
| **[IC-M8](../tools/checkcases/main_test.go) ✅** | 케이스 표가 `external`로 적은 접두어의 행을 가짐 | **케이스로 읽지 않는다.** 테스트가 다른 리포에 있는 행을 읽으면 남의 리포에 있는 테스트를 「없다」고 막는다. 목록에서 빼지 않고 `external`로 적어서, 왜 재지 않는지를 설정에 남긴다 |
| [IC-M9](../tools/checkcases/main_test.go) ✅ | `docs/`의 표와 테스트 바로 옆의 표 | 링크가 **그 문서에서 본 상대 경로**다. 한 가지로 적으면 한쪽이 깨진다 |
| **[IC-M10](../tools/checkcases/main_test.go) ✅** | 케이스 표에 `owned`에도 `external`에도 없는 접두어의 행이 있음 | **막는다.** 예전에는 아무 표시 없이 건너뛰었다. 그러면 새 접두어가 들어와도 아무도 모르고, 그 행들은 처음부터 관문 밖에 있다 |
| **[IC-M11](../tools/checkcases/main_test.go) ✅** | `docs.tsv`의 잘못된 행(칸 수, 리포 밖이나 정규형이 아닌 경로, 같은 경로 둘, 엔진이 모르는 접두어, 빈 항목·중복 항목, 아무 접두어도 다루지 않는 행, 빈 설정, 없는 파일) | **고쳐 읽지 않고 막는다.** 설정이 틀렸는데 관문이 통과하면, 그 통과는 아무것도 재지 않은 결과일 수 있다 |
| **[IC-M12](../tools/checkcases/main_test.go) ✅** | 한 접두어를 `owned`와 `external`에 함께 적음(한 행 안이든 두 행에 걸쳐서든) | **막는다.** 이 리포가 재면서 남도 잰다는 말은 둘 중 하나가 틀렸다는 뜻이다. 두 문서가 같은 접두어를 `external`로 적는 것은 된다 |
| [IC-M13](../tools/checkcases/main_test.go) ✅ | `docs.tsv`가 `tools/checkcases/`에 있음 | 경로를 **리포 루트 기준**으로 읽는다. 설정 파일 자리에서 이어 붙이면 문서를 못 찾는다. 링크도 그 경로에서 계산한다 |
| **[IC-M14](../tools/checkcases/main_test.go) ✅** | 축약한 번호의 행(`CP-TOKEN-4·5·6` 꼴)에서 링크를 벗기고 `-write` | **링크를 다시 찍는다.** 관문은 링크를 첫 번호로 걸고 `-write`는 행의 글자 전체로 테스트를 찾아서, 축약한 행만 복구되지 않았다. 그 행만 손으로 붙여야 했다 |
| **[IC-M15](../tools/checkcases/main_test.go) ✅** | 번호 칸이 `[번호]`처럼 괄호만 씌워져 있고 링크가 없음 | **아직 링크되지 않은 케이스 행으로 읽는다.** 못 읽으면 그 행이 아무 표시 없이 관문 밖으로 나가 테스트 없는 ✅ 행도 막히지 않는다. `-write`가 링크를 붙인다 |

> **막는 것은 셋이다.** ✅ 인데 테스트에 번호가 없는 것, 테스트에 있는데 문서에 없는 것,
> 그리고 ⏳·🔜 인데 테스트가 있는 것(표시가 낡았다는 뜻이다).
> 여기에 더해 설정 자체가 틀린 것(IC-M11·M12)과 어느 쪽으로도 분류하지 않은 접두어의
> 행(IC-M10)도 막는다.

### V. 선언 형식과 자체 검사 (`pkg/inventory/decl`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-D9](../pkg/inventory/decl/decl_test.go) ✅** | IP 없는 노드 / 스코프에만 있고 IP 표에 없는 노드 | **둘 다 짚고 결과를 같게 알려 준다**. IP가 노드로 이어지지 않으면 선언 엣지는 미관측으로, 관측 엣지는 UNDECLARED로 구분된다 |
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
| [IC-UI12](../pkg/inventory/ui/ui_test.go) ✅ | `/static/` 요청 | **바이너리에서** 나온다. CDN이면 망이 끊긴 기계에서 화면이 깨지고, 남의 서버 스크립트는 라이선스 관문이 볼 수조차 없다 |
| [IC-UI13](../pkg/inventory/ui/ui_test.go) ✅ | 그려진 화면 | 그 둘을 실제로 부른다. 적어만 두고 안 부르면 아무 일도 일어나지 않는다 |
| **[IC-UI14](../pkg/inventory/ui/ui_test.go) ✅** | 규칙 표를 그리고 다시 읽음 | 같은 규칙이 나오고 **어느 파일의 어느 계층인지**가 남는다. 잃으면 저장할 곳을 잃는다 |
| **[IC-UI15](../pkg/inventory/ui/ui_test.go) ✅** | 세 칸이 모두 빈 줄에 `action`만 exclude | 규칙으로 만들지 않는다. 그대로 두면 `exclude,*,*,*`가 되어 **인벤토리가 통째로 빈다.** `*`를 적으면 규칙이 된다 |
| [IC-UI16](../pkg/inventory/ui/ui_test.go) ✅ | 세 칸을 비움 | 그 규칙이 지워진다. 화면에서 규칙을 뺄 유일한 방법이다 |
| [IC-UI17](../pkg/inventory/ui/ui_test.go) ✅ | 규칙 「행 추가」 | 화면과 같은 폼 이름, 다음 번호로 오른 버튼, **기본은 include** (exclude가 기본이면 실수 한 번이 인벤토리를 지운다) |
| [IC-UI18](../pkg/inventory/ui/ui_test.go) ✅ | 계층 파일 없이 스코프 화면 | 편집 표를 그리지 않는다. **저장할 곳이 없는 칸을 사람이 채우게 하지 않는다** |
| **[IC-UI19](../pkg/inventory/ui/ui_test.go) ✅** | 화면 넷을 통째로 영어로 그림 | **한글이 한 글자도 없다.** 문구 하나를 안 옮기면 그 자리만 한국어로 뜨는데, 문구가 200개라 눈으로는 못 찾는다. 말 바꾸기 토글만 예외다 |
| [IC-UI20](../pkg/inventory/ui/ui_test.go) ✅ | 같은 화면을 두 말로 | 둘 다 그려진다. 한쪽만 되면 토글이 장식이 된다 |
| **[IC-UI21](../pkg/inventory/ui/i18n_test.go) ✅** | `?lang` · 쿠키 · `Accept-Language`가 다 있음 | **고른 것이 브라우저에 우선한다.** 브라우저가 우선하면 토글을 눌러도 다음 화면에서 되돌아간다 |
| [IC-UI22](../pkg/inventory/ui/i18n_test.go) ✅ | 말을 바꿈 | 보던 자리와 알림이 그대로 남는다. 첫 화면으로 되돌리면 아무도 안 바꾼다 |
| [IC-UI23](../pkg/inventory/ui/i18n_test.go) ✅ | 토글 이름표 | **가려는 쪽의 말로** 적는다. 지금 말을 못 읽는 사람도 자기 말은 알아본다 |
| **[IC-UI24](../pkg/inventory/ui/ui_test.go) ✅** | 걸러 낸 결과의 「몇 개 중 몇 개」를 두 말로 | **숫자가 뒤집히지 않는다.** 한국어는 「전체 중 몇 개」, 영어는 「몇 개 of 전체」로 어순이 다르다. 자리를 번호로 고정하지 않으면 한쪽 말에서만 바뀌어 뜨고, 보는 사람은 **걸러 낸 것을 전부로 읽는다** |
| **[IC-UI26](../pkg/inventory/ui/ui_test.go) ✅** | 스코프에만 있는 노드 · 주소 표에만 있는 노드 | **한 표로 합쳐 보여 준다**. 스코프 순서가 앞이고, IP를 모르는 노드는 IP 칸이 빈 줄이다 |
| **[IC-UI29](../pkg/inventory/ui/ui_test.go) ✅** | 화면을 그림 | 설명이 **「도움말」로 접혀 있다**. 날마다 여는 자리지 읽는 자리가 아니다. 지우지 않고 접어 두어, 필요한 사람만 편다 |
| **[IC-UI27](../pkg/inventory/ui/ui_test.go) ✅** | 합친 표를 저장한다. IP를 비운 줄이 섞여 있다 | **IP를 적은 줄만 관리 대상**이 된다. 이을 근거가 없는 이름을 관리 대상에 두면 대조 결과가 오류 없이 틀린다. **뺀 것은 알림에 적는다**. 표에서 사라진 것만 보이면 지워진 것으로 읽힌다 |
| **[IC-UI39](../pkg/inventory/ui/ui_test.go) ✅** | 리뷰 큐의 CNG 항목 | **「플랫폼 조치」**가 붙는다. 계획의 provider 칸이 비는 자리라, 말하지 않으면 빠뜨린 것으로 읽힌다 |
| **[IC-UI40](../pkg/inventory/ui/ui_test.go) ✅** | 자동통과 항목의 계획 칸(포함·종류·목표·수준·되돌림 메모)을 폼으로 보냄 · 결론 칸도 함께 보냄 | 계획 칸은 **세션의 자동통과 항목에 얹히고**, 결론은 얹히지 않는다. 리뷰 항목과 같은 이름의 칸이다 | 두 벌로 만들면 한쪽만 고쳐지는 날이 오고, 그날 한쪽 항목은 계획에 넣을 수 없는 칸을 든다 |
| **[IC-UI42](../pkg/inventory/ui/ui_test.go) ✅** | 바꾸는 양식(선언 · 스코프 · 판정과 그 새 화면) | 모두 CSRF 토큰 필드를 싣는다. **값이 비어도 필드는 그린다.** 비었다고 빼면 토큰을 채우지 않은 실수가 화면에서 감춰진다 |
| **[IC-UI43](../inventory/cmd/pqcaton-ui/tables_test.go) ✅** | 여섯 화면 · 두 말의 표 | **모든 표가 가로 스크롤 상자(`.table-wrap`) 안에 있다.** 좁은 화면에서 표만 옆으로 밀리고 페이지 전체는 넘치지 않는다. 650px 이하의 규칙이 스타일에 있고, 단추를 양식에 담은 상자도 그 양식을 한 줄 전체로 두어 글 칸이 찌그러지지 않는다. 05의 활성화 단계 목록은 단계 카드의 격자를 물려받지 않는다 |
| **[IC-UI44](../inventory/cmd/pqcaton-ui/tables_test.go) ✅** | 선언 · 스코프의 편집 표와 「행 추가」 조각 | **모든 칸에 칸 이름(`data-label`)과 역할(`row` · `cell`)이 있다.** 좁은 화면에서 행을 카드로 접으면 머리글이 보이지 않고, `display`를 바꾸면 브라우저가 표의 의미를 지우기 때문이다 |
| **[IC-UI45](../inventory/cmd/pqcaton-ui/a11y_test.go) ✅** | 여섯 화면 · 두 말과 「행 추가」 조각의 입력 · 선택 · 여러 줄 입력 | **이름 없는 컨트롤이 하나도 없다.** 표 칸의 입력은 「노드 2 IP」처럼 몇 번째 무엇의 어느 칸인지를 이름으로 단다. 머리글은 표 위에 있어서 화면 읽기 프로그램에는 칸 이름이 들리지 않는다 |
| **[IC-UI46](../inventory/cmd/pqcaton-ui/a11y_test.go) ✅** | 다섯 화면 | 첫 자리가 「본문으로 건너뛰기」이고 본문은 `<main id="main">` 하나다. 지금 단계 카드에만 `aria-current="step"`, 조회 탭에 `aria-current="page"`가 붙는다 |
| [IC-UI47](../inventory/cmd/pqcaton-ui/a11y_test.go) ✅ | 저장 성공 · 막힌 이유 · 거절 페이지 | 성공은 `role="status"`, 막힌 이유와 거절은 `role="alert"`다 |
| **[IC-UI48](../inventory/cmd/pqcaton-ui/a11y_test.go) ✅** | 정적 파일과 다섯 화면 | `app.js`(행 추가 · 제거 뒤 포커스)가 바이너리에 담겨 나가고 새 껍데기가 싣는다. 양수 `tabindex`가 없고 스타일이 포커스 표시를 지우지 않는다. 실제 키보드 이동은 브라우저 검증 보고서에 남긴다 |
| **[IC-UI49](../pkg/inventory/ui/notice_test.go) ✅** | 알림 · 거절 · 오류 · 계획 칸 사유의 문구 목록 | 코드마다 두 말이 있고, 영어에 한글이 없고, 두 말의 자리표시자 수가 인자 종류 수와 같다. `decision.Plan*` 사유마다 문구가 있다 |
| [IC-UI50](../pkg/inventory/ui/notice_test.go) ✅ | 쉼표 · 물결 · 퍼센트 · 한글이 든 인자, 인자 수 · 종류 · 역할이 맞지 않는 알림 | 인코딩했다 되돌려도 같다. 맞지 않으면 「알 수 없는 알림」이다 |
| [IC-UI51](../pkg/inventory/ui/notice_test.go) ✅ | 선언 문제 코드 아홉 가지를 두 말로 | 코드마다 문장 · 그대로 두면 · 칸이 있고 값 자리나 강조 표시가 남지 않는다. 자리 표기는 그 말의 행 이름으로 옮긴다 |
| **[IC-UI52](../pkg/inventory/ui/guide_test.go) ✅** | 운영자 안내서 두 판 · 화면 문구가 가리키는 안내서의 절 | 절의 번호와 개수, 명령 블록과 실행 표시의 차례가 같고, 오류 표의 코드가 화면의 오류 코드 전부와 같다. 화면 문구가 가리키는 절은 그 판에 실제로 있다 |
| **[IC-UI53](../pkg/inventory/ui/plural_test.go) ✅** | 수가 들어가는 영어 문구 전부를 1과 2로 | 1이면 명사와 바로 뒤의 동사가 단수다(「1 nodes」 · 「1 … are」가 없다). 2면 그대로다. 수를 채우는 길은 `T.F` 하나다 |
| **[IC-UI54](../pkg/inventory/ui/ui_test.go) ✅** | 설치가 둘인 항목과 하나인 항목으로 판정 화면 · 사람이 본 목록의 표지 · 다른 표지 · 「설치마다 다름」을 보냄 | 둘인 항목에만 경로 목록 · 관측 출처와 두 선택이 보이고, 같은 경로를 원천 둘이 본 항목에는 없다. **본 목록의 표지일 때만 확인으로 받고**, 다른 표지는 받지 않는다 |
| **[IC-UI57](../pkg/inventory/ui/plan_scope_test.go) ✅** | 정책 쪽 거절 사유 여섯을 두 말로, 서명 없음 · 서명 있음 | 사유마다 문장이 있고 **지금 계획(plan.json)의 문제로 적는다**(서명된 계획의 문제로 적지 않는다). 서명 · 생성 양식이 없고 서명된 계획이 있을 때만 보관 양식이 있다 |
| **[IC-UI58](../pkg/inventory/ui/scope_round_test.go) ✅** | 정책 회차로 열린 05를 두 말로: 근거가 운영자 진술 · 비어 있음 · pqcota 이력 대조 · 정책 회차가 아닌 세션 | **운영자 진술이거나 근거가 비어 있으면(v0.27.x 이하의 과거 기록) 「pqcota 이력과 대조하지 않았다」를 보인다.** pqcota 이력 대조면 「확인 시점에 pqcota 이력과 일치했다」를 보이고, 지금도 일치한다거나 적재를 증명한다고 말하지 않는다. 두 정책 지문이 함께 보인다. 정책 회차가 아닌 세션에는 없다 |
| **[IC-UI59](../pkg/inventory/ui/survey_activation_test.go) ✅** | 04의 활성화 안내를 두 말로 · 활성화할 것이 없음 | 이 대조가 활성 정책으로 한 것이라는 문장, 두 지문, 05로 가는 링크를 보인다. 활성화 양식은 없다. 활성화할 것이 없으면 안내도 없다 |
| **[IC-UI60](../pkg/inventory/ui/scope_next_test.go) ✅** | 02의 다음 단계 · 05의 활성화 상자 · 04의 단추를 두 말로 | 02는 「아직 대조에 반영되지 않았다」와 두 링크를 04 먼저 보이고, 다음 단계가 없으면 없다. 05의 세 단계(pqcota 재적재 · pqcota 재적재 결과 대조 · 정책 활성화)는 의미 있는 `<ol>` 안에 순서대로 있고 운영자 확인 칸이 없으며, 첫 단계에 작업 폴더 기준 상대 경로의 명령과 그 안내, 둘째에 대조 설명, 셋째에 단추가 있다. 04의 단추는 「활성화 절차 보기」다 |
| **[IC-UI41](../pkg/inventory/ui/ui_test.go) ✅** | 같은 자산에 판정 행과 계획 선택 행이 있는 원장을 조회 화면에 얹음 | 이력에 **두 행이 다 보이되** 계획 선택 행은 「계획에 넣기로 한 기록」으로 서고 판정으로 보이지 않는다 | 결론 칸이 빈 것을 판정으로 읽으면 사람이 판정을 안 적은 줄 안다 |
| **[IC-UI37](../pkg/inventory/ui/ui_test.go) ✅** | 「관측 이름」을 적고 저장 | 선언에 남고 다시 그려도 그대로다. collector가 자기 id로 보내는 흔한 자리를 사람이 한 번 적어 두는 곳이다 |
| **[IC-UI38](../pkg/inventory/ui/ui_test.go) ✅** | 어느 노드에도 안 붙은 관측 이름 | **「관측 이름」 칸에 후보로 뜬다**. 안 붙었다는 것은 그 노드의 자산이 통째로 UNDECLARED로 오른다는 뜻이다 |
| **[IC-UI35](../pkg/inventory/ui/ui_test.go) ✅** | 관측이 있는 노드의 컴포넌트 칸 | **관측된 이름이 후보로 뜬다**. 그것이 대조가 맞다고 보는 이름이다. 같은 것이 여러 번 관측돼도 후보는 하나다 |
| **[IC-UI36](../pkg/inventory/ui/ui_test.go) ✅** | 관측이 없는 노드 | 후보 목록을 만들지 않는다. 빈 목록을 매달면 고르는 칸처럼 보이면서 아무것도 뜨지 않는다 |
| **[IC-UI33](../pkg/inventory/ui/ui_test.go) ✅** | 자산의 런타임 칸 | **목록에서 고른다**. 관측 결과에 나올 수 있는 이름만 들어 있다. 파일에 있던 낯선 이름은 목록에 더해 고른 채로 남긴다. 화면이 아무 표시 없이 바꿔 쓰면 선언이 사람 몰래 달라진다 |
| **[IC-UI34](../pkg/inventory/ui/ui_test.go) ✅** | 자산의 컴포넌트 칸 | 맞대는 방식이 화면에 있다. **글자 그대로 같아야** 하고, `.so` 뒤는 떼고 적으며, 벤더링 해시는 떼지 않는다. 모르면 오류 없이 미관측·UNDECLARED로 구분된다 |
| **[IC-UI31](../pkg/inventory/ui/ui_test.go) ✅** | 「제거」로 가운데 줄을 지운 폼 | **뒤의 줄이 살아 남는다**. 번호가 끊긴 자리에서 읽기를 멈추면 지운 줄 뒤가 통째로, 오류 없이 저장되지 않는다 |
| **[IC-UI32](../pkg/inventory/ui/ui_test.go) ✅** | 제거·다시 불러오기 | **묻고 지운다**(hx-confirm). 물음에 「저장해야 파일에 반영된다」까지 적혀 있다. 화면에서 지운 것과 파일에서 지운 것은 다르다 |
| **[IC-UI30](../pkg/inventory/ui/ui_test.go) ✅** | 손으로 고친 파일의 IP 없는 스코프 이름 | 표에는 올린다. 저장하면 빠지지만, 화면에서 지우면 IP를 채워 넣을 자리조차 없다 |
| **[IC-UI25](../pkg/inventory/ui/ui_test.go) ✅** | 노드 · 런타임 · 컴포넌트 · 상태로 찾기 | 자유 문자열이 **아무 칸에나** 걸리고, 상태 조건과 함께 주면 둘 다 걸린다. 어느 칸인지 미리 고르게 하면 **무엇을 찾는지 모를 때 여는 화면**에서 그 칸을 알아야 한다 |

### CV. 수집기 판 확인 (`pkg/inventory/collectorver`) ✅

수집기를 실행하지 않고 Go 빌드 정보와 JAR의 manifest · `pom.properties`만 읽는다. 픽스처는 리포에 바이너리를 넣지 않고 시험
중에 상류와 같은 모양(의존 모듈을 옆 폴더 replace로 가리키고 태그를 단 git 체크아웃)으로 빌드한다.

| TC | Given → When | Then |
|---|---|---|
| **[IC-CV1](../pkg/inventory/collectorver/collectorver_test.go) ✅** | 깨끗한 태그 체크아웃에서 빌드(의존은 옆 폴더 replace) | **확인 불가(replace 의존)**. 수집기 자신의 판 · 커밋은 읽히지만 replace한 의존의 실제 소스는 알 수 없다. 지금의 상류 빌드가 모두 이렇다 |
| **[IC-CV2](../pkg/inventory/collectorver/collectorver_test.go) ✅** | 옛 판(v0.10.6) 태그에서 빌드 | **맞지 않음**. v0.26.2 때의 옛 `pqcota-jvmscan` 위험을 막는다 |
| **[IC-CV3](../pkg/inventory/collectorver/collectorver_test.go) ✅** | `-buildvcs=false` · 고친 트리 · 태그 뒤 커밋 | 모두 **확인 불가**(devel · dirty · 의사 판). 판이 찍혀 있어도 그대로 믿지 않는다 |
| **[IC-CV4](../pkg/inventory/collectorver/collectorver_test.go) ✅** | 상류 데모처럼 다른 모듈에서 replace로 끌어와 빌드 | 선언된 판이 맞아도 **확인 불가**(로컬 replace), 선언된 판은 보인다. 로컬 replace가 옛 소스여도 통과시키지 않는다 |
| [IC-CV5](../pkg/inventory/collectorver/collectorver_test.go) ✅ | Windows 교차 빌드(PE) | 같은 빌드 정보로 읽는다(의존이 replace라 리눅스와 같이 확인 불가). Windows 수집기도 같은 기준으로 본다 |
| [IC-CV6](../pkg/inventory/collectorver/collectorver_test.go) ✅ | 빌드 정보 없는 파일 · 다른 모듈 · 고정 판을 모르는 화면 | **확인 불가**. 읽을 수 없는 것을 통과로 세지 않는다 |
| **[IC-CV7](../pkg/inventory/collectorver/collectorver_test.go) ✅** | `collector.jar`: 지금 상류처럼 판 없음 · `Implementation-Version` · `pom.properties` · 다른 판 · 깨진 파일 | 판 없음은 확인 불가(판 없음), 같은 판은 확인됨, 다른 판은 맞지 않음, 깨진 파일은 확인 불가. JAR은 실행하지 않고 판을 읽는다 |
| [IC-CV8](../pkg/inventory/collectorver/collectorver_test.go) ✅ | 수집기끼리 다른 정식 판 | 섞였다고 알린다. 하나하나가 맞아 보여도 섞인 묶음은 막는다 |
| **[IC-CV9](../pkg/inventory/collectorver/collectorver_test.go) ✅** | 옆 `pqcota-common`을 v0.10.6으로 바꿔 빌드 | **결코 확인됨이 되지 않는다**(확인 불가, replace 의존). 빌드 정보에 실제 소스 판이 드러나지 않는다는 실물의 한계를 시험으로 고정한다 |
| **[IC-CV10](../pkg/inventory/collectorver/collectorver_test.go) ✅** | replace 없이 의존을 그 판으로 받아 태그에서 빌드 | **확인됨**. 상류가 그런 릴리스 빌드를 내면 override 없이 통과한다 |

### R2. 여러 노드 대조 (`pkg/inventory/report`) ✅
| TC | Given → When | Then |
|---|---|---|
| **[IC-R8](../pkg/inventory/decl/decl_test.go) ✅** | 관측 IP → 스코프 노드 잇기(포트 붙은 주소 · 망 둘에 걸친 노드 · 스코프 밖 · 이미 이어진 것) | 맞는 것만 잇고 나머지는 그대로 둔다. **잘못 이으면 CONFIRMED여야 할 엣지가 UNDECLARED로 올라온다**(그럴듯한 오답) |
| [IC-R9](../pkg/inventory/report/report_test.go) ✅ | NETWORK 계층이 커버 vs 강등 | 강등을 커버로 세지 않는다. 세면 **못 본 노드가 「봤다」가 되어** 토폴로지 점선이 실선이 된다 |
| [IC-R10](../pkg/inventory/report/report_test.go) ✅ | 한 노드를 collector 둘이 봄 | 중복은 지우되 **처음 순서를 지키고 입력을 덮지 않는다** |
| [IC-R11](../pkg/inventory/report/report_test.go) ✅ | 깨진 결과 파일 | 나머지는 읽되 **건너뛴 것을 이름으로 알려 준다**. 모르면 「관측 안 됨」과 「못 읽음」이 뒤섞인다 |
| [IC-R12](../pkg/inventory/report/report_test.go) ✅ | 관측 결과가 하나도 없음 | 선언만으로 대조가 돌고 **전부 미관측**이 된다. 그것이 「없다」가 아니라 「아직 못 봤다」다 |
| **[IC-R17](../pkg/inventory/report/report_test.go) ✅** | **JSON Lines 결과(`*.jsonl`, JVM 수집기)와 단일 객체(`*.json`)가 한 디렉터리에** | 셋 다 읽힌다. 건너뛴 것이 없다. `*.json`만 고르면 그 노드의 JCA 자산이 아무것도 실패하지 않은 채 「관측 안 됨」이 된다. 실제로 그랬고 종단 데모에서야 드러났다. 상류의 공식 디코더(`resultio.LoadDir`)를 쓴다 |
| **[IC-R18](../pkg/inventory/report/provenance_test.go) ✅** | **같은 결과 디렉터리를 상류 `ingest.IngestWith`(메모리 이력)와 이 리포의 대조가 각각 읽음** · 결과 파일 순서를 섞음 | 근거의 스냅샷 지문이 **상류가 저장한 스냅샷의 v1 지문과 같고**, 상류 이력이 그 지문으로 찾는다. 원천 노드는 봉투의 이름, 규칙 판은 상류 것. 순서를 섞어도 같다 | 되짚기의 전제다. 전에는 결과 하나마다 정규화하고 네트워크 결과를 비켜 두어 어떤 경우에도 같은 지문이 나올 수 없었다. 이제 원천 노드별로 모아 상류와 같은 함수로 정규화한다 |
| **[IC-R19](../pkg/inventory/report/provenance_test.go) ✅** | 원천 노드 둘이 `observed_as`로 선언 노드 하나에 걸리고 같은 자산을 봄. 파일 순서는 약한 증거가 앞 | 근거가 **둘 다** 실리고, 주 근거는 **가장 강한 증거**(`confirmed`)이며 `FindingID`가 `Sources[0]`과 같다 | 전에는 같은 자산의 첫 관측만 남아 둘째 원천의 근거가 사라졌고, 주 근거가 입력 순서에 달렸다 |
| **[IC-R16](../pkg/inventory/reconcile/reconcile_test.go) ✅** | CNG 관측(상류 v0.6.0) | **자산이 된다**. 런타임 `cng` · 컴포넌트 `cng-providers`. 갈래를 안 더하면 Windows 노드의 암호 자산이 인벤토리에서 통째로 사라진다. **모르는 런타임은 그대로 버린다**. 이름을 지어내면 선언과 영영 맞지 않는 자산이 생긴다 |
| **[IC-R15](../pkg/inventory/report/report_test.go) ✅** | 관측 이름이 겹치거나 이름과 부딪힘 | **이름이 이기고, 겹친 관측 이름은 먼저 적힌 쪽이 가진다**. 뒤에 적힌 것으로 뒤집히면 파일 순서만 바뀌어도 자산이 다른 노드에 붙는다 |
| **[IC-R14](../pkg/inventory/report/report_test.go) ✅** | 관측 노드 id가 선언 이름과 다름 | **선언 노드로 잇는다**. 호스트명(짧은 이름 포함)이 같으면 알아서, 아니면 적어 둔 「관측 이름」으로. 대소문자는 가리지 않는다. **어디에도 안 걸리면 관측이 부른 이름을 그대로 둔다**. 억지로 고르면 남의 노드 자산이 붙는다 |
| **[IC-R13](../pkg/inventory/report/report_test.go) ✅** | 못 본 계층을 화면·콘솔에 보여 줌 | 상류 enum 상수(`COLLECTION_LAYER_ARTIFACT`)를 그대로 보여 주지 않고 **관측이 어디서 오는지**를 적되 원래 이름을 괄호에 남긴다. **모르는 값은 바꾸지 않고 그대로 보여 준다**. 상류에 계층이 늘었을 때 뭉개면 못 본 것이 화면에서 사라진다 |

> **구현 위치**: 이 리포에는 엣지 대조 `pkg/inventory/reconcile/edge.go`와 토폴로지 DOT `pkg/inventory/reconcile/topology.go`가 있습니다. 등급 분류 pqcota-common의 `pkg/kernel/posture/`, 저장 pqcota-inventory의 `pkg/inventory/history`(Snapshot.Edges, Postgres `edges` JSONB), 관측 엣지 스키마 pqcota-common 계약의 `ObservedEdge`(CollectionResult.observed_edges)는 pqcota에 있습니다. 이 계약을 채우는 **network-collector(디스커버리 설계 §2.3, AF_PACKET)가 라이브 관측을 공급합니다**(대조 엔진은 합성 데이터로도 검증됩니다).

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
| 9 | **행 수준 보안**(핸들 격리가 우회돼도 DB가 막음) | L1~4 | ✅ integration |
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
| 20 | **문체 관문**(한 번 없앤 말이 다시 들어오는가) | 상류 pqcota의 케이스. 이 리포는 설정만 | ✅ |
| 21 | **케이스 관문**(번호와 테스트가 실제로 대응하는가) | M1~9 | ✅ |

**핵심 인수 기준**: **IC-P4**(판정이 끝나지 않으면 계획을 넘기지 않는다. 반드시 거쳐야 하는 관문)와 **IC-F3~F5**(판정자 서명·전 필수 판정 없으면 확정 불가), 그리고 **IC-P8**(판정이 끝난 계획은 `IN_REVIEW`로 나가고 승인 칸은 상류가 채운다).

## 3. 데이터 모델 매핑 (구현 위치)

| TC 그룹 | 구현 위치 |
|---|---|
| 대조·큐·confidence | `pkg/inventory/reconcile/` |
| 상태기계·확정 계획·관문 | `pkg/inventory/decision/` |
| 판정 영속화 | `pkg/inventory/decision/` + pqcota-inventory `pkg/inventory/history`의 PgStore 패턴 |
| 엣지·토폴로지 | `pkg/inventory/reconcile/{edge,topology}.go` |
| Decision·FinalizedPlan 스키마 | pqcota `contracts/` (공개 스키마) |
