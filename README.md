# pqcaton

**개요** · [릴리스 노트](RELEASE_NOTES.md) · [여정](docs/journey.md) · [설계](docs/design.md) · [검증 기준](docs/testcases.md) · [데모](demo/README.md) · [구조 그림](https://www.sntsoft.co.kr/pqcaton/)

**PQC 이관에서 무엇을 바꿀지 정하는 자리입니다.** 관측은 [pqcota](https://github.com/randyinthedev-hash/pqcota)가
하고, 이 리포는 그 관측을 **선언과 대조하고, 리뷰 큐에 올리고, 확정합니다.**

**이름** — *pqcaton*(발음 **P-caton**) = **PQC** + **baton**(지휘봉). pqcota가 교향악의 *단원*이라면
이것은 지휘자가 쥐는 막대입니다. **지휘자는 운영자이고, pqcaton은 그가 사용하는 도구입니다 —
판단은 도구가 아니라 그 손에 있습니다.**

> **소스 공개(source-available)이지 오픈소스가 아닙니다.** [BUSL-1.1](LICENSE) — 관리 노드 5대까지
> 무료, 그 이상은 계약. **각 릴리스가 4년 뒤 Apache-2.0으로 전환됩니다.** → [라이선스 안내](LICENSING.md)

---

## 왜 따로 있나

pqcota는 **관측한 사실만 냅니다.** 🔴 표시는 "취약하다"는 판정이 아니라 "고전 알고리즘으로
협상됐다"는 관측이고, 무엇을 언제 바꿀지는 그 도구가 정하지 않습니다. 그 선을 지키려고
선언 대조·리뷰 큐·확정 거버넌스를 **명시적으로 만들지 않았습니다.**

그런데 조직에서는 누군가 그 판단을 해야 합니다. 그리고 그 판단은 **남아야 합니다** — 누가
언제 무엇을 근거로 정했는지가 감사 대상이기 때문입니다. 이 리포가 그 자리를 맡습니다.

```
pqcota                          pqcaton
  관측 ─────► contracts/ ─────►  선언과 대조 (3-상태)
  정규화                          confidence 스코어링
  전환물 생성 ◄───── 확정 계획 ◄── 리뷰 큐 → 확정
```

두 리포는 **계약으로만 이어집니다.** pqcota는 이 리포 없이도 혼자 완결되고, 실제로 그렇게
쓸 수 있습니다.

## 무엇이 들어 있나

| 모듈 | 하는 일 |
|---|---|
| [`pkg/inventory/reconcile`](pkg/inventory/reconcile) | **3-상태 대조** — CONFIRMED(선언∩관측) · UNDECLARED(관측만 = shadow) · UNOBSERVED(선언만) |
| [`pkg/inventory/decision`](pkg/inventory/decision) | **리뷰-확정 상태기계** — draft → in-review → finalized. 확정 전에는 프로비저닝이 돌지 않습니다 |
| [`pkg/inventory/review`](pkg/inventory/review) | **세션 파일 형식과 확정 게이트** — 명령과 화면이 이 하나를 씁니다 |
| [`pkg/inventory/scope`](pkg/inventory/scope) | **자산 스코프 거버넌스** — 계층 상속·변경 승인·제외분 재검토. 규칙 형식과 집행은 pqcota 것을 그대로 씁니다 |
| [`inventory/cmd/pqcaton-decide`](inventory/cmd/pqcaton-decide) | **리뷰 큐를 사람이 판정하고 확정** — 확정 계획을 계약 형식으로 냅니다 |
| [`inventory/cmd/pqcaton-reconcile`](inventory/cmd/pqcaton-reconcile) | 대조 실행 |
| [`inventory/cmd/pqcaton-scope`](inventory/cmd/pqcaton-scope) | **「이 자산은 안 본다」를 승인하고 배포** — 확정된 정책이 pqcota 집행기의 입력이 됩니다 |
| [`inventory/cmd/pqcaton-ui`](inventory/cmd/pqcaton-ui) | **리뷰 큐를 사람이 다루는 화면** — 표준 라이브러리만 쓰고 기본은 127.0.0.1입니다 |
| [`inventory/cmd/pqcaton-report`](inventory/cmd/pqcaton-report) | 거버넌스 리포트·토폴로지 |

**UNDECLARED가 이 도구의 첫 값입니다.** CMDB에 없는데 실제로 통신하고 있는 엣지 — 조직이
모르는 연결입니다. 보안에서 가장 먼저 봐야 할 것이 거기 있습니다.

**UNOBSERVED는 기계가 확정하지 않습니다.** 선언에는 있는데 관측되지 않은 것이 *실재하는데 못
본 것*인지 *이미 없어진 것*인지는 사람만 압니다. pqcota의 완전성 맵이 "원리상 관측 불가"인지
"실제 없음"인지를 갈라 주고, 그 위에서 사람이 정합니다.

**구조 그림**은 [www.sntsoft.co.kr/pqcaton](https://www.sntsoft.co.kr/pqcaton/)([소스](site/index.html)), **처음부터 끝까지 따라가는 여정**은
[docs/journey.md](docs/journey.md)에 있습니다. 설계 근거는 [docs/design.md](docs/design.md),
검증 기준은 [docs/testcases.md](docs/testcases.md)입니다.

## 써보기

```bash
make            # 라이선스 게이트 → 빌드 → 테스트
```

**이 리포만으로 한 바퀴가 돕니다.** 관측할 대상은 이 머신입니다.

```bash
go build -o bin/ ./inventory/cmd/...

# ① 선언 — CMDB가 "있다"고 말하는 것. 직접 씁니다
printf 'node,runtime,component\nlocal,openssl,libssl\nlocal,jca,provider-chain\n' > decl.csv

# ② 대조 — 이 머신을 스캔해 선언과 맞대고, 리뷰 큐를 세션 파일로 냅니다
bin/pqcaton-decide open decl.csv local > session.json

# ③ 판정 — 사람이 하는 자리. session.json 을 열어
#    필수 항목의 conclusion, 그리고 reviewer · signature 를 채웁니다
#    확정 계획에 넣을 항목은 `확정_계획에_넣는다`를 true 로

# ④ 확정 — 전 필수 판정과 승인 서명이 있어야 통과하고,
#    판정은 append-only 로 남습니다 (감사 기록)
bin/pqcaton-decide close session.json -judgments judgments.jsonl -org acme > plan.json

# ⑤ 재관측한 뒤 — 근거가 바뀐 판정만 다시 봅니다 (전면 재리뷰가 아닙니다)
bin/pqcaton-decide delta judgments.jsonl decl.csv local -org acme
```

**JSON을 눈으로 훑기 싫으면 화면으로 채웁니다.** 같은 파일, 같은 게이트입니다.

```bash
bin/pqcaton-ui session.json -judgments judgments.jsonl -org acme
# → http://127.0.0.1:8765 — 정책 단위로 결론을 채우고 「확정하고 계획 내기」
```

**무엇을 계속 볼지도 승인을 거칩니다.** 「이 자산은 안 본다」는 사고 뒤에 근거를 대야 하는
결정이라, 같은 왕복으로 다룹니다. 계층은 준 순서대로 이깁니다 — 조직 · 환경 · 노드군 순.

```bash
# 계층을 겹쳐 바뀐 규칙만 리뷰에 올립니다 (-base 로 지금 쓰는 정책을 주면 델타만)
bin/pqcaton-scope open corp.csv prod.csv pay.csv -org acme > scope-session.json

# 승인 — exclude 추가는 결론이 없으면 확정되지 않습니다
bin/pqcaton-scope close scope-session.json -judgments judgments.jsonl -org acme > asset-scope.csv

# 나온 CSV 가 그대로 상류 집행기의 입력입니다
pqcota-ingest -scope-assets asset-scope.csv results/

# 제외는 영구 면제가 아닙니다 — 승인이 없거나 오래된 것만 다시 올립니다
bin/pqcaton-scope review asset-scope.csv results/ -judgments judgments.jsonl -org acme
```

**③에서 정책 단위로 판정합니다.** 세션 파일의 `정책_판정`에 정책 하나당 결론 하나를 적으면
같은 정책의 항목이 한 번에 판정됩니다 — 수천 대를 한 건씩 보는 리뷰는 끝나지 않습니다.
개별 `conclusion`은 예외를 위한 자리입니다.

**④가 이 리포의 최강 게이트입니다.** 하나라도 비면 확정하지 않고 **무엇이 남았는지 말합니다.**

```
❌ finalize: 미판정 필수 항목 존재(전 필수 판정 전 불가)
   · signature 가 비어 있습니다
   · 결론 없음: local/openssl/libssl-e2f2d68a (UNDECLARED)
```

나온 `plan.json`은 **상류가 그대로 받습니다** — 계약 형식이라 우리 형식이 따로 없습니다.

```bash
pqcota-provision --level l2 plan.json > provision.yml   # pqcota 리포의 명령
```

여러 노드를 훑는 길과 거버넌스 토폴로지는 [여정](docs/journey.md)에 있습니다.

데모는 pqcota의 디스커버리 데모 위에 얹습니다 — [demo/README.md](demo/README.md).

## 라이선스

[**BUSL-1.1**](LICENSE) · **각 릴리스가 공개일로부터 4년 뒤 Apache-2.0으로 전환됩니다.**
버전별 전환일은 [릴리스 노트](RELEASE_NOTES.md)에 있습니다(v0.1.0~v0.5.0은 2030-08-11).

- 평가·개발·테스트는 규모 제한 없이 무료
- 프로덕션은 관리 노드 5대까지 무료
- 그 이상은 계약 — **kty@sntsoft.co.kr**

자세한 것은 [LICENSING.md](LICENSING.md)에 있습니다. 기여하실 때는
[CONTRIBUTING.md](CONTRIBUTING.md)를 먼저 읽어 주십시오(CLA가 필요합니다).
보안 취약점은 이슈가 아니라 [SECURITY.md](SECURITY.md)의 경로로 알려 주십시오.

상류 pqcota는 Apache-2.0이고, 귀속 고지는 [NOTICE](NOTICE)에 있습니다.

---

(주)에스앤티소프트
