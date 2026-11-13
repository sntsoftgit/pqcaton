[English](build.md) · 한국어

# 소스에서 빌드하기

[개요](../README.ko.md) · [환경 변수](environment-variables.ko.md) · [기여하기](../CONTRIBUTING.ko.md)

## 필요한 것

- **Go**: [`go.mod`](../go.mod)에 적힌 판입니다. 빌드와 시험에 다른 런타임은 필요 없습니다.
- **Docker**: `make check-reuse`(라이선스 머리말)와 `make e2e-demo`(데모를 처음부터)에만 씁니다.
- 상류 pqcota 모듈은 `go.mod`가 고정한 판으로 Go 모듈 프록시에서 받습니다. 옆 폴더 체크아웃도 `replace`도 필요 없습니다.

## 빌드와 시험

```bash
make
```

`make`는 관문을 먼저 돌리고, 처음 실패한 관문에서 멈춥니다: 라이선스, 코드 속 문구, 문서 문체, 사이트 페이지의 두 말, 문서(짝과
링크), 케이스(번호와 시험), 서식, 모듈 zip, 발행 관문 도구. 그다음 빌드하고 시험을 돌립니다. CI가 같은 명령을 돌립니다.

명령만 빌드하려면:

```bash
go build -o bin/ ./inventory/cmd/...
```

## 생성물

화면 템플릿(`.templ`)은 생성물(`*_templ.go`)과 함께 리포에 두므로, 빌드에 생성기가 필요 없습니다. `.templ`을 고쳤으면 다시 만들고
확인한 뒤, 생성물을 같은 커밋에 넣습니다.

```bash
make generate
make verify-generated
```

## 그 밖의 검사

| 명령 | 무엇을 재나 |
|---|---|
| `make check-reuse` | 모든 파일에 라이선스와 저작권 표기가 있는지(REUSE, Docker에서 돕니다) |
| `make test-pg` | Postgres 계약 시험(IC-S26 · IC-D5 등 `PQCOTA_TEST_DSN`을 읽는 시험). `make test`와 달리 건너뛰지 않습니다: `PQCOTA_TEST_DSN`이 없거나, 시험 하나라도 건너뛰었거나, IC-S26이 돌지 않았거나, 출력에 DSN이 나오면 실패합니다. CI가 PostgreSQL 16 서비스로 돌리므로, 발행 관문이 요구하는 GitHub `ci`의 일부입니다 |
| `make e2e-demo` | pqcota 데모를 처음부터 띄워 정책 활성화 · 판정 · 승인 · 생성을 끝까지 돌립니다(`PQCOTA_DIR` 필요, [데모](../demo/README.ko.md) 참조) |
| `make release-check VERSION=vX.Y.Z` | 릴리스 태그를 달기 전의 모든 것(후보 커밋의 필수 GitHub 워크플로 ci · site가 모두 성공했는지 포함). 요약 · 모듈의 `h1:` 해시 · 릴리스 본문(`release-body.md`: 영어 절에 태그 고정 링크, 맨 위에 한국어 노트의 같은 판 링크)으로 끝납니다 |
| `make release-verify VERSION=vX.Y.Z COMMIT=… H1=…` | 발행 뒤: 태그 · 모듈 프록시 · 체크섬 DB와 거기서 받은 설치, 그리고 릴리스 본문이 영어 절과 같고 한국어 링크가 열리는지 |
