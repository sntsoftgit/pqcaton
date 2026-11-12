English · [한국어](build.ko.md)

# Building from source

[Overview](../README.md) · [Environment variables](environment-variables.md) · [Contributing](../CONTRIBUTING.md)

## What you need

- **Go**: the version in [`go.mod`](../go.mod). No other runtime is needed for building and testing.
- **Docker**: only for `make check-reuse` (license headers) and `make e2e-demo` (the demo from scratch).
- The upstream pqcota modules come through the Go module proxy at the version `go.mod` pins; no sibling checkout or `replace` is needed.

## Build and test

```bash
make
```

`make` runs the gates first and stops at the first one that fails: licenses, text in code, document style, the two languages of the
site pages, documents (pairs and links), cases (numbers and tests), format, module zip and the release-check tool. Then it builds and runs
the tests. CI runs the same command.

To build only the commands:

```bash
go build -o bin/ ./inventory/cmd/...
```

## Generated files

The screen's templates (`.templ`) are kept together with their generated files (`*_templ.go`), so a build needs no generator. After
changing a `.templ`, regenerate and check, and commit the generated files in the same commit.

```bash
make generate
make verify-generated
```

## Other checks

| Command | What it checks |
|---|---|
| `make check-reuse` | Every file has a license and copyright (REUSE, runs in Docker) |
| `make e2e-demo` | Starts the pqcota demo from scratch and runs the policy activation, judging, approval and generation to the end (needs `PQCOTA_DIR`; see the [demo](../demo/README.md)) |
| `make release-check VERSION=vX.Y.Z` | Everything before tagging a release, including that the candidate commit's required GitHub workflows (ci, site) all succeeded, ending with a summary, the module `h1:` hash and the release body (`release-body.md`: the English section with tag-pinned links and a link to the same version in the Korean notes on top) |
| `make release-verify VERSION=vX.Y.Z COMMIT=… H1=…` | After publishing: the tag, the module proxy, the checksum database and installing from them, and that the release body equals the English section and its Korean link opens |
