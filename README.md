# pqcaton

**개요** · [릴리스 노트](RELEASE_NOTES.md) · [여정](docs/journey.md) · [설계](docs/design.md) · [검증 기준](docs/testcases.md) · [데모](demo/README.md) · [구조 그림](https://www.sntsoft.co.kr/pqcaton/)

**PQC 이관에서 무엇을 바꿀지 정하는 자리입니다.** 관측은 [pqcota](https://github.com/randyinthedev-hash/pqcota)가
하고, 이 리포는 그 관측을 **선언과 대조하고, 리뷰 큐에 올리고, 확정합니다.**

**쓰는 길이 둘입니다.** 이 문서가 설명하는 것은 **직접 설치**입니다. 관측 결과를 자기
인프라에 두고 대조·판정·확정까지 같은 자리에서 끝내므로 밖으로 나가는 것이 없습니다.
**호스팅**으로 쓰면 관측 결과가 고객망을 나와 컨트롤 플레인으로 올라가고, 그 올리는 자리가
[`saas/runner`](saas/runner/README.md)입니다. 무엇을 보내고 무엇을 보내지 않는지가 그
문서에 표로 있습니다.

**어디부터 읽을지.**

| 무엇을 하려는가 | 어디부터 |
|---|---|
| 한 바퀴 돌려 본다 | 아래 [사전 준비](#사전-준비) → [써보기](#써보기) |
| 절차를 처음부터 끝까지 따라간다 | [여정](docs/journey.md) |
| 왜 이렇게 만들었는지 본다 | [설계](docs/design.md) · [검증 기준](docs/testcases.md) |
| 호스팅에서 무엇이 밖으로 나가는지 본다 | [`saas/runner`](saas/runner/README.md) |
| 고친 것을 보낸다 | [CONTRIBUTING](CONTRIBUTING.md) · [CLA](CLA.md)([English](CLA.en.md)) |
| 취약점을 알린다 | [SECURITY](SECURITY.md) |

**이름**: *pqcaton*(발음 **P-caton**) = **PQC** + **baton**(지휘봉). pqcota가 교향악의 *단원*이라면
이것은 지휘자가 쥐는 막대입니다. **지휘자는 운영자이고, pqcaton은 그가 사용하는 도구입니다.
판단은 도구가 아니라 사람이 합니다.**

> **소스 공개(source-available)이지 오픈소스가 아닙니다.** [BUSL-1.1](LICENSE)입니다. 관리 노드 5대까지
> 무료이고, 그 이상은 계약입니다. **각 릴리스가 4년 뒤 Apache-2.0으로 전환됩니다.** → [라이선스 안내](LICENSING.md)

---

## 왜 따로 있나

pqcota는 **관측한 사실만 알려 줍니다.** 🔴 표시는 「취약하다」는 판정이 아니라 「고전 알고리즘으로
협상됐다」는 관측이고, 무엇을 언제 바꿀지는 그 도구가 정하지 않습니다. 그 선을 지키려고
선언 대조·리뷰 큐·확정 거버넌스를 **명시적으로 만들지 않았습니다.**

그런데 조직에서는 누군가 그 판단을 해야 합니다. 그리고 그 판단은 **남아야 합니다.** 누가
언제 무엇을 근거로 정했는지가 감사 대상이기 때문입니다. 이 리포가 그 자리를 맡습니다.

```
pqcota                          pqcaton
  관측 ─────► contracts/ ─────►  선언과 대조 (3-상태)
  정규화                          confidence 스코어링
  전환물 생성 ◄───── 확정 계획 ◄── 리뷰 큐 → 확정
```

두 리포는 **계약으로만 이어집니다.** pqcota는 이 리포 없이도 그 자체로 완결되고, 실제로 그렇게
쓸 수 있습니다.

## 무엇이 들어 있나

| 모듈 | 하는 일 |
|---|---|
| [`pkg/inventory/reconcile`](pkg/inventory/reconcile) | **3-상태 대조**: CONFIRMED(선언∩관측) · UNDECLARED(관측만) · UNOBSERVED(선언만) |
| [`pkg/inventory/decision`](pkg/inventory/decision) | **리뷰-확정 상태기계**: draft → in-review → finalized. 확정 전에는 프로비저닝이 돌지 않습니다 |
| [`pkg/inventory/review`](pkg/inventory/review) | **세션 파일 형식과 확정 관문**: 명령과 화면이 이 형식 하나를 함께 씁니다 |
| [`pkg/inventory/decl`](pkg/inventory/decl) | **선언 형식과 자체 검사**: 노드↔IP가 틀리면 대조 결과가 **오류 없이** 틀립니다. 저장 전에 짚습니다 |
| [`pkg/inventory/report`](pkg/inventory/report) | **여러 노드 대조**: 레인 분리·IP 잇기·완전성. 명령과 화면이 이 계산 하나를 씁니다 |
| [`pkg/inventory/ui`](pkg/inventory/ui) | **화면**: 그리는 것과 폼을 읽는 것만 합니다. 어디서 읽고 누가 접속하는지는 부르는 쪽이 정합니다 |
| [`pkg/inventory/scope`](pkg/inventory/scope) | **자산 스코프 거버넌스**: 계층 상속·변경 승인·제외분 재검토. 규칙 형식과 집행은 pqcota 것을 그대로 씁니다 |
| [`pkg/inventory/localscan`](pkg/inventory/localscan) | **이 기계를 스캔하는 지름길**: 관측 없이 전체 절차를 한 번 돌려 보는 자리입니다. 못 본 것은 「없다」로 보고하지 않습니다 |
| [`inventory/cmd/pqcaton-decide`](inventory/cmd/pqcaton-decide) | **리뷰 큐를 사람이 판정하고 확정**: 확정 계획을 계약 형식으로 만듭니다 |
| [`inventory/cmd/pqcaton-scope`](inventory/cmd/pqcaton-scope) | **「이 자산은 안 본다」를 승인하고 배포**: 확정된 정책이 pqcota 집행기의 입력이 됩니다 |
| [`inventory/cmd/pqcaton-ui`](inventory/cmd/pqcaton-ui) | **사람이 쓰는 화면 다섯**: 선언 · 암호 자산 스코프 · 대조 · 판정 · 인벤토리 조회. 기본은 127.0.0.1이고, 스타일·스크립트까지 바이너리 하나에 들어 있어 망이 끊긴 기계에서도 뜹니다 |
| [`inventory/cmd/pqcaton-report`](inventory/cmd/pqcaton-report) | 거버넌스 리포트·토폴로지 |

**UNDECLARED 가 이 도구가 주는 첫 번째 쓸모입니다.** CMDB에 없는데 실제로 통신하고 있는 엣지, 곧 조직이
모르는 연결입니다. 보안에서 가장 먼저 봐야 할 것이 거기 있습니다.

**UNOBSERVED는 기계가 확정하지 않습니다.** 선언에는 있는데 관측되지 않은 것이 *실재하는데 못
본 것*인지 *이미 없어진 것*인지는 사람만 압니다. pqcota의 완전성 맵이 「원리상 관측 불가」인지
「실제 없음」인지를 구분해 주고, 그 위에서 사람이 정합니다.

**구조 그림**은 [www.sntsoft.co.kr/pqcaton](https://www.sntsoft.co.kr/pqcaton/)에 있습니다([소스](site/index.html)).

## 사전 준비

| | 필요한가 | 왜 |
|---|---|---|
| **Go** | **필수** | 빌드에 씁니다. 필요한 버전은 [`go.mod`](go.mod)에 적혀 있습니다(지금은 1.26.4). 다른 런타임은 필요 없습니다 |
| **Graphviz**(`dot`) | **선택** | 거버넌스 토폴로지를 그림으로 볼 때만. 없으면 화면과 명령이 **DOT 원문**을 보여 주고, 나중에 아무 데서나 그릴 수 있습니다 |
| Postgres | 선택 | 판정 원장을 DB에 둘 때만. 기본은 파일(JSONL)이라 없어도 전체 과정이 됩니다 |
| Docker · pqcota 리포 | 데모에만 | [demo/README.md](demo/README.md) 참조 |

`dot` 설치는 [graphviz.org/download](https://graphviz.org/download/)에 있습니다:
`apt install graphviz` · `brew install graphviz` · `winget install graphviz`.

**어디서 실행되는가.** 이 리포는 **ctl 노드**(관측 결과를 모아 대조·판정하는 자리)의 일을 합니다.
ctl 노드는 **OS를 가리지 않습니다.** 관측 자체는 pqcota 의 collector 가 대상 노드에서
하고, 리눅스와 Windows 를 다룹니다(상류 v0.6.3).

> 예외가 하나 있습니다. `pqcaton-decide open` 을 `-results` 없이 쓰면 **명령을 실행한 그 기계 자신을**
> 스캔합니다(`/proc`). 「체크아웃만으로 한 바퀴」를 위한 지름길이라 **리눅스에서만** 됩니다.
> 여러 노드를 제대로 다루는 길은 pqcota가 모은 결과를 읽는 [`pqcaton-report`](inventory/cmd/pqcaton-report)입니다.

## 써보기

```bash
make            # 라이선스 · 문구 관문 → 빌드 → 테스트
```

**이 리포만으로 처음부터 끝까지 해볼 수 있습니다.** 관측할 대상은 이 기계입니다.
**이 지름길은 `/proc` 을 읽으므로 리눅스에서만 됩니다.** macOS·Windows 에서는 아래 ②의
주석처럼 pqcota 가 모은 `results/` 를 읽는 길로 갑니다.

```bash
go build -o bin/ ./inventory/cmd/...

# ① 선언 — 「우리는 이렇게 알고 있다」를 적습니다. CMDB에서 뽑아도, 손으로 적어도 됩니다
printf 'node,runtime,component\nlocal,openssl,libssl\nlocal,jca,jca-provider-chain\n' > decl.csv

# ② 대조 — 이 기계를 스캔해 선언과 맞대고, 리뷰 큐를 세션 파일로 만듭니다
bin/pqcaton-decide open decl.csv local > session.json

#    여러 노드를 다루는 길은 이쪽입니다 — pqcota 가 모은 관측으로 대조합니다
#    bin/pqcaton-decide open declaration.json -results results/ -org acme > session.json

# ③ 판정 — 사람이 하는 자리. session.json 을 열어
#    필수 항목의 conclusion, 그리고 reviewer · signature 를 채웁니다
#    확정 계획에 넣을 항목은 `include_in_plan` 을 true 로

# ④ 확정 — 전 필수 판정과 승인 서명이 있어야 통과하고,
#    판정은 append-only 로 남습니다 (감사 기록)
bin/pqcaton-decide close session.json -judgments judgments.jsonl -org acme > plan.json

# ⑤ 재관측한 뒤 — 근거가 바뀐 판정만 다시 봅니다 (전면 재리뷰가 아닙니다)
bin/pqcaton-decide delta judgments.jsonl decl.csv local -org acme
```

**JSON 을 손으로 채우기 번거로우면 화면에서 채웁니다.** 같은 파일, 같은 관문입니다.

**필요한 파일만 주면 화면이 세션까지 만듭니다.** 명령을 먼저 돌리지 않아도 됩니다.

```bash
bin/pqcaton-ui session.json \
  -decl declaration.json -results results/ \
  -layers corp.csv,prod.csv -base asset-scope.csv \
  -judgments judgments.jsonl -org acme
# → http://127.0.0.1:8765 — 탭이 절차 순서입니다
#   ① 선언(관리 대상 노드와 그 안의 암호 자산·통신 엣지) → ② 암호 자산 스코프
#   → ③ 대조(3-상태·등급·토폴로지) → ④ 판정(리뷰 큐 — 판정·확정)
```

`session.json` 이 없으면 **선언과 관측 결과로 화면이 세션을 만듭니다.** 자산 스코프도 계층 CSV를
주면 그렇습니다. 그리고 **규칙을 화면에서 고칩니다.** 다섯 칸이 무슨 뜻인지는 「규칙을
적는 법」 도움말에 있고, `action` 은 고르는 칸이라 오타로 규칙이 어긋나지 않습니다.

명령으로 세션을 먼저 만드는 길도 그대로입니다. 같은 파일이고 같은 관문입니다.

```bash
bin/pqcaton-decide open declaration.json -results results/ -org acme > session.json
bin/pqcaton-scope  open corp.csv prod.csv -base asset-scope.csv -org acme > scope-session.json
```

**화면에 쓸 말은 한국어와 English 중에서 고릅니다.** 이동 링크 오른쪽 끝의 토글을 누르면 보던
자리 그대로 말만 바뀌고, 고른 것은 기억됩니다. 처음에는 브라우저 설정을 따릅니다.

> **명령의 출력과 로그는 영어 하나입니다.** 붙여 넣어 검색하고 이슈에 올리는 것이라,
> 말이 서로 다르면 같은 문제가 두 문장으로 남습니다. 자세한 규칙은
> [CONTRIBUTING.md](CONTRIBUTING.md#어느-말로-쓰나)에 있습니다.

`-decl`·`-layers`·`-results` 는 **주는 것만 탭이 열립니다.** 선언 파일만 주면 「선언」과
「판정」 둘만 보입니다. 없는 것을 눌러 보게 하지 않습니다.

**무엇을 계속 볼지도 승인을 거칩니다.** 인벤토리에서 뺀 자산은 나중에 「왜 이것은 안 봤나」에
답해야 하므로, 판정과 확정을 같은 절차에서 함께 다룹니다.

계층은 준 순서대로 겹칩니다. 조직 · 환경 · 노드군 순입니다. **같은 자산에 규칙이 여럿 걸리면 뒤
계층의 것이 적용됩니다.**

```bash
# 계층을 겹쳐 바뀐 규칙만 리뷰에 올립니다 (-base 로 지금 쓰는 정책을 주면 델타만)
bin/pqcaton-scope open corp.csv prod.csv pay.csv -org acme > scope-session.json

# 승인 — exclude 추가는 결론이 없으면 확정되지 않습니다
bin/pqcaton-scope close scope-session.json -judgments judgments.jsonl -org acme > asset-scope.csv

# 나온 CSV 가 그대로 pqcota 집행기의 입력입니다
pqcota-ingest -scope-assets asset-scope.csv results/

# 제외는 영구 면제가 아닙니다 — 승인이 없거나 오래된 것만 다시 올립니다
bin/pqcaton-scope review asset-scope.csv results/ -judgments judgments.jsonl -org acme
```

**③에서 정책 단위로 판정합니다.** 세션 파일의 `policy_decisions` 에 정책 하나당 결론 하나를 적으면
같은 정책의 항목이 한 번에 판정됩니다. 수천 대를 한 건씩 보는 리뷰는 끝나지 않습니다.
개별 `conclusion`은 예외를 위한 자리입니다.

**④가 이 리포에서 반드시 거쳐야 하는 관문입니다.** 하나라도 비면 확정하지 않고 **무엇이 남았는지 알려 줍니다.**

```
❌ cannot finalize: mandatory items are still unjudged — every mandatory item must be judged
   · the signature is not filled in
   · no record of why this was decided: local/openssl/libssl-e2f2d68a (UNDECLARED)
```

> **명령의 출력은 영어입니다.** 화면에 쓸 말은 한국어와 English 중에서 고릅니다.
> 자세한 것은 [CONTRIBUTING.md 「어느 말로 쓰나」](CONTRIBUTING.md#어느-말로-쓰나)에 있습니다.

나온 `plan.json`은 **pqcota가 그대로 받습니다.** 계약 형식이라 우리 형식이 따로 없습니다.

```bash
pqcota-provision --level l2 plan.json > provision.yml   # pqcota 리포의 명령
```

여러 노드를 훑는 길과 거버넌스 토폴로지는 [여정](docs/journey.md)에 있습니다.

데모는 pqcota의 디스커버리 데모 위에 얹습니다. [demo/README.md](demo/README.md) 를 보십시오.

## 라이선스

[**BUSL-1.1**](LICENSE) · **각 릴리스가 공개일로부터 4년 뒤 Apache-2.0으로 전환됩니다.**
버전별 전환일은 [릴리스 노트](RELEASE_NOTES.md)에 있습니다(v0.1.0~v0.5.0은 2030-08-11).

- 평가·개발·테스트는 규모 제한 없이 무료
- 프로덕션은 관리 노드 5대까지 무료
- 그 이상은 계약: **kty@sntsoft.co.kr**

자세한 것은 [LICENSING.md](LICENSING.md)에 있습니다. 기여하실 때는
[CONTRIBUTING.md](CONTRIBUTING.md)를 먼저 읽어 주십시오([CLA](CLA.md)가 필요합니다).
보안 취약점은 이슈가 아니라 [SECURITY.md](SECURITY.md)의 경로로 알려 주십시오.

> **English**: the contributor agreement is available in English at
> [CLA.en.md](CLA.en.md). The rest of the documentation is in Korean; the code, its output
> and the file formats are in English.

기반 프로젝트 pqcota는 Apache-2.0이고, 귀속 고지는 [NOTICE](NOTICE)에 있습니다.

---

(주)에스앤티소프트
