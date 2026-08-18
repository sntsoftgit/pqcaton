# 데모 — pqcota 위에 얹는 거버넌스

[개요](../README.md) · [릴리스 노트](../RELEASE_NOTES.md) · [여정](../docs/journey.md) · [설계](../docs/design.md) · [검증 기준](../docs/testcases.md) · **데모** · [구조 그림](https://www.sntsoft.co.kr/pqcaton/)

📊 **실행 전 예상 결과**: [`expected-output/`](expected-output/) — 확장 리포트·거버넌스 토폴로지 샘플 + 차이점 설명.

이 데모는 **독립 스택이 아니라 확장**입니다. [pqcota의 디스커버리 데모](https://github.com/randyinthedev-hash/pqcota/tree/main/demo)를
그대로 띄운 뒤, 그 위에 이 리포의 기능 — **선언 대비 3-상태 대조(CONFIRMED/UNDECLARED shadow/UNOBSERVED) +
리뷰 큐 + 거버넌스 토폴로지** — 를 얹습니다.

```
관측 등급 (pqcota)              →   + 선언 대비 reconciliation (pqcaton)
🟢 web-gw→pay-app MLKEM                🟢 web-gw→pay-app  TLS  CONFIRMED
🔴 web-gw→pay-db  고전                 🔴 web-gw→pay-db   TLS  CONFIRMED
🟢 web-gw→pay-app SSH sntrup761        🟢 web-gw→pay-app  SSH  UNDECLARED(shadow) ← 선언 안 된 통신!
🔴 web-gw→pay-db  SSH curve25519       🔴 web-gw→pay-db   SSH  UNDECLARED(shadow)
                                       ⚪ pay-db→pay-app  TLS  UNOBSERVED (선언했으나 미관측 ≠ 부재)
```

## 실행

```bash
# 1) OSS 디스커버리 데모를 먼저 띄운다 (환경 + 수집)
../../pqcota/demo/scripts/up.sh
../../pqcota/demo/scripts/demo.sh

# 2) pqcaton의 대조·토폴로지를 그 위에 확장
./scripts/extend.sh          # 산출: demo/topology-governance.svg

# 3) 정리 (core 데모가 환경 소유)
../../pqcota/demo/scripts/down.sh
```

`extend.sh`는 새 컨테이너를 만들지 않습니다. 실행 중인 `pqcota-ctl`에 이 리포의 `pqcota-report`를 주입하고,
core가 이미 수집한 `/work/results`에 **선언(declaration.json)** 을 대조해 3-상태 인벤토리 + 거버넌스
토폴로지를 만듭니다.

## 요구 사항
- 실행 중인 pqcota 디스커버리 데모(위 1단계).
- 빌드 머신에 **Go**. 그게 전부입니다 — pqcota v0.5.0부터 모듈 경로가 리포 주소와 같아져
  `go build`가 계약을 스스로 받아옵니다. 형제 체크아웃도 `replace`도 필요 없습니다.

## 커스터마이즈
`declaration.json`(고객 선언)을 편집하면 CONFIRMED/UNDECLARED/UNOBSERVED 분포가 달라집니다.
`extend.sh`가 `__IP_*__`를 실제 컨테이너 IP로 채웁니다.
