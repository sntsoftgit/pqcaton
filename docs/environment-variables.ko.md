[English](environment-variables.md) · 한국어

# 환경 변수

[개요](../README.ko.md) · [운영자 안내서](operator-guide.ko.md) · [빌드](build.ko.md)

pqcaton이 읽는 환경 변수입니다. 비밀값은 환경 변수(또는 아래에 적은 파일)로만 받고, 작업 폴더 · 처리 기록 · 로그 · 오류 문구 ·
화면에 적지 않습니다.

## 실행

| 변수 | 읽는 쪽 | 뜻 |
|---|---|---|
| `PQCOTA_DSN` | `pqcaton-ui`, `pqcaton-decide baseline` · `activate-scope` | pqcota 이력의 연결 정보입니다. **화면은 이것 없이 시작하지 않습니다.** 비었으면 `pqcaton-ui`가 기본 승인 키 옆의 `pqcota.env`(소유자 전용, `PQCOTA_DSN=<값>` 한 줄)를 읽습니다([운영자 안내서 2절](operator-guide.ko.md#2-설치)). 화면이 띄우는 관측 프로세스에는 넘기지 않습니다 |
| `PQCOTA_ORG` | `pqcota-ingest`(상류) | pqcota가 적재할 조직입니다. 판정 조직과 같아야 05의 대조가 그 적재를 찾습니다 |
| `PQCATON_SCOPE_ASSETS` | `pqcaton-report` | 자산 스코프 정책 파일입니다. `pqcota-ingest -scope-assets`에 준 것과 같은 파일을 줍니다 |
| `PQCATON_REQUIRE_RLS` | Postgres의 판정 원장 | `1`이면 행 수준 보안이 걸리지 않는 연결(슈퍼유저나 `BYPASSRLS` 롤)에서는 원장을 열지 않습니다([설계 §2.2](developers.ko.md#22-행-수준-보안-db가-막는-한-겹)) |
| `PQCATON_RUN` | `pqcaton-ui`가 붙임 | 화면이 띄우는 관측 프로세스에 붙이는 표지입니다. 화면은 이 표지로 자기 것임이 증명된 프로세스만 끝냅니다. 손으로 두지 않습니다 |

06의 승인 키는 환경 변수가 아니라 파일입니다(`approval.key` · `approval.pub`, 또는 `-approval-key` · `-approval-verify-key`). 내용은
`pqcota-keygen`의 출력(`PQCOTA_SIGN_KEY` · `PQCOTA_VERIFY_KEY` 줄)입니다. 상류 명령 `pqcota-approve` · `pqcota-provision`은
`PQCOTA_APPROVAL_KEY` · `PQCOTA_APPROVAL_KEYS`를 스스로 읽습니다.

## 데모와 검증

| 변수 | 읽는 쪽 | 뜻 |
|---|---|---|
| `PQCOTA_DIR` | `make e2e-demo`, 데모 스크립트 | 상류 `pqcota` 체크아웃입니다(`go.mod`가 고정한 판으로 다섯 리포를 나란히) |
| `PQCATON_BIN_DIR` | 데모 스크립트 | Go로 빌드하는 대신 쓸, 미리 빌드한 리눅스 바이너리입니다 |
| `PQCATON_UI_PORT` | `demo/scripts/ui.sh` | 이 기계에서 중계가 받을 포트입니다(기본 8765) |
| `PQCATON_E2E_TRACE` | `demo/scripts/extend.sh` | `1`이면 되짚기 사슬을 재는 전용 픽스처로 돌립니다 |
| `E2E_KEEP_WS` | `make e2e-demo` | 정리하기 전에 작업 폴더를 꺼내 둘 폴더입니다 |
| `E2E_UBUNTU_MIRROR` | `make e2e-demo` | dind 안의 빌드가 쓸 우분투 미러입니다 |
| `PQCOTA_TEST_DSN` | 시험 | Postgres가 필요한 시험이 쓰는 DB입니다. 없으면 그 시험은 건너뜁니다 |
