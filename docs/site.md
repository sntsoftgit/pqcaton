English · [한국어](site.ko.md)

# The public site

Today everything under `docs/` goes to GitHub Pages as it is ([workflow](../.github/workflows/pages.yml)). As in pqcota, the site's
first page (`index.html`) and the documents sit in one place. **A documentation site built from the repository's documents is being
added** ([below](#the-documentation-site)); until it is deployed from a release tag, the live site stays as it is.

| Now | |
|---|---|
| [`index.html`](index.html) | The architecture document. A self-contained single page with no external request at all. Readable in Korean and English |
| [`ui-next.html`](ui-next.html) | **A past static mockup of the v0.22.0 screen design** (2026-08-26). That design became `pqcaton-ui`'s default screen from v0.22.0, and the real screen has changed in places since. It is kept as a record and reads or writes no data. Readable in Korean and English |

## The documentation site

The repository's documents (pinned in [`pages.txt`](../tools/site/pages.txt) and [`pages.ko.txt`](../tools/site/pages.ko.txt)) become an
English site and a Korean site under `/pqcaton/ko/` ([`tools/site`](../tools/site)). **The source documents in the repository are the
reference; the site is a read-only copy of one release tag, and where the two differ, the source wins.** Generated files are never
committed: the [site workflow](../.github/workflows/site.yml) builds and checks the site on main and keeps it only as an Actions
artifact, without deploying.

### Which version a build is

`build.sh` does not trust any input for the version. A build is a **release** build only when exactly one `vX.Y.Z` tag points at
`HEAD`, that tag's commit is `HEAD`, and the working tree is clean. Everything else is a **development** build: every page gets a
"Development build: not a release. Do not deploy." band and `noindex`. `build-info.txt` at the top of the site records the kind, the
tag, the full commit, the Python and Zensical versions and the hashes of the pinned requirements and of `pip freeze`. Deploying from a
release tag only, and never rebuilding a tag's site with other content, is the next step; an urgent fix to the documents goes out as a
patch release.

### What the build checks

- Python is the pinned version ([`python-version`](../tools/site/python-version)), the tools are installed only from
  [`requirements.txt`](../tools/site/requirements.txt) with hashes, and `pip freeze` equals [`freeze.txt`](../tools/site/freeze.txt).
- The tracked documents equal the pinned page lists. A link that leaves the repository, points to nothing or is an absolute path stops
  the build.
- After the build, every link in every HTML page (the generated pages, the first page and `ui-next.html`) resolves inside the site, with
  its anchor. An absolute path without the `/pqcaton/` prefix or a relative path that leaves the site is a problem, and so is a link to
  this repository's `main` on GitHub. The links of the source documents kept at their old addresses resolve to files in the repository.
  A Korean page linking to an English page says so (영문, 영어 or English) in the same block. The check must report 0 problems.
- Heading anchors follow GitHub's rule ([`slugs.py`](../tools/site/slugs.py)), so an anchor written for GitHub opens on the site too.

### Addresses kept

`/pqcaton/` stays the first page and `/pqcaton/ui-next.html` stays where it is. Every `docs/*.md` is also copied byte for byte to the
top of the site, so the addresses that open the source today keep working. The generated pages live under `overview/`, `users/`,
`demo/`, `developers/` and `project/`. The `saas/` documents are not part of this site.

### Where the tools come from

`assemble.py`, `build.sh`, `check_site.py`, `langswitch.py` and the two mkdocs files come from pqcota's `tools/site` at commit
`8a4e9bf060416533fab9d40dcce8427d81e10faa` (v0.13.0) and stay under the Apache License, Version 2.0 (their headers say so, the
license text is [LICENSES/Apache-2.0.txt](../LICENSES/Apache-2.0.txt), and [NOTICE](../NOTICE) records it). pqcaton pins its own copy; it
does not fetch pqcota's tools when building. The changes:

| File | Changed for pqcaton |
|---|---|
| `assemble.py` | One repository instead of five, pqcaton's destinations, `saas/` and the old-address pages left out, the first page and `ui-next.html` as site files, and a link that leaves the repository, points to nothing or is absolute stops the build instead of being kept |
| `build.sh` | The version from the tag that points at `HEAD` (development otherwise), the Python and `pip freeze` checks, all `docs/*.md` kept at their old addresses, the hand-written pages' links to `main` pinned to the build's version, and `stamp.py` |
| `check_site.py` | Also `<link>`, `<img>` and `<script>`, the kept source documents, the missing prefix and leaving the site as named problems, links to this repository's `main`, and the language marker words |
| `langswitch.py` | `ui-next.html` is left alone as a hand-written page |
| `mkdocs.base.yml` · `mkdocs.ko.base.yml` | No logo or favicon lines, and GitHub's heading anchors |

`stamp.py`, `slugs.py`, `local.sh`, `python-version`, `freeze.txt` and the hashes in `requirements.txt` are new in pqcaton.

To build it locally (Docker; the output stays outside the repository and nothing is deployed):

```bash
tools/site/local.sh ~/pqcaton-sandbox/site-out/main
```

## Rules

- **Keep it self-contained.** No CDN scripts, web fonts or remote images. Double-clicking the file offline must open it as it is.
- **Write in two languages.** Texts go in pairs of `<span lang="ko">` and `<span lang="en">`, and CSS shows one side. **Write both
  languages when you add a text.** Writing only one leaves a blank when viewed in the other. Texts not shown on screen (`title`,
  `aria-label`, `placeholder`, `data-toast`) go into `-ko` and `-en` attributes and a script swaps them. The chosen language is
  remembered by both pages under the same identifier.
- **English may use em dashes.** The [style gate](../tools/checkprose) looks only at lines that contain Korean. `index.html` puts the two
  languages on different lines so the English lines fall outside the gate, and `ui-next.html`, whose labels are short and share a line,
  does not use em dashes in English instead.
- **Draw both light and dark.** Colors come only from the tokens in `:root`. A color defined only inside a media query leaves text
  invisible in one of the themes.
- **Links from the pages to repository documents are absolute GitHub URLs.** Pages does not render `.md` and serves the source as it is,
  so a relative link to `developers.md` in the same place opens hard-to-read source. Relative links and `#anchors` between pages, and the
  repository paths that absolute URLs point to, are checked by `make check-docs`.

## When widening it into a home page

Rewrite `index.html` as an English-first introduction page and move the current one to `architectures/`, keeping its address on
the site as `/pqcaton/architectures/` (the second step of the site rework). The `<nav>` at the top is the place kept for that.

## It is live

**https://www.sntsoft.co.kr/pqcaton/**: the organization has a custom domain (`www.sntsoft.co.kr`), so it is not `github.io`. The source
is GitHub Actions and HTTPS is enforced.

The organization is on the Team plan, so **it deploys from a private repository too.** But know the price:

> **Even if the repository is private, the deployed site is public.** Anyone who knows the URL sees it. Pages with access control are
> available only on Enterprise Cloud.

So `docs/` holds **only what may be public**. The repository is public, so the documents in `docs/` going out with the site reveal
nothing new, but a secret or an internal record placed in this folder would be published as it is.

`ui-next.html` is also a static mockup that may be public. It is a draft of the v0.22.0 screen design drawn from the `demo-corp` screen of
2026-08-26, and it does not access the declaration, CSV files or the judgment ledger, nor finalize anything. See the current screen by
starting `pqcaton-ui`.
