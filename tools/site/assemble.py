#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Great Honor <randyinthedev@gmail.com>
# SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
# SPDX-License-Identifier: Apache-2.0
"""pqcaton 리포의 문서로 사이트 원고 트리 하나를 모은다.

pqcota tools/site/assemble.py(8a4e9bf0, v0.13.0)에서 가져와 pqcaton에 맞게 고쳤다. 고친 내역은 docs/site.md에 있다.

usage: assemble.py <repo> <out-dir> --ref <tag-or-commit> --pages FILE [--lang en|ko] [--site-prefix /pqcaton]

리포에서 추적하는 Markdown 가운데 사이트에 싣는 것을 고정 목록(FILE, 「경로<TAB>사이트 자리」 줄)과 견주고, 다르면 멈춘다.
목록의 문서를 <out-dir>/docs에 옮기며 링크를 고친다. 사이트에 실린 문서끼리는 사이트 안 상대 링크가 되고, 그 밖의 리포
파일은 --ref의 GitHub 주소가 된다. **리포 밖으로 나가는 상대 링크와 없는 파일로 가는 링크는 멈춘다.** --lang ko는 추적하는
`*.ko.md`로 한국어 트리를 모은다(쪽은 영어 짝의 자리에 놓인다). 리포는 건드리지 않는다.
"""
import collections, json, os, re, shutil, subprocess, sys

ORG, REPO = "sntsoftgit", "pqcaton"
HOME = "overview.md"            # 사이트 맨 위는 손으로 쓴 첫 화면이 차지한다
SITE_PREFIX = "/pqcaton"
LANG = "en"
# 사이트에 싣지 않는 추적 문서. saas/는 이 사이트의 범위가 아니다. 옛 자리의 안내 쪽(stub)은 원문 주소 보존으로 남는다.
SKIP_PREFIX = ("saas/",)
SKIP = {"LICENSING.md", "docs/operator-guide.en.md"}
# 사이트가 Markdown이 아닌 자리로 옮겨 싣는 파일. 링크는 사이트 루트 기준 주소가 된다(첫 화면 · 화면 시안).
SITE_FILES = {"docs/index.html": "", "docs/ui-next.html": "ui-next.html"}

DEST = {
    "README.md": HOME,
    "docs/operator-guide.md": "users/operator-guide.md",
    "docs/journey.md": "users/journey.md",
    "docs/compatibility.md": "users/compatibility.md",
    "demo/README.md": "demo/index.md",
    "demo/expected-output/README.md": "demo/expected-output/index.md",
    "docs/developers.md": "developers/index.md",
    "docs/build.md": "developers/build.md",
    "docs/environment-variables.md": "developers/environment-variables.md",
    "docs/design.md": "developers/design.md",
    "docs/testcases.md": "developers/testcases.md",
    "docs/site.md": "developers/site.md",
    "docs/licensing.md": "developers/licensing.md",
    "CONTRIBUTING.md": "project/contributing.md",
    "SECURITY.md": "project/security.md",
    "CLA.md": "project/cla.md",
    "RELEASE_NOTES.md": "project/release-notes.md",
}


def english_of(path):
    return path[:-len(".ko.md")] + ".md" if path.endswith(".ko.md") else path


def discover(root, lang):
    files = subprocess.check_output(["git", "-C", root, "ls-files", "*.md"], text=True).split("\n")
    out = []
    for f in files:
        if not f or f.startswith(SKIP_PREFIX) or f in SKIP:
            continue
        if f.endswith(".ko.md") != (lang == "ko"):
            continue
        en = english_of(f)
        if en not in DEST:
            sys.exit("assemble: %s is tracked but has no site destination; add it to DEST or SKIP" % f)
        text = open(os.path.join(root, f), encoding="utf-8").read()
        m = re.search(r"^#\s+(.+?)\s*$", text, re.M)
        title = re.sub(r"[`*]", "", m.group(1)) if m else os.path.basename(f)
        out.append((f, DEST[en], title))
    return out


LINK = re.compile(r'(!?)\[((?:[^\[\]]|!\[[^\]]*\]\([^)]*\))*)\]\(([^)\s]+)((?:\s+"[^"]*")?)\)')
GH = re.compile(r'https://github\.com/%s/%s/(blob|tree)/(?:main|v[\w.]+)/([^#]*)(?:#(.*))?$' % (ORG, REPO))
MAP, OTHER = {}, {}   # 이 언어 트리의 (경로 → 자리), 다른 언어 트리의 (경로 → 자리)


def github_url(path, ref, kind="blob", anchor=""):
    return "https://github.com/%s/%s/%s/%s/%s" % (ORG, REPO, kind, ref, path) + ("#" + anchor if anchor else "")


def page_url(dest, lang, anchor=""):
    """다른 언어 트리의 쪽 · 사이트 파일로 가는 사이트 루트 기준 주소."""
    page = dest[:-len("index.md")] if dest.endswith("index.md") else dest[:-len(".md")] + "/"
    return SITE_PREFIX + ("/ko/" if lang == "ko" else "/") + page + ("#" + anchor if anchor else "")


def rewrite(text, src, ref, root, stats, problems):
    dest_src = MAP[src]
    out, fence = [], False
    for line in text.split("\n"):
        if line.lstrip().startswith("```"):
            fence = not fence
            out.append(line)
            continue
        if fence:
            out.append(line)
            continue

        def resolve(tpath, anchor, kind_hint=None):
            if tpath in MAP:
                stats["internal"] += 1
                return os.path.relpath(MAP[tpath], os.path.dirname(dest_src)) + ("#" + anchor if anchor else "")
            if tpath in OTHER:
                stats["other language"] += 1
                return page_url(OTHER[tpath], "en" if LANG == "ko" else "ko", anchor)
            if tpath in SITE_FILES:
                stats["site file"] += 1
                return SITE_PREFIX + "/" + SITE_FILES[tpath] + ("#" + anchor if anchor else "")
            full = os.path.join(root, tpath)
            if not os.path.exists(full):
                problems.append("%s: link to %s, which is not in the repository" % (src, tpath))
                return None
            stats["github at ref"] += 1
            return github_url(tpath, ref, kind_hint or ("tree" if os.path.isdir(full) else "blob"), anchor)

        def sub(m):
            bang, label, target, title = m.groups()
            if target.startswith("#"):
                stats["same page"] += 1
                return m.group(0)
            gm = GH.match(target)
            if gm:
                new = resolve(gm.group(2).rstrip("/"), gm.group(3) or "", gm.group(1))
                return m.group(0) if new is None else "%s[%s](%s%s)" % (bang, label, new, title)
            if re.match(r"(https?:|mailto:)", target):
                stats["external"] += 1
                return m.group(0)
            if target.startswith("/"):
                problems.append("%s: absolute link %s; write it relative to the file" % (src, target))
                return m.group(0)
            path, _, anchor = target.partition("#")
            tpath = os.path.normpath(os.path.join(os.path.dirname(src), path)) if path else src
            if tpath == ".." or tpath.startswith("../"):
                problems.append("%s: link %s leaves the repository" % (src, target))
                return m.group(0)
            full = os.path.join(root, tpath)
            if bang and os.path.isfile(full) and not tpath.endswith(".md"):
                asset = "assets/" + tpath
                os.makedirs(os.path.dirname(os.path.join(OUT, "docs", asset)), exist_ok=True)
                shutil.copyfile(full, os.path.join(OUT, "docs", asset))
                stats["image"] += 1
                return "%s[%s](%s%s)" % (bang, label, os.path.relpath(asset, os.path.dirname(dest_src)), title)
            new = resolve(tpath, anchor)
            return m.group(0) if new is None else "%s[%s](%s%s)" % (bang, label, new, title)

        out.append(LINK.sub(sub, line))
    return "\n".join(out)


GROUPS = ["@home", "users/", "demo/", "developers/", "project/"]
ORDER = ["users/operator-guide.md", "users/journey.md", "users/compatibility.md", "demo/index.md",
         "demo/expected-output/index.md", "developers/index.md", "developers/design.md", "developers/testcases.md",
         "developers/build.md", "developers/environment-variables.md", "developers/site.md", "developers/licensing.md",
         "project/release-notes.md", "project/contributing.md", "project/security.md", "project/cla.md"]


def order_key(dest):
    return (-1, dest) if dest == HOME else (ORDER.index(dest) if dest in ORDER else len(ORDER), dest)


OUT = ""


def main():
    global OUT, LANG, SITE_PREFIX, MAP, OTHER
    root, OUT = sys.argv[1], sys.argv[2]
    arg = lambda k, d=None: sys.argv[sys.argv.index(k) + 1] if k in sys.argv else d
    ref, pages_file = arg("--ref"), arg("--pages")
    if not ref or not pages_file:
        sys.exit("usage: assemble.py <repo> <out-dir> --ref <tag-or-commit> --pages FILE [--lang en|ko]")
    LANG = arg("--lang", "en")
    SITE_PREFIX = arg("--site-prefix", SITE_PREFIX).rstrip("/")
    pages = sorted(discover(root, LANG), key=lambda p: order_key(p[1]))
    # 다른 언어 트리의 쪽. 한국어 쪽에서 영어 파일로 건 링크(언어 줄 · 영어만 있는 문서)는 영어 사이트의 그 쪽으로 간다.
    OTHER = {p: d for p, d, _ in discover(root, "en" if LANG == "ko" else "ko")}
    listing = "".join("%s\t%s\n" % (p, d) for p, d, _ in pages)
    want = "".join(l for l in open(pages_file, encoding="utf-8") if l.strip() and not l.startswith("#"))
    if want != listing:
        import difflib
        sys.exit("page list differs from the pinned list:\n" + "".join(
            difflib.unified_diff(want.splitlines(1), listing.splitlines(1), "pinned", "found")))
    MAP = {p: d for p, d, _ in pages}
    stats, problems = collections.Counter(), []
    for src, dest, _ in pages:
        text = open(os.path.join(root, src), encoding="utf-8").read()
        text = rewrite(text, src, ref, root, stats, problems)
        path = os.path.join(OUT, "docs", dest)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        open(path, "w", encoding="utf-8").write(text)
    if problems:
        sys.exit("assemble: %d link problem(s):\n  " % len(problems) + "\n  ".join(problems))
    json.dump({"ref": ref, "lang": LANG, "pages": len(pages), "stats": dict(stats)},
              open(os.path.join(OUT, "link-report.json"), "w"), indent=1)
    write_nav(OUT, pages)
    print("pages:", len(pages), dict(sorted(stats.items())))


def write_nav(out, pages):
    ko = LANG == "ko"
    title = {d: t for _, d, t in pages}
    yq = lambda t: '"%s"' % t.replace('"', "'")
    lines = ["nav:", "  - %s: %s" % ("개요" if ko else "Overview", HOME)]
    for label, prefix in ([("사용자용", "users/"), ("데모", "demo/"), ("개발자용", "developers/"), ("프로젝트", "project/")] if ko else
                          [("For users", "users/"), ("Demo", "demo/"), ("For developers", "developers/"), ("Project", "project/")]):
        ds = [d for _, d, _ in pages if d.startswith(prefix)]
        if ds:
            lines.append("  - %s:" % label)
            lines += ["      - %s: %s" % (yq(title[d]), d) for d in ds]
    open(os.path.join(out, "nav.yml"), "w", encoding="utf-8").write("\n".join(lines) + "\n")


main()
