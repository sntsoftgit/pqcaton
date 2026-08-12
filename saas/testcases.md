# 컨트롤 플레인 테스트케이스 명세

[러너 설계](design.md)를 **검증 가능한 인수 기준**으로 옮긴 것입니다. 구현은 이 테스트를
통과하는 것을 목표로 합니다.

무엇부터 만드는지는 [implementation.md](implementation.md)에 있고, 이 문서는 그 각
마일스톤이 **무엇으로 끝났다고 말할 수 있는지**를 정합니다.

> **§ 표기**: 별도 언급이 없으면 [러너 설계](design.md)의 절 번호입니다.

---

## 0. 실행 환경

**케이스는 대부분 unit입니다** — 실물 없이 어디서나 돕니다. 예외는 `CP-PG-*`와 `CP-ORG-1`로,
`PQCATON_TEST_DSN`이 있으면 실 Postgres로 돌고 없으면 스킵합니다. **스킵은 통과가 아닙니다.**

**`CP-ORG-1`이 스킵되면 조직 격리를 확인하지 못한 것입니다.** 인메모리 케이스는 저장소
객체가 애초에 달라서, 통과해도 격리를 증명하지 않습니다 — **한 테이블을 공유하는 쪽에서만**
잴 수 있습니다.

케이스 번호는 **`CP`(Control Plane) - 무엇을 보나 - 순번**입니다 — `CP-TOKEN`(러너 토큰) ·
`CP-KEY`(collector 공개키 등록소) · `CP-RUNNER`(러너 등록) · `CP-ORG`(조직 격리) ·
`CP-PG`(실 Postgres). 번호는 그것을 검증하는 **테스트 파일로 이어집니다.**

## 1. M0 — 조직·토큰 경계

### CP-TOKEN. 러너 토큰 (§6.4.1)

| 케이스 | Given → When | Then | 목적 |
|---|---|---|---|
| [CP-TOKEN-1](internal/access/access_test.go) | `TestTokenShapeAndNoPlaintextStored` — 토큰 발급 | 접두어 `pqcrt_`, 조회키 8자·비밀 32자, 저장되는 것은 **SHA-256 32바이트뿐** | 평문을 되돌릴 수 없어야 유출 경로가 하나 줄어듭니다. 접두어는 로그·시크릿 스캐너에 걸리라고 붙입니다 |
| [CP-TOKEN-2](internal/access/access_test.go) | `TestAuthenticateDerivesOrg` — 유효한 토큰으로 인증 | 그 토큰의 **조직**이 나온다 | **조직은 여기서만 나옵니다.** 요청 본문의 주장을 보지 않는다는 것이 이 제품의 격리 전제입니다(§6.4) |
| [CP-TOKEN-3](internal/access/access_test.go) | `TestMalformedTokenNeverReachesStore` — 빈 문자열·접두어 없음·길이 틀림 | 조회하지 않고 `ErrMalformed` | 아무 문자열이나 던지는 쪽에 저장소 비용을 주지 않습니다 |
| [CP-TOKEN-4·5·6](internal/access/access_test.go) | `TestAuthenticateDistinguishesRejections` — 모르는 조회키 / 비밀 불일치 / 폐기된 토큰 | 각각 `ErrUnknownToken`·`ErrSecret`·`ErrRevoked`. 폐기는 **비밀이 맞아도** 거절 | 폐기된 토큰을 계속 쓰는 러너와 아무 토큰이나 넣어 보는 쪽은 다른 일입니다. 기록에서 갈라야 무엇에 대응할지 정해집니다(응답은 어느 쪽이든 같습니다) |
| [CP-TOKEN-7](internal/access/access_test.go) | `TestAuthenticateRecordsLastUsed` — 인증 성공 | `last_used_at` 갱신 | 만료를 두지 않는 대신 이것으로 안 쓰이는 토큰을 찾아 거둡니다 |
| [CP-TOKEN-8](internal/access/access_test.go) | `TestTokensAreDistinct` — 64회 발급 | 조회키·평문이 겹치지 않는다 | 난수 경로가 상수를 물고 있으면 여기서 드러납니다 |
| [CP-TOKEN-9](internal/access/access_test.go) | `TestTokenWithoutOrgIsRejected` — 조직 없이 저장 | `org.ErrEmpty` | 빈 조직을 품는 경로가 하나라도 있으면 그 경로로 데이터가 섞입니다 |

### CP-KEY. collector 공개키 등록소 (§6.4.2)

| 케이스 | Given → When | Then | 목적 |
|---|---|---|---|
| [CP-KEY-1·2](internal/access/access_test.go) | `TestActiveKeysAllowsRotationWindow` — 같은 `(조직, collector)`에 키 둘 등록 | **둘 다 유효** | 키 교체 구간입니다. 이 구간이 없으면 키를 바꾸는 순간 그 조직의 관측이 **전부 거절**됩니다 |
| [CP-KEY-3](internal/access/access_test.go) | `TestRevokedKeyDisappears` — 하나를 폐기 | 나머지만 남는다 | 교체가 끝난 뒤 옛 키를 닫을 수 있어야 교체가 완결됩니다 |
| [CP-KEY-4·5](internal/access/access_test.go) | `TestActiveKeysIsolatesOrgAndCollector` — 다른 조직·다른 collector의 키가 함께 있음 | 그 조직 그 collector의 것만 | **이 목록을 그대로 `sign.Verify`에 넘깁니다.** 여기서 새면 다른 조직의 collector가 서명한 결과가 통과합니다 |
| [CP-KEY-6](internal/access/access_test.go) | `TestActiveKeysRequiresOrg` — 조직 없이 조회 | `org.ErrEmpty` | 조직 조건이 빠진 조회 경로를 두지 않습니다 |

### CP-RUNNER. 러너 등록

| 케이스 | Given → When | Then | 목적 |
|---|---|---|---|
| [CP-RUNNER-1·2](internal/access/access_test.go) | `TestRunnerRegisterAndTouch` — 등록 후 갱신 | 버전·마지막 접속이 갱신되고 **어느 토큰으로 등록했는지가 남는다** | 토큰을 폐기했을 때 **누가 끊기는지** 알아야 폐기가 운영 가능한 조치가 됩니다 |
| [CP-RUNNER-3](internal/access/access_test.go) | `TestTouchUnknownRunner` — 없는 러너 갱신 | `ErrNotFound` | 등록되지 않은 러너의 상태를 만들어 주지 않습니다 |

### CP-ORG · CP-PG. 실 Postgres

| 케이스 | Given → When | Then | 목적 |
|---|---|---|---|
| [**CP-ORG-1**](internal/access/pg_test.go) | `TestPgActiveKeysIsolatesOrg` — **한 테이블에** 두 조직의 키 | 자기 조직 것만 | 인메모리는 객체가 달라 격리를 증명하지 않습니다. **여기서만 잴 수 있습니다** |
| [CP-PG-1](internal/access/pg_test.go) | `TestPgTokenRoundTrip` — 저장 → 인증 → 폐기 | 폐기 뒤에는 통과하지 않는다 | 즉시 폐기가 이 토큰 설계의 존재 이유입니다(§6.4.1) |
| [CP-PG-2](internal/access/pg_test.go) | `TestPgRotationWindowAndRevoke` — 실 테이블에서 키 둘 | 덮어쓰이지 않는다 | 기본키에 `public_key`가 빠지면 두 번째 등록이 첫 번째를 **조용히 덮습니다** |
| [CP-PG-3](internal/access/pg_test.go) | `TestPgRunnerRoundTrip` — 등록·갱신·빈 버전으로 갱신 | 빈 버전이 기존 값을 **지우지 않는다** | `status`가 버전을 안 보내는 경우에 기록이 지워지면 안 됩니다 |
| [CP-PG-4](internal/access/pg_test.go) | `TestPgRefusesMissingSchema` — 테이블이 없는 곳을 가리킴 | `ErrSchemaMissing` | 생성자가 말없이 DDL을 돌면, 가리키는 곳이 어긋났을 때 빈 테이블이 생기고 거기에 씁니다 — 데이터가 사라진 것처럼 보입니다 |

## 2. 아직 없는 것

M1 이후의 케이스입니다. 만들 때 이 문서에 함께 채웁니다.

| 마일스톤 | 검증할 것 |
|---|---|
| **M1** 결과 수신 | 서명 없는 결과 거절 · **서명 형식 불일치를 위조와 다른 사유로** 기록(§6.6) · canonical 해시 멱등 · 본문 크기 초과는 자르지 않고 거절 · 조직이 토큰에서만 유도됨 |
| **M2** 작업 배포 | 롱폴 · 만료·재배포 · `provision`은 `observe`와 다른 재배포 정책 |
| **M3** 등재 | 지문 충돌 보류 · 등재 실패는 과금하지 않음 · 월중 고유 노드 누적 |
| **M4** 관측 | 접속 정보가 컨트롤 플레인에 올라오지 않음 · `target_node_ids`로만 지시 |

## 3. 세는 법

케이스는 **테스트 함수 단위**입니다. 아래 값과 어긋나면 이 표가 틀린 것입니다.

| 레벨 | 수 |
|---|---|
| unit | 13 |
| Postgres 필요 | 5 |
| **전체** | **18** |

```bash
grep -rh '^func Test' --include='*_test.go' saas/ | wc -l    # 전체
grep -c '^func Test' saas/internal/access/pg_test.go          # Postgres 필요
```
