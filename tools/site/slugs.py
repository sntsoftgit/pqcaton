# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
"""제목 앵커를 GitHub와 같은 규칙으로 만든다(mkdocs toc의 slugify).

리포 문서의 앵커 링크는 GitHub 렌더에 맞춰 쓰고 checkdocs가 그 규칙(tools/checkdocs의 slug)으로 잰다. 사이트도 같은 규칙이어야
같은 링크가 두 곳에서 열린다. 남기는 것: 글자(L*) · 십진 숫자(Nd) · `-` · `_`. 공백은 `-`로, 나머지(③ 같은 기호 숫자 포함)는 지운다.
"""
import unicodedata


def github_slug(value, separator="-"):
    out = []
    for ch in value.strip().lower():
        cat = unicodedata.category(ch)
        if cat.startswith("L") or cat == "Nd" or ch in "-_":
            out.append(ch)
        elif ch == " ":
            out.append("-")
    return "".join(out)
