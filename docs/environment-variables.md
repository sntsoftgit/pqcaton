English · [한국어](environment-variables.ko.md)

# Environment variables

[Overview](../README.md) · [Operator guide](operator-guide.md) · [Build](build.md)

The environment variables pqcaton reads. Secrets are read only from the environment (or from the file named below) and are not written to
the workspace, processing records, logs, error messages or the screen.

## Running

| Variable | Read by | Meaning |
|---|---|---|
| `PQCOTA_DSN` | `pqcaton-ui`, `pqcaton-decide baseline` · `activate-scope` | The connection of the pqcota history. **The screen does not start without it.** When it is empty, `pqcaton-ui` reads `pqcota.env` next to the default approval key (owner-only, one line `PQCOTA_DSN=<value>`; [operator guide section 2](operator-guide.md#2-installation)). Not passed to the observation processes the screen starts |
| `PQCOTA_ORG` | `pqcota-ingest` (upstream) | The organization pqcota ingests under. It must be the organization of the judgments, or the reconciliation in 05 does not find the ingest |
| `PQCATON_SCOPE_ASSETS` | `pqcaton-report` | The asset-scope policy file, the same one given to `pqcota-ingest -scope-assets` |
| `PQCATON_REQUIRE_RLS` | judgment ledger on Postgres | With `1`, the ledger does not open when row-level security is not in force (a superuser or `BYPASSRLS` role) ([design §2.2](developers.md#22-row-level-security-one-layer-the-db-enforces)) |
| `PQCATON_RUN` | set by `pqcaton-ui` | The marker the screen puts on the observation processes it starts, so it ends only processes proven to be its own. Not set by hand |

The approval keys of 06 are files, not environment variables (`approval.key` · `approval.pub`, or `-approval-key` ·
`-approval-verify-key`). Their content is the output of `pqcota-keygen` (`PQCOTA_SIGN_KEY` · `PQCOTA_VERIFY_KEY` lines). The upstream
commands `pqcota-approve` and `pqcota-provision` read `PQCOTA_APPROVAL_KEY` and `PQCOTA_APPROVAL_KEYS` themselves.

## Demo and checks

| Variable | Read by | Meaning |
|---|---|---|
| `PQCOTA_DIR` | `make e2e-demo`, demo scripts | The upstream `pqcota` checkout (the five repositories side by side at the version `go.mod` pins) |
| `PQCATON_BIN_DIR` | demo scripts | Prebuilt Linux binaries to use instead of building with Go |
| `PQCATON_UI_PORT` | `demo/scripts/ui.sh` | The port on this machine for the relay (default 8765) |
| `PQCATON_E2E_TRACE` | `demo/scripts/extend.sh` | With `1`, runs the dedicated fixture that measures the trace chain |
| `E2E_KEEP_WS` | `make e2e-demo` | A folder to copy the workspace to before cleaning up |
| `E2E_UBUNTU_MIRROR` | `make e2e-demo` | An Ubuntu mirror for the builds inside dind |
| `PQCOTA_TEST_DSN` | tests | A Postgres for the tests that need one; without it those tests are skipped |
