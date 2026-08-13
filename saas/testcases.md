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

> **실측으로 확인했습니다**(2026-08-12, PostgreSQL 16). `ActiveKeys`의 질의에서 `org` 조건만
> 빼 보면 `CP-ORG-1`은 실패하는데, **같은 것을 보는 인메모리 케이스는 그대로 통과합니다.**
> 위 문단이 주장이 아니라 사실이라는 뜻입니다.

### 통과는 검증이 아닙니다

케이스가 무엇을 재는지는 **일부러 깨서** 확인합니다. 지금까지 확인한 것:

| 무엇을 깼나 | 어느 케이스가 잡았나 |
|---|---|
| `ActiveKeys` 질의에서 `org` 조건 제거 | `CP-ORG-1` (인메모리 케이스는 못 잡음) |
| 검증자가 다른 조직의 키를 조회 | `CP-INTAKE-3` — *"다른 조직 키가 통과했다"* |
| 거절된 것까지 멱등에 표시 | `CP-INTAKE-6` — *"거절이 굳어 다시 들어오지 못했다"* |

`-race`도 같습니다. 단일 고루틴 케이스만 있으면 검출기가 볼 것이 없어 **깨끗한 것이
아무것도 증명하지 않습니다.** `CP-INTAKE-10·11`이 그 자리를 메웁니다.

### 돌리는 법

```bash
docker run -d --name pqcaton-test-pg --label pqcota-test \
  -e POSTGRES_PASSWORD=CHANGEME -e POSTGRES_DB=pqcaton \
  -p 127.0.0.1:55432:5432 postgres:16-alpine

PQCATON_TEST_DSN='postgres://postgres:CHANGEME@127.0.0.1:55432/pqcaton?sslmode=disable' \
  go test ./... -count=1

docker rm -f pqcaton-test-pg
```

**DB는 테스트가 도는 머신에 띄웁니다.** 다른 머신의 Postgres를 가리키면 방화벽에 걸리기
쉽고, 그때 나오는 오류(`no route to host`)가 테스트 실패처럼 보여 시간을 버립니다.

`--label pqcota-test`는 이 머신의 teardown 규약입니다 — 그 라벨만 정리합니다.

표는 매번 `CREATE TABLE IF NOT EXISTS`로 올라가고, 케이스마다 조직 이름을 다르게 두므로
같은 DB를 여러 번 돌려도 서로 밟지 않습니다.

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

## 2. M1 — 결과 수신

검증·멱등·적재(`CP-INTAKE`)와 그 위의 HTTP 경계(`CP-HTTP`)입니다.

### CP-INTAKE. 검증 · 멱등 · 적재 (§5.1 · §6.4.2 · §6.5)

| 케이스 | Given → When | Then | 목적 |
|---|---|---|---|
| [CP-INTAKE-1](internal/intake/intake_test.go) | `TestAcceptsResultSignedByRegisteredKey` — 등록된 키로 서명된 결과 | 통과해서 스냅샷이 쌓인다 | 거절만 시험하면 게이트가 **정상 반입까지 막는 것**을 못 잡습니다 |
| [CP-INTAKE-2](internal/intake/intake_test.go) | `TestRejectsUnsignedResult` — 서명 없는 결과 | 거절 | 여러 조직의 결과가 한 저장소로 모이는 곳에 조용히 통과하는 경로를 두지 않습니다 |
| [CP-INTAKE-3](internal/intake/intake_test.go) | `TestRejectsResultSignedByAnotherOrgsKey` — **다른 조직에 등록된 키**로 서명 | 거절 | 등록소 조회에 조직 조건이 붙는다는 것이 여기서 값을 합니다. 안 걸면 남의 collector가 서명한 결과가 우리 조직으로 들어옵니다 |
| [CP-INTAKE-4](internal/intake/intake_test.go) | `TestResendIsFoldedNotRejected` — 같은 결과 재전송 | **중복으로 세고 받았다고 답한다** | 재전송은 러너의 정상 동작입니다(올린 뒤 응답을 못 받은 경우). 거절로 세면 러너가 고장 난 것처럼 보입니다 |
| [**CP-INTAKE-5**](internal/intake/intake_test.go) | `TestResultsFromSameNodeAndInstantAreDistinct` — 같은 collector·노드·시각인데 **내용이 다른 결과 둘** | 둘 다 통과, 지문도 다르다 | **멱등 키 선택의 이유입니다.** 엔벨로프 삼중키였다면 둘이 같은 키가 되어 정상 결과 하나가 조용히 사라집니다(JVM마다 결과 하나) |
| [**CP-INTAKE-6**](internal/intake/intake_test.go) | `TestRejectedResultCanBeRetriedAfterKeyIsRegistered` — 거절된 뒤 키를 등록하고 같은 결과 재전송 | **들어온다** | 거절된 것까지 「본 것」으로 남기면 그 결과는 영영 못 들어옵니다. 멱등은 재전송을 접는 장치이지 **실패를 굳히는 장치가 아닙니다** |
| [CP-INTAKE-7](internal/intake/intake_test.go) | `TestBothKeysWorkDuringRotation` — 옛 키·새 키로 각각 서명 | 둘 다 통과 | 교체 구간이 적재 경로에서도 성립하는지 — 등록소만 되고 여기서 막히면 소용이 없습니다 |
| [CP-INTAKE-8](internal/intake/intake_test.go) | `TestReceiveRequiresOrg` — 조직 없이 수신 | `ErrNoOrg` | 여기까지 왔는데 조직이 비었으면 인증 경로가 깨진 것입니다. 품지 않습니다 |
| [CP-INTAKE-9](internal/intake/intake_test.go) | `TestSeenStoreIsolatesOrg` — 멱등 저장소에 두 조직의 같은 지문 | 서로 안 보인다 | 여기서 섞이면 한 조직의 결과가 다른 조직에서 **"이미 받았다"로 사라집니다** |
| [CP-INTAKE-10](internal/intake/concurrency_test.go) | `TestConcurrentDistinctResults` — 서로 다른 결과 24개를 동시에 | 전부 들어간다 | 저장소는 요청 사이에서 공유됩니다. 동시성 케이스가 없으면 `-race`가 볼 것이 없어 **깨끗한 것이 아무것도 증명하지 않습니다** |
| [**CP-INTAKE-11**](internal/intake/concurrency_test.go) | `TestConcurrentResendIsCountedOnce` — **같은** 결과를 16개 고루틴이 동시에 | 한 번만 적재, 나머지는 중복 | **실제 버그를 잡은 케이스입니다.** 확인과 표시가 나뉘어 있어 16번 다 적재됐습니다 — 러너의 재시도가 앞 요청과 겹치면 관측 횟수가 부풀려집니다 |

### CP-HTTP. 경계 (§6.2)

| 케이스 | Given → When | Then | 목적 |
|---|---|---|---|
| [CP-HTTP-1](internal/api/api_test.go) | `TestResultsAcceptedAndStoredInTokensOrg` — 유효 토큰 + 서명된 결과 | 200, **그 토큰의 조직**에 쌓인다 | 경계가 실제로 끝까지 이어지는지 |
| [**CP-HTTP-2**](internal/api/api_test.go) | `TestOrgComesFromTokenNotBody` — 본문에 `"org":"beta"`를 적어 보냄 | 무시하고 토큰의 조직에 쌓는다. beta 저장소는 **열리지도 않는다** | **조직은 러너가 주장하는 것이 아닙니다**(§6.4). 핸들러가 본문에서 조직을 읽을 방법을 두지 않는 것이 이 성질을 지킵니다 |
| [CP-HTTP-3](internal/api/api_test.go) | `TestAuthFailuresAreIndistinguishable` — 모르는 토큰 / 비밀 불일치 / 헤더 없음 | 셋 다 401이고 **응답 본문이 같다** | 어느 쪽이 틀렸는지 알려 주면 시도하는 쪽에 정보를 줍니다. 구분은 기록에서만 합니다 |
| [**CP-HTTP-4**](internal/api/api_test.go) | `TestOversizedBodyIsRejectedNotTruncated` — 상한을 넘는 본문 | 413, **아무것도 적재되지 않는다** | 잘라서 받으면 관측 결과가 조용히 훼손되고 "관측했는데 없더라"가 됩니다 |
| [CP-HTTP-5](internal/api/api_test.go) | `TestTrustProxyRejectsPlainRequest` — 앞단 신뢰 켬, `X-Forwarded-Proto` 없음/`https` | 없으면 400, `https`면 통과 | 조용히 받으면 프록시 설정이 풀린 것을 아무도 모릅니다 |
| [CP-HTTP-6](internal/api/api_test.go) | `TestForwardedProtoIgnoredWhenNotTrusted` — 앞단 신뢰 끔, `X-Forwarded-Proto: http` | 무시하고 통과 | 아무나 붙일 수 있는 헤더라, 믿으면 평문 요청이 HTTPS로 둔갑합니다 |
| [CP-HTTP-7](internal/api/api_test.go) | `TestBrokenResultDoesNotDropTheBatch` — 계약에 안 맞는 결과가 섞임 | 나머지는 들어간다 | 하나가 깨졌다고 배치를 버리면 러너가 그것만 골라낼 방법이 없습니다 |

## 3. 아직 없는 것

M1 이후의 케이스입니다. 만들 때 이 문서에 함께 채웁니다.

| 마일스톤 | 검증할 것 |
|---|---|
| **M2** 작업 배포 | 롱폴 · 만료·재배포 · `provision`은 `observe`와 다른 재배포 정책 |
| **M3** 등재 | 지문 충돌 보류 · 등재 실패는 과금하지 않음 · 월중 고유 노드 누적 |
| **M4** 관측 | 접속 정보가 컨트롤 플레인에 올라오지 않음 · `target_node_ids`로만 지시 |

## 4. 세는 법

케이스는 **테스트 함수 단위**입니다. 아래 값과 어긋나면 이 표가 틀린 것입니다.

| 레벨 | 수 |
|---|---|
| unit | 31 |
| Postgres 필요 | 5 |
| **전체** | **36** |

```bash
grep -rh '^func Test' --include='*_test.go' saas/ | wc -l    # 전체
grep -c '^func Test' saas/internal/access/pg_test.go          # Postgres 필요
```
