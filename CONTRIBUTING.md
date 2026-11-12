English · [한국어](CONTRIBUTING.ko.md)

# Contributing: pqcaton

Issues and PRs are welcome. This repository is under a **source-available commercial license** (BUSL-1.1)
(→ [LICENSING](docs/licensing.md)), so code contributions take one extra step. Please read this first.

## Contributor License Agreement (CLA)

**Before you send code, you must agree to the [CLA](CLA.md).** A bot guides you on your first PR.
The Korean original is [CLA.ko.md](CLA.ko.md); where the two texts differ, the Korean original governs.

### Why it is needed

**Each pqcaton release converts to Apache-2.0 four years after its publication.** The license promises that, so the company must have
the right to release all of the code under Apache-2.0 on that day.

If the code you send does not carry that right, **the promise cannot be kept.** A single line mixed in blocks that file. Fixing it
afterwards means asking every contributor again, and if one person cannot be reached, the only option is to take that code out. So we
ask **from the start**.

A commercial license agreement rests on the same right.

### What you agree to

- The code you send is **yours**, or you have the right to submit it
- If you work for a company, **the company has allowed it** (employment contracts often assign code to the company)
- SNT Soft Co., Ltd. may release that code **under BUSL-1.1, and under Apache-2.0 after the Change Date**

**The copyright stays with the contributor.** It is a permission, not a transfer. You may use your code freely elsewhere.

If you cannot agree, **tell us in an issue instead of code.** Steps to reproduce and the behavior you expected are already a real help.

## Adding a dependency

**Only permissive licenses are accepted**: MIT, Apache-2.0 and the BSD family. GPL, LGPL and AGPL libraries cannot be linked.

The reason is not taste. Linking a copyleft library **makes the whole derivative follow its terms**, which blocks both distributing
under BUSL and converting to Apache-2.0 on the Change Date.

**It is not only Go modules.** The `.js` and `.css` the screen serves to the browser pass the same gate. They ship inside the binary, so
they are part of the distribution. If you add such a file, write its path and SPDX identifier in `licenses.txt`, and **put the original
LICENSE text it rests on next to the file** (`pkg/inventory/ui/static/LICENSE.htmx` is an example). Files we wrote are `BUSL-1.1`.

Do not replace them with CDN links. The screen breaks on a machine without a network, and a script coming from someone else's server
is **something this gate cannot even see.**

CI checks this (`make check-licenses`). If the gate stops you, find an alternative, or discuss in an issue how to call it as a separate
process.

## Which language to use

| | Language | Why |
|---|---|---|
| **Comments** | Korean | This is where the repository records **why it did what it did**. Translating thins that judgment |
| **Documents** (`docs/`, `README`) | **English by default with a Korean pair** (`x.md` · `x.ko.md`) | The same shape as pqcota. The first lines of the two editions point to each other, and their commands, version numbers and case IDs must be the same (`make check-docs`) |
| **Identifiers and code** | English | The common convention |
| **Command output, logs, errors** | **English** | People paste them into searches and issues. If the languages differ, one problem is left as two sentences |
| **File formats** (JSON keys, session values) | **English** | Files are read by a screen opened in another language too |
| **Screen** | **Korean · English** | The user chooses |
| **Text in tests** | Korean | What a case measures is read by the people who build this repository |

`make check-text` enforces this: a Korean character in a code string stops the build. **It reads the code with a parser, not a regular
expression.** A quick script once cut lines at the `//` inside a URL and missed one `"화면: http://%s"`, and missed two in multi-line
backtick strings.

**`make check-prose` checks the style of documents and screen texts.** It keeps a word that was removed once from coming back. The engine
is upstream pqcota's `tools/checkprose`, run at the version the Makefile pins (`PROSE_CHECKER`). This repository holds only the
settings: what is blocked is in `tools/checkprose/rules.tsv` with the word to use instead for each rule, the screen-text files to read
besides Markdown are in `files.txt`, and words the rules catch wrongly are in `overlap.txt`. What exists today is recorded per file in
`baseline.tsv`, and **any increase is blocked.** When a fix reduces it, lower the baseline with `make prose-baseline` and commit it in
the same commit. `notices.tsv` is a notice, not a gate: it only lists candidates the machine cannot judge because they are correct in
headings and tables (a spaced hyphen " - "), and passes even when they appear. Look at them with
`go run github.com/randyinthedev-hash/pqcota/tools/checkprose@v0.9.1 -dir tools/checkprose -list` and decide by eye (keep the version
the same as the Makefile's `PROSE_CHECKER`). The engine's cases are upstream; this repository runs that engine in CI with its own
settings.

**`make check-docs` checks document pairs and links.** An English document `x.md` and a Korean document `x.ko.md` live as a pair
(documents allowed without a pair are listed with a reason in `tools/checkdocs/single.txt`), and their first lines are language lines
pointing to each other. Relative links and `#anchors` inside the repository must exist, Korean documents link to Korean editions and
English documents to English editions. If the version numbers, case IDs or shell commands of the two editions differ, it blocks.

**`make check-cases` checks that case numbers match real tests.** Write the number in a test comment (`// IC-R1`) and the gate matches it
against the table in the document. Writing it on only one side is blocked. Do not attach the links on numbers by hand; make them with
`go run ./tools/checkcases -write` and put them in the same commit. The case table is the English edition (`docs/testcases.md`), and
the gate counts cases there only once. The Korean edition (`docs/testcases.ko.md`) must state the same facts row by row, so after
re-stamping links run `go run ./tools/checkdocs -sync-cases` to align its first cells.

### Sentences that show on the screen and are printed by commands

Finalize refusals, the declaration's problem list and observation warnings are like that. Left in English, the Korean screen gets
English mixed in; left in Korean, the command speaks Korean.

**Make them values, not sentences.** And one rule:

> **The English belongs to the package that owns the fact, and the Korean belongs to the screen catalog (`pkg/inventory/ui/text*.go`).**

With English in two places, one day only one side gets fixed, and that day the command and the screen describe the same problem
differently. Code without Korean goes out in English, which is better than a blank.

Examples are [`review.Warning`](pkg/inventory/review/build.go) (a code and an English sentence) and
[`ui/text_more.go`](pkg/inventory/ui/text_more.go) (the Korean for that code). The declaration's problem list
([`decl.Problem`](pkg/inventory/decl/decl.go)) has only English, because the command prints where the problem is and the screen
only reports the count.

### Adding screen texts

Write **both languages side by side** as `T{KO: …, EN: …}` in `pkg/inventory/ui/text.go`. Splitting them into files means one day only
one side gets fixed. **IC-UI19** catches a missing one: it draws the whole screen in English and blocks if a single Korean character
remains.

## Changing the screen (`.templ`)

Templates are written with [templ](https://templ.guide), and **the generated files (`*_templ.go`) are kept in the repository.** So a
build needs no generator, and whoever runs `go get` on this repository installs no tool.

If you changed a `.templ`:

```
make generate          # regenerate *_templ.go
make verify-generated  # catch the day you changed it and forgot to run it
```

Put the generated files **in the same commit**. If you leave them out, the source is new but the screen shows the old one, and what you
see no longer matches.

## About the base project (pqcota)

Observation, normalization and generating changes are done by [pqcota](https://github.com/randyinthedev-hash/pqcota). If you need a
fix there or a change to its contracts (`contracts/`), **open an issue in the pqcota repository directly.** We do not take it here and
pass it on. That repository has a different owner, and a discussion through its public process opens the same door to its other
consumers.

## Commits and PRs

- A commit message says **what changed and why**, in Korean or English
- One change per PR, split to a reviewable size
- When you change code that has tests, change the tests with it
- Run **`make`** before sending. It passes the license, text, style, document, case and format (`gofmt`) gates and goes on to build and
  test. If you changed the screen (`.templ`), also run `make verify-generated`. **CI runs the same commands.** Green at your desk is
  green there too
- New source files (`.go`, `.sh`, `.py`, workflows) start with the two SPDX header lines, the same as the first two lines of other
  source files. If you forget, `make check-reuse` blocks it (it needs Docker, and CI runs it). For a new kind of file that cannot carry
  a header, such as a document or a setting, add it to `REUSE.toml`

## Contact

- Bugs and suggestions → [Issues](https://github.com/sntsoftgit/pqcaton/issues)
- Commercial license → **kty@sntsoft.co.kr**
- Security vulnerabilities → do not open an issue; use the routes in [SECURITY.md](SECURITY.md)
