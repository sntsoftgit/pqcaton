#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Great Honor <randyinthedev@gmail.com>
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: Apache-2.0
"""언어 전환 링크를 만든 사이트에 얹는다.

pqcota tools/site/langswitch.py(8a4e9bf0, v0.13.0)에서 가져와 pqcaton에 맞게 고쳤다. 고친 내역은 docs/site.md에 있다.

usage: langswitch.py <site-dir>

영어 쪽 `X/index.html`에 한국어 짝 `ko/X/index.html`이 있으면 영어 쪽에 「한국어」 링크를, 한국어 쪽에는 「English」 링크를
단다. 짝이 없는 쪽에는 아무것도 달지 않는다(비어 있는 한국어 경로로 보내지 않는다). 한국어 쪽의 `<html lang>`도 고친다.
링크는 쪽 위치에서의 상대 경로라 하위 경로(/pqcaton/)에 얹어도 그대로 맞는다. 다시 돌려도 결과가 같다."""
import os, re, sys

site = sys.argv[1]
MARK = "pq-lang"
STYLE = ('<style>.pq-lang{position:fixed;right:16px;bottom:16px;z-index:50;padding:6px 14px;border-radius:16px;'
         'background:#1a2233;color:#fff;font:600 13px/1.4 system-ui,sans-serif;text-decoration:none;'
         'border:1px solid rgba(255,255,255,.35);box-shadow:0 1px 6px rgba(0,0,0,.3)}.pq-lang:hover{background:#c0453e}</style>')


def pages(root):
    for d, _, fs in os.walk(root):
        for f in fs:
            if f.endswith(".html"):
                yield os.path.relpath(os.path.join(d, f), site)


ko_pages = {p[len("ko/"):] for p in pages(os.path.join(site, "ko"))} if os.path.isdir(os.path.join(site, "ko")) else set()
en_pages = {p for p in pages(site) if not p.startswith("ko/")}
n = {"en->ko": 0, "ko->en": 0}
for rel in sorted(p for p in pages(site)):
    # 손으로 쓴 쪽(첫 화면 · 아키텍처 · 화면 시안)은 저마다 언어를 다루므로 건드리지 않는다.
    if rel in ("index.html", "404.html", "ko/404.html", "ui-next.html") or rel.startswith("architectures/") or rel == "ko/index.html":
        continue
    is_ko = rel.startswith("ko/")
    twin = (rel[3:] if is_ko else "ko/" + rel)
    if is_ko and rel[3:] not in en_pages: continue            # 영어 짝이 없는 한국어 쪽(생기지 않아야 한다)
    if not is_ko and rel not in ko_pages: continue            # 한국어 짝이 없는 영어 쪽: 링크를 달지 않는다
    href = os.path.relpath(os.path.dirname(twin) or ".", os.path.dirname(rel) or ".").replace(os.sep, "/") + "/"
    path = os.path.join(site, rel)
    h = open(path, encoding="utf-8").read()
    h = re.sub(r'<a class="%s".*?</a>' % MARK, "", h, flags=re.S).replace(STYLE, "")
    link = '<a class="%s" href="%s" hreflang="%s" lang="%s">%s</a>' % (MARK, href, "en" if is_ko else "ko", "en" if is_ko else "ko", "English" if is_ko else "한국어")
    assert "</body>" in h, rel
    h = h.replace("</body>", STYLE + link + "</body>", 1)
    if is_ko: h = re.sub(r'<html lang="[a-zA-Z-]+"', '<html lang="ko"', h, count=1)
    open(path, "w", encoding="utf-8").write(h)
    n["ko->en" if is_ko else "en->ko"] += 1
print("language links:", n)
