English · [한국어](site.ko.md)

# The public site

Everything under `docs/` goes to GitHub Pages as it is ([workflow](../.github/workflows/pages.yml)). As in pqcota, the site's first
page (`index.html`) and the documents sit in one place (it used to be in `site/`). There is no build step: **static
files go up as they are.** There is one reason for having no static site generator: there is only one page now, and bringing a tool in
first means maintaining that tool first.

| Now | |
|---|---|
| [`index.html`](index.html) | The architecture document. A self-contained single page with no external request at all. Readable in Korean and English |
| [`ui-next.html`](ui-next.html) | **A past static mockup of the v0.22.0 screen design** (2026-08-26). That design became `pqcaton-ui`'s default screen from v0.22.0, and the real screen has changed in places since. It is kept as a record and reads or writes no data. Readable in Korean and English |

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

Rewrite `index.html` as an introduction page and move the current one to `architecture.html`. The `<nav>` at the top is the place kept
for that.

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
