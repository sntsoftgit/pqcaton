[English](site.md) · 한국어

# 공개 사이트

지금은 `docs/` 아래가 그대로 GitHub Pages로 올라갑니다([워크플로](../.github/workflows/pages.yml)). pqcota처럼 사이트의 첫 화면
(`index.html`)과 문서가 한 자리에 있습니다. **리포의 문서로 만드는 문서 사이트를 더하고 있습니다**([아래](#문서-사이트)). 릴리스
태그로 배포하기 전까지 켜져 있는 사이트는 지금 그대로입니다.

| 지금 | |
|---|---|
| [`index.html`](index.html) | 아키텍처 문서. 자립형 한 장이라 외부 요청이 하나도 없습니다. 한국어와 English로 봅니다 |
| [`ui-next.html`](ui-next.html) | **v0.22.0 화면 설계의 과거 정적 시안**(2026-08-26). 이 설계는 v0.22.0부터 `pqcaton-ui`의 기본 화면이 되었고, 실제 화면은 그 뒤로 달라진 데가 있습니다. 기록으로 남겨 둔 것이며 데이터를 읽거나 쓰지 않습니다. 한국어와 English로 봅니다 |

## 문서 사이트

리포의 문서([`pages.txt`](../tools/site/pages.txt) · [`pages.ko.txt`](../tools/site/pages.ko.txt)로 고정)가 영어 사이트와
`/pqcaton/ko/`의 한국어 사이트가 됩니다([`tools/site`](../tools/site)). **기준은 리포의 원본 문서이고, 사이트는 한 릴리스 태그의
읽기 사본입니다. 둘이 다르면 원본이 우선합니다.** 생성물은 커밋하지 않습니다. [사이트 워크플로](../.github/workflows/site.yml)는 main에서
사이트를 만들고 재서 Actions artifact로만 남기고, 배포하지 않습니다.

### 빌드의 판

`build.sh`는 판을 입력으로 받지 않습니다. `HEAD`를 가리키는 `vX.Y.Z` 태그가 하나이고, 그 태그의 커밋이 `HEAD`이며, 작업 트리가
깨끗할 때만 **릴리스** 빌드입니다. 그 밖은 모두 **개발판**이고, 모든 쪽에 「개발판입니다: 릴리스가 아니며 배포하지 않습니다」 띠와
`noindex`가 붙습니다. 사이트 맨 위의 `build-info.txt`에 판 종류 · 태그 · 전체 커밋 · Python과 Zensical 판 · 고정한 의존성과
`pip freeze`의 해시를 적습니다. 릴리스 태그로만 배포하고 한 태그의 사이트를 다른 내용으로 다시 만들지 않는 것은 다음 단계에서
합니다. 급한 문서 수정도 패치 릴리스로 냅니다.

### 빌드가 재는 것

- Python이 고정한 판([`python-version`](../tools/site/python-version))이고, 도구는 해시를 단
  [`requirements.txt`](../tools/site/requirements.txt)로만 설치하며, `pip freeze`가 [`freeze.txt`](../tools/site/freeze.txt)와 같아야 합니다.
- 추적하는 문서가 고정한 쪽 목록과 같아야 합니다. 리포 밖으로 나가는 링크, 없는 파일로 가는 링크, 절대 경로는 빌드를 멈춥니다.
- 빌드 뒤에 모든 HTML(생성한 쪽 · 첫 화면 · `ui-next.html`)의 링크가 사이트 안에서 앵커까지 풀려야 합니다. `/pqcaton/` 접두어가
  빠진 절대 경로, 사이트 밖으로 나가는 상대 경로, GitHub의 이 리포 `main`을 가리키는 링크는 문제입니다. 옛 주소로 남긴 원본 문서의
  링크는 리포 안의 파일로 풀려야 합니다. 한국어 쪽에서 영어 쪽으로 가는 링크는 같은 블록에 그렇다고 적어야 합니다(영문 · 영어 ·
  English). 문제는 0건이어야 합니다.
- 제목 앵커는 GitHub의 규칙을 따릅니다([`slugs.py`](../tools/site/slugs.py)). GitHub에 맞춰 쓴 앵커가 사이트에서도 열립니다.

### 지키는 주소

`/pqcaton/`은 첫 화면으로 남고 `/pqcaton/ui-next.html`도 그 자리에 남습니다. 모든 `docs/*.md`도 바이트 그대로 사이트 맨 위에
복사해, 지금 원문이 열리는 주소가 그대로 열립니다. 생성한 쪽은 `overview/` · `users/` · `demo/` · `developers/` · `project/` 아래에
있습니다. `saas/` 문서는 이 사이트에 넣지 않습니다.

### 도구의 출처

`assemble.py` · `build.sh` · `check_site.py` · `langswitch.py`와 mkdocs 설정 둘은 pqcota `tools/site`의 커밋
`8a4e9bf060416533fab9d40dcce8427d81e10faa`(v0.13.0)에서 가져왔고, Apache License 2.0 그대로입니다(파일 머리말에 적었고, 라이선스
본문은 [LICENSES/Apache-2.0.txt](../LICENSES/Apache-2.0.txt), 고지는 [NOTICE](../NOTICE)에 있습니다). pqcaton은 자기 사본을 고정하고,
빌드할 때 pqcota의 도구를 받아 오지 않습니다. 고친 내역은 다음과 같습니다.

| 파일 | pqcaton에 맞게 고친 것 |
|---|---|
| `assemble.py` | 리포 다섯 대신 하나, pqcaton의 사이트 자리, `saas/`와 옛 자리 안내 쪽 빼기, 첫 화면과 `ui-next.html`을 사이트 파일로, 리포 밖 · 없는 파일 · 절대 경로 링크는 남기지 않고 빌드를 멈춤 |
| `build.sh` | 판을 `HEAD`를 가리키는 태그에서 구함(아니면 개발판), Python과 `pip freeze` 검사, 모든 `docs/*.md`를 옛 주소에 남김, 손으로 쓴 쪽의 `main` 링크를 빌드의 판으로 고정, `stamp.py` |
| `check_site.py` | `<link>` · `<img>` · `<script>`까지, 옛 주소로 남긴 원본 문서, 접두어 빠짐과 사이트 밖을 문제로 이름 붙임, 이 리포 `main` 링크, 언어 표시 낱말 |
| `langswitch.py` | 손으로 쓴 쪽인 `ui-next.html`은 건드리지 않음 |
| `mkdocs.base.yml` · `mkdocs.ko.base.yml` | logo · favicon 줄 빼기, GitHub의 제목 앵커 |

`stamp.py` · `slugs.py` · `local.sh` · `python-version` · `freeze.txt`와 `requirements.txt`의 해시는 pqcaton에서 새로 만들었습니다.

이 기계에서 만들어 보려면(Docker가 필요하고, 산출물은 리포 밖에 생기며 배포하지 않습니다):

```bash
tools/site/local.sh ~/pqcaton-sandbox/site-out/main
```

## 규칙

- **자립형으로 둡니다.** CDN 스크립트·웹폰트·원격 이미지를 쓰지 않습니다. 오프라인에서 파일을
  더블클릭해도 그대로 열려야 합니다.
- **두 말로 씁니다.** 문구를 `<span lang="ko">`와 `<span lang="en">` 쌍으로 두고 CSS가 한쪽만
  보입니다. **새 문구를 더할 때 두 말을 함께 씁니다.** 한쪽만 쓰면 다른 말로 볼 때 그 자리가
  빕니다. 화면에 안 보이는 문구(`title`·`aria-label`·`placeholder`·`data-toast`)는 `-ko`·`-en`
  속성에 담고 스크립트가 갈아 끼웁니다. 고른 말은 두 페이지가 같은 식별자로 기억합니다.
- **영어에는 엠대시를 써도 됩니다.** [문체 관문](../tools/checkprose)은 한국어가 든 줄만 봅니다.
  `index.html`은 두 말을 다른 줄에 두어 영어 줄이 관문 밖에 놓이고, `ui-next.html`은 라벨이
  짧아 한 줄에 두는 대신 영어에서 엠대시를 쓰지 않습니다.
- **라이트·다크 둘 다 그립니다.** 색은 `:root`의 토큰으로만 씁니다. 미디어 쿼리 안에서만
  정의된 색이 있으면 한쪽 테마에서 글자가 안 보입니다.
- **페이지에서 저장소 문서로 거는 링크는 GitHub의 절대 URL**입니다. Pages는 `.md`를 그리지 않고 원문 그대로 내므로, 같은 자리의
  `developers.md`를 상대 링크로 걸면 읽기 어려운 원문이 열립니다. 페이지 사이의 상대 링크와 `#앵커`, 그리고 절대 URL이 가리키는
  저장소 경로는 `make check-docs`가 잽니다.

## 소개 홈페이지로 넓힐 때

`index.html`을 영어 기본의 소개 페이지로 새로 쓰고, 지금 것을 `architectures/`로 옮겨 사이트에서
`/pqcaton/architectures/`로 남깁니다(사이트 개편 2단계). 머리의 `<nav>`가 그때 쓰라고 미리 있는 자리입니다.

## 켜져 있습니다

**https://www.sntsoft.co.kr/pqcaton/**: 조직에 커스텀 도메인(`www.sntsoft.co.kr`)이 걸려 있어
`github.io`가 아닙니다. Source는 GitHub Actions이고 HTTPS를 강제합니다.

조직이 Team 플랜이라 **private 리포에서도 배포됩니다.** 다만 그 대가를 알고 있어야 합니다:

> **리포가 private이어도 배포된 사이트는 공개입니다.** URL을 아는 사람은 누구나 봅니다.
> 접근 제어가 붙은 Pages는 Enterprise Cloud에서만 됩니다.

그래서 `docs/` 아래에는 **공개돼도 되는 것만** 둡니다. 리포가 공개라 `docs/`의 문서가 사이트로 함께 나가도 새로 드러나는 것은
없지만, 이 폴더에 비밀값이나 내부 기록을 두면 그대로 공개됩니다.

`ui-next.html`도 공개해도 되는 정적 시안입니다. 2026-08-26의 `demo-corp` 화면을 바탕으로 그린
v0.22.0 화면 설계의 초안이고, 선언·CSV·판정 원장에 접근하거나 확정을 수행하지 않습니다. 지금 화면은
`pqcaton-ui`를 띄워서 봅니다.
