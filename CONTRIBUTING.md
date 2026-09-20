# Contributing to Norn

Thanks for taking a look at Norn.

Norn is still in an early alpha stage, so small focused contributions are more useful than broad
redesigns. Please open an issue or discussion first if you want to make a significant change to
policy format, evaluation semantics, or CLI behavior.

## Local development

Prerequisites:

- Go `1.23+`

Common commands:

```sh
go test ./...
go build ./...
go run ./cmd/norn check --plan testdata/plan.json --policies ./policies
go run ./cmd/norn test --policies ./policies --tests ./testdata/tests
```

## Repository layout

- `cmd/norn/` — CLI entrypoint
- `internal/policy/` — policy schema, loading, normalization
- `internal/plan/` — Terraform/OpenTofu plan loading
- `internal/engine/` — CEL environment, compilation, evaluation, unknown handling
- `internal/report/` — text, JSON, and SARIF output
- `internal/policytest/` — fixture-based policy tests
- `policies/azure/` — starter Azure rule packs
- `testdata/` — plan fixtures and policy test suites
- `docs/` — roadmap and supporting project documentation

## Contribution guidelines

### Engine and CLI changes

- Keep changes focused and minimal.
- Prefer adding tests alongside behavior changes.
- Preserve deterministic output and exit codes.
- Avoid introducing network or server dependencies into the core evaluation path.

### Policy contributions

If you add or modify a policy:

- place it under the appropriate policy pack, currently `policies/azure/`
- add or update fixture coverage in `testdata/tests/`
- prefer clear messages over terse ones
- document assumptions in the policy description when needed

### Fixture guidance

- use sanitized plans only
- include edge cases when possible: `create`, `update`, `replace`, `delete`, nested objects, and unknown values
- keep fixtures small enough to understand, but realistic enough to prevent regressions

## Pull requests

A good pull request usually includes:

- the problem being solved
- the behavior change
- any policy or CLI compatibility impact
- tests or fixture updates

If your change affects policy authoring or output shape, please update `README.md` and any relevant docs.

## Reporting issues

When filing bugs, include:

- the command you ran
- the Norn version or commit
- a sanitized plan snippet or reproducer if possible
- the policy that triggered the problem

## Code of collaboration

Be direct, kind, and technical. Early-stage tools improve fastest when feedback is specific and reproducible.
