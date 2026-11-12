#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Great Honor <randyinthedev@gmail.com>
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: Apache-2.0
"""만든 사이트를 생성기의 경고에 기대지 않고 잰다.

pqcota tools/site/check_site.py(8a4e9bf0, v0.13.0)에서 가져와 pqcaton에 맞게 고쳤다. 고친 내역은 docs/site.md에 있다.

usage: check_site.py <site-dir> --prefix /pqcaton --repo <repo> --ref <tag-or-commit>

- 모든 HTML(생성한 쪽 · 첫 화면 · 화면 시안)의 <a href> · <link href> · <img src> · <script src>: 사이트 안 링크는 있는 파일로
  가야 하고, #조각은 그 쪽의 id여야 한다. `/pqcaton/` 접두어가 빠진 절대 경로, 사이트 밖으로 나가는 상대 경로는 문제다.
- 원문 주소를 지키려고 사이트 맨 위에 복사한 Markdown: 상대 링크는 리포의 같은 자리(docs/)에서 풀어 리포 안의 파일이어야
  하고, 절대 경로는 `/pqcaton/` 아래의 있는 파일이어야 한다.
- GitHub의 이 리포 main을 가리키는 링크는 문제다. 사이트는 한 판의 사본이므로 그 판(--ref)을 가리켜야 한다.
- 한국어 쪽 본문(문단 · 목록 항목 · 표 칸)에서 사이트 안 영문 쪽으로 가는 링크는 그 블록에 「영문」 · 「영어」 · English 가운데 하나가
  있어야 한다(이 리포의 문서는 「영어판」이라고 쓴다). 손으로 쓴 쪽(첫 화면 · 화면 시안 · 아키텍처)은 언어를 묻지 않는다.
마지막 줄 `problems: N`이 0이어야 통과다.
"""
import collections, html.parser, os, re, sys, urllib.parse

site = sys.argv[1]
arg = lambda k, d=None: sys.argv[sys.argv.index(k) + 1] if k in sys.argv else d
PREFIX = (arg("--prefix") or "").rstrip("/")
REPO, REF = arg("--repo"), arg("--ref")
if not PREFIX or not REPO or not REF:
    sys.exit("usage: check_site.py <site-dir> --prefix /pqcaton --repo <repo> --ref <ref>")
MAIN_LINK = re.compile(r"https://github\.com/sntsoftgit/pqcaton/(?:blob|tree)/main/")
BLOCKS = ("p", "li", "td", "th", "dd", "h1", "h2", "h3", "h4")
CHROME = ("nav", "header", "footer")


class P(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.ids, self.refs = set(), []
        self.chrome, self.block, self.body_links = 0, [], []

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if "id" in a: self.ids.add(a["id"])
        if tag == "a" and "name" in a: self.ids.add(a["name"])
        if tag in CHROME: self.chrome += 1
        if tag in BLOCKS and not self.chrome: self.block.append([[], []])
        for k in ("href", "src"):
            if k in a and a[k] is not None and tag in ("a", "link", "img", "script", "source", "iframe"):
                self.refs.append(a[k])
                if tag == "a" and self.block and not self.chrome and "pq-lang" not in (a.get("class") or ""):
                    self.block[-1][1].append(a[k])

    def handle_endtag(self, tag):
        if tag in CHROME and self.chrome: self.chrome -= 1
        if tag in BLOCKS and self.block and not self.chrome:
            texts, links = self.block.pop()
            text = "".join(texts)
            if self.block: self.block[-1][0].append(text)
            for l in links: self.body_links.append((l, text))

    def handle_data(self, data):
        if self.block: self.block[-1][0].append(data)


pages, raw_md = {}, []
for d, _, fs in os.walk(site):
    for f in fs:
        path = os.path.join(d, f)
        rel = os.path.relpath(path, site)
        if f.endswith(".html"):
            p = P(); p.feed(open(path, encoding="utf-8").read())
            pages[rel] = p
        elif f.endswith(".md") and os.sep not in rel:
            raw_md.append(rel)


def target(frm, ref):
    """사이트 안 대상(사이트 루트 기준 상대 경로)과 조각. 문제면 (None, 사유)."""
    u = urllib.parse.urlparse(ref)
    path = urllib.parse.unquote(u.path)
    if path.startswith("/"):
        if not (path == PREFIX or path.startswith(PREFIX + "/")):
            return None, "absolute path without the %s/ prefix" % PREFIX
        full = os.path.normpath(path[len(PREFIX):].lstrip("/")) if path[len(PREFIX):].strip("/") else ""
    else:
        full = os.path.normpath(os.path.join(os.path.dirname(frm), path)) if path else frm
    if full == ".": full = ""
    if full == ".." or full.startswith("../"):
        return None, "leaves the site"
    if full == "" or full.endswith("/") or os.path.isdir(os.path.join(site, full)):
        full = os.path.join(full, "index.html")
    if not os.path.exists(os.path.join(site, full)) and os.path.exists(os.path.join(site, full + ".html")):
        full += ".html"
    return full, u.fragment


cnt, bad = collections.Counter(), []
NEUTRAL = ("index.html", "ui-next.html")   # 손으로 쓴 쪽은 두 말을 함께 담아 언어 표시를 묻지 않는다
for pg, p in sorted(pages.items()):
    if pg in ("404.html", "ko/404.html"):   # 생성기의 404 쪽은 사이트 루트 링크와 건너뛰기 앵커를 일부러 쓴다
        continue
    for h in p.refs:
        if re.match(r"(https?:|mailto:|javascript:|data:)", h):
            cnt["external"] += 1
            if MAIN_LINK.match(h):
                bad.append((pg, h, "points at main on GitHub; a site is one version and should point at %s" % REF))
            continue
        if h == "#" or h == "":
            continue
        full, frag = target(pg, h)
        if full is None:
            bad.append((pg, h, frag)); continue
        if not os.path.exists(os.path.join(site, full)):
            bad.append((pg, h, "missing")); continue
        cnt["internal"] += 1
        if frag:
            tp = pages.get(full)
            if tp is not None and frag not in tp.ids:
                bad.append((pg, h, "anchor missing"))

MD_LINK = re.compile(r'\[(?:[^\[\]]|!\[[^\]]*\]\([^)]*\))*\]\(([^)\s]+)(?:\s+"[^"]*")?\)')
for md in sorted(raw_md):
    fence = False
    for line in open(os.path.join(site, md), encoding="utf-8"):
        if line.lstrip().startswith("```"):
            fence = not fence; continue
        if fence: continue
        for h in MD_LINK.findall(re.sub(r"`[^`]*`", "", line)):
            if re.match(r"(https?:|mailto:|#)", h):
                if MAIN_LINK.match(h): cnt["raw markdown -> main"] += 1
                continue
            path = urllib.parse.unquote(h.partition("#")[0])
            if path.startswith("/"):
                full, why = target(md, h)
                if full is None or not os.path.exists(os.path.join(site, full)):
                    bad.append((md, h, why if full is None else "missing"))
                continue
            rel = os.path.normpath(os.path.join("docs", path))
            if rel == ".." or rel.startswith("../"):
                bad.append((md, h, "raw markdown link leaves the repository")); continue
            if not os.path.exists(os.path.join(REPO, rel)):
                bad.append((md, h, "raw markdown link points to nothing in the repository")); continue
            cnt["raw markdown -> repository"] += 1

for pg, p in pages.items():
    if not pg.startswith("ko/"): continue
    for h, text in p.body_links:
        if re.match(r"(https?:|mailto:|javascript:|#)", h): continue
        full, _ = target(pg, h)
        if full and not full.startswith("ko/") and full in pages and full not in NEUTRAL and not full.startswith("architectures/"):
            cnt["Korean page -> English page"] += 1
            if not any(w in text for w in ("영문", "영어", "English")):
                bad.append((pg, h, "link to an English page without 영문/영어/English in the same block"))

n_ko = sum(1 for pg in pages if pg.startswith("ko/"))
print("html:", len(pages), "(Korean %d)" % n_ko, "· raw markdown:", len(raw_md))
for k, v in sorted(cnt.items()): print("  %-32s %d" % (k, v))
print("problems:", len(bad))
for b in bad[:60]: print("  ", b)
