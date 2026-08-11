# pqcaton

**PQC 이관에서 무엇을 바꿀지 정하는 자리.** 관측은 [pqcota](https://github.com/randyinthedev-hash/pqcota)가
하고, 이 리포는 그 관측을 **선언과 대조하고, 리뷰에 걸고, 확정한다.**

**이름** — *pqcaton*(발음 **P-caton**) = **PQC** + **baton**(지휘봉). pqcota가 교향악의 *단원*이라면
이것은 지휘자가 쥐는 막대다. **지휘하는 것은 여전히 사람이고**, 이 도구는 그 손에 들린다.

> **소스 공개(source-available)이지 오픈소스가 아니다.** [BUSL-1.1](LICENSE) — 관리 노드 5대까지
> 무료, 그 이상은 계약. **2030-08-11에 Apache-2.0으로 전환된다.** → [라이선스 안내](LICENSING.md)

---

## 왜 따로 있나

pqcota는 **관측한 사실만 낸다.** 🔴 표시는 "취약하다"는 판정이 아니라 "고전 알고리즘으로
협상됐다"는 관측이고, 무엇을 언제 바꿀지는 그 도구가 정하지 않는다. 그 선을 지키려고
선언 대조·리뷰 큐·확정 거버넌스를 **명시적으로 만들지 않았다.**

그런데 조직에서는 누군가 그 판단을 해야 한다. 그리고 그 판단은 **남아야 한다** — 누가 언제
무엇을 근거로 정했는지가 감사 대상이기 때문이다. 이 리포가 그 자리를 맡는다.

```
pqcota                          pqcaton
  관측 ─────► contracts/ ─────►  선언과 대조 (3-상태)
  정규화                          confidence 스코어링
  전환물 생성 ◄───── 확정 계획 ◄── 리뷰 큐 → 확정
```

두 리포는 **계약으로만 이어진다.** pqcota는 이 리포 없이도 혼자 완결되고, 실제로 그렇게 쓸 수 있다.

## 무엇이 들어 있나

| 모듈 | 하는 일 |
|---|---|
| [`pkg/inventory/reconcile`](pkg/inventory/reconcile) | **3-상태 대조** — CONFIRMED(선언∩관측) · UNDECLARED(관측만 = shadow) · UNOBSERVED(선언만) |
| [`pkg/inventory/decision`](pkg/inventory/decision) | **리뷰-확정 상태기계** — draft → in-review → finalized. 확정 전에는 프로비저닝이 돌지 않는다 |
| [`inventory/cmd/pqcota-reconcile`](inventory/cmd/pqcota-reconcile) | 대조 실행 |
| [`inventory/cmd/pqcota-report`](inventory/cmd/pqcota-report) | 거버넌스 리포트·토폴로지 |

**UNDECLARED가 이 도구의 첫 값이다.** CMDB에 없는데 실제로 통신하고 있는 엣지 — 조직이 모르는
연결이다. 보안에서 가장 먼저 봐야 할 것이 거기 있다.

**UNOBSERVED는 기계가 확정하지 않는다.** 선언에는 있는데 관측되지 않은 것이 *실재하는데 못 본 것*인지
*이미 없어진 것*인지는 사람만 안다. pqcota의 완전성 맵이 "원리상 관측 불가"인지 "실제 없음"인지를
갈라 주고, 그 위에서 사람이 정한다.

설계는 [docs/design.md](docs/design.md), 검증 기준은 [docs/testcases.md](docs/testcases.md).

## 써보기

```bash
make            # 라이선스 게이트 → 빌드 → 테스트
```

데모는 pqcota의 디스커버리 데모 위에 얹는다 — [demo/README.md](demo/README.md).

## 라이선스

[**BUSL-1.1**](LICENSE) · 2030-08-11에 Apache-2.0으로 전환.

- 평가·개발·테스트는 규모 제한 없이 무료
- 프로덕션은 관리 노드 5대까지 무료
- 그 이상은 계약 — **kty@sntsoft.co.kr**

자세한 것은 [LICENSING.md](LICENSING.md). 기여는 [CONTRIBUTING.md](CONTRIBUTING.md)를 먼저 읽어 주십사 한다
(CLA가 필요하다).

상류 pqcota는 Apache-2.0이고, 귀속 고지는 [NOTICE](NOTICE)에 있다.

---

(주)에스앤티소프트
