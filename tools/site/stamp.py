#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: BUSL-1.1
"""만든 사이트에 판을 찍는다.

usage: stamp.py <site-dir> --kind release|development --label <표시> --info <build-info 줄 파일>

- 첫 화면의 `__VERSION__` 자리를 판 표시로 바꾼다(자리가 있을 때만. 없으면 첫 화면을 건드리지 않는다).
- **개발판(development)이면 모든 HTML에** 「개발판 · 릴리스 아님 · 배포하지 않는다」 띠와 `noindex`를 단다. 릴리스 판이면 달지 않는다.
- build-info.txt를 사이트 맨 위에 쓴다(판 종류 · 태그 · 전체 커밋 · 도구 판). 다시 돌려도 결과가 같다.
"""
import os, re, sys

site = sys.argv[1]
arg = lambda k: sys.argv[sys.argv.index(k) + 1]
kind, label, info = arg("--kind"), arg("--label"), arg("--info")
if kind not in ("release", "development"):
    sys.exit("stamp: --kind must be release or development")
MARK = "pq-devbuild"
BANNER = ('<div class="%s" role="note" style="position:sticky;top:0;z-index:100;padding:6px 12px;background:#b3261e;color:#fff;'
          'font:600 13px/1.4 system-ui,sans-serif;text-align:center">Development build (%s): not a release. '
          'Do not deploy. 개발판입니다: 릴리스가 아니며 배포하지 않습니다.</div>')
NOINDEX = '<meta name="robots" content="noindex">'

n = 0
for d, _, fs in os.walk(site):
    for f in fs:
        if not f.endswith(".html"):
            continue
        path = os.path.join(d, f)
        h = open(path, encoding="utf-8").read()
        h = re.sub(r'<div class="%s".*?</div>' % MARK, "", h, flags=re.S).replace(NOINDEX, "")
        if os.path.relpath(path, site) == "index.html":
            h = h.replace("__VERSION__", label)
        if kind == "development":
            if "<head>" in h:
                h = h.replace("<head>", "<head>" + NOINDEX, 1)
            m = re.search(r"<body[^>]*>", h)
            if not m:
                sys.exit("stamp: %s has no <body>" % path)
            h = h[:m.end()] + BANNER % (MARK, label) + h[m.end():]
        open(path, "w", encoding="utf-8").write(h)
        n += 1

with open(os.path.join(site, "build-info.txt"), "w", encoding="utf-8") as out:
    out.write("kind: %s\nlabel: %s\n" % (kind, label))
    out.write(open(info, encoding="utf-8").read())
print("stamped %d page(s) as %s (%s)" % (n, kind, label))
