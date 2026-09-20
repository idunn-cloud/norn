# Norn roadmap

This roadmap turns the current proof-of-concept into a viable policy tool for real CI/CD use.
It is intentionally mapped to the current repository layout so work can start without a redesign.

## Current baseline

Norn already has a solid core:

- CLI entrypoints in `cmd/norn/main.go`
- policy loading and validation in `internal/policy/`
- Terraform/OpenTofu plan loading in `internal/plan/`
- CEL compilation and evaluation in `internal/engine/`
- text, JSON, and SARIF output in `internal/report/`
- fixture-based policy tests in `internal/policytest/`
- starter Azure policy packs in `policies/azure/`

The next step is not a new architecture. It is improving authoring ergonomics, explainability, CI integration, and trust.

## Prioritization rules

1. Prefer features that reduce time-to-first-success for policy authors.
2. Prefer features that make failures easier to trust and debug.
3. Preserve the existing shape of the engine unless a change unlocks multiple roadmap items.
4. Keep Terraform/OpenTofu plan evaluation local and deterministic.

## Delivery plan

- **P0**: required for serious trial use
- **P1**: makes Norn pleasant and scalable in teams

---

## P0-1. Explainable findings and better failure messages

**Goal**

Make every failure and unknown result actionable without reading the policy source or raw plan JSON.

**Why this matters**

Right now findings are accurate, but they mostly return the policy's static `message`. That is enough for demos, but not enough for repeated team use. Teams will trust Norn much faster if the output identifies the exact attribute, observed value, and unknown path involved.

**Scope**

- Add structured finding details:
  - failing attribute path
  - observed value when known
  - expected condition or operator
  - unknown path when the result is undecidable
- Support policy-side message templates such as:
  - `${after.min_tls_version}`
  - `${resource.address}`
- Improve text, JSON, and SARIF renderers to emit those details consistently.

**Repository changes**

- Extend `internal/engine/Finding` in `internal/engine/engine.go`
- Add template rendering utilities, likely in new package `internal/render/` or `internal/policy/message.go`
- Update `internal/report/report.go`
- Update `internal/report/sarif.go`
- Add fixture coverage in `internal/engine/engine_test.go` and `internal/report/`

**Suggested implementation**

Start small:

1. Add optional policy fields:
   - `messageTemplate`
   - `unknownMessageTemplate`
2. Add helper functions for safe interpolation against `before`, `after`, and `resource`
3. Populate `Finding.Details` with machine-readable metadata for all formats

**Definition of done**

- A failed TLS rule can report the actual value, for example `TLS1_0`
- An unknown NSG rule can report the unknown field, for example `after.source_address_prefix`
- JSON and SARIF include structured detail fields, not only flattened prose

---

## P0-2. Exceptions with owner and expiry

**Goal**

Support temporary, auditable exceptions so teams can adopt Norn without disabling entire policies.

**Why this matters**

This is one of the clearest gaps between a useful OSS prototype and a tool teams will keep in CI. Without exceptions, users either bypass the pipeline or weaken policies globally.

**Scope**

- Add exception files in YAML
- Match exceptions by policy ID and resource address at minimum
- Support metadata:
  - `owner`
  - `reason`
  - `expires`
  - optional `ticket`
- Expired exceptions fail closed
- Exceptions are visible in text, JSON, SARIF, and tests

**Repository changes**

- Add new package `internal/exception/`
- Extend `cmd/norn/main.go` with `--exceptions`
- Apply exception logic near outcome calculation in `internal/report/report.go`
- Add test support in `internal/policytest/policytest.go`
- Add sample fixtures under `testdata/exceptions/`

**Suggested file shape**

```yaml
apiVersion: norn.idunn.cloud/v1alpha1
kind: ExceptionSet
exceptions:
  - policy: az-keyvault-no-delete
    address: azurerm_key_vault.legacy
    owner: platform-security
    reason: Approved migration window
    expires: 2026-10-31
    ticket: SEC-123
```

**Definition of done**

- Overridden findings distinguish between CLI override and persisted exception
- Expired exceptions are treated as configuration errors or blocking failures
- `norn test` can validate policies with and without exceptions

---

## P0-3. Terraform-specific CEL helpers

**Goal**

Reduce raw-plan friction by giving authors primitives that match how they think about Terraform changes.

**Why this matters**

This is the biggest adoption lever after explainability. CEL alone is simpler than Rego for many engineers, but raw Terraform plan structure is still awkward.

**Scope**

Start with a narrow, high-value helper set:

- `changed("field")`
- `created()`
- `updated()`
- `deleted()`
- `replaced()`
- `is_unknown("field")`
- `any_non_null([a, b, c])`
- network helpers for Azure-first rule packs:
  - `is_public_cidr(string)`
  - `port_matches(value, ["22", "3389"])`

**Repository changes**

- Add new package `internal/celfunc/` or `internal/engine/functions.go`
- Wire helpers into `NewEnv()` in `internal/engine/engine.go`
- Expand policy docs in `README.md`
- Simplify some existing policies in `policies/azure/*.yaml`

**Design note**

Keep helper behavior pure and deterministic. Helpers should not mutate state or perform I/O.

**Definition of done**

- Existing Azure rules can be rewritten to be shorter and clearer
- At least one fixture test proves `changed()` and one proves network helpers
- README contains a short helper reference

---

## P0-4. CI packaging and GitHub Action

**Goal**

Make the first successful CI integration take minutes, not an afternoon.

**Why this matters**

Distribution is part of viability. If installation is clumsy, teams will not trial the engine long enough to appreciate the policy model.

**Scope**

- Add a GitHub Action wrapper
- Add release automation for multi-platform binaries
- Add a Docker image build path
- Add an example workflow that:
  - runs `norn check`
  - emits SARIF
  - uploads SARIF to code scanning

**Repository changes**

- New `action.yml`
- New `.github/workflows/ci.yml`
- New `.github/workflows/release.yml`
- New `.goreleaser.yml`
- Optional `Dockerfile`
- README install and CI sections

**Definition of done**

- A public repo can adopt Norn with one action block and a policy path
- Tagged releases produce binaries and checksums
- SARIF upload example is copy-pasteable

---

## P0-5. Rule-pack quality and compatibility corpus

**Goal**

Prove Norn works on real plans and ships rules worth adopting.

**Why this matters**

The engine can be elegant and still fail adoption if the initial policy pack feels thin or brittle.

**Scope**

- Grow Azure pack from starter rules to a curated baseline
- Add sanitized real-world fixtures covering:
  - create, update, replace, delete
  - `for_each`
  - `count`
  - nested lists and maps
  - module addresses
  - unknown values deep in objects and lists
- Add a compatibility matrix to the docs

**Repository changes**

- Add `testdata/corpus/`
- Add more `PolicyTest` suites in `testdata/tests/`
- Expand `policies/azure/`
- Add docs page such as `docs/compatibility.md`

**Definition of done**

- At least 20 strong Azure policies exist with fixture coverage
- The test corpus includes multiple realistic plans, not just a single smoke plan
- Compatibility expectations for Terraform/OpenTofu versions are documented

---

## P1-1. Relationship queries and resource indexing

**Goal**

Enable non-trivial cross-resource policies without forcing authors into inefficient `resources.filter(...)` expressions everywhere.

**Why this matters**

Cross-resource rules are where policy engines become strategically valuable, but they also become hard to author and easy to slow down.

**Scope**

- Build an indexed plan view by address, type, and module
- Expose helper patterns such as:
  - `resources.of_type("azurerm_subnet")`
  - `resources.with_action("create")`
  - `resources.by_address("...")`
- Later, consider typed relationship helpers for common Azure joins

**Repository changes**

- Add indexing layer in new package `internal/graph/` or `internal/planview/`
- Feed indexed structures into `internal/engine/engine.go`
- Add helper registration in CEL env setup
- Add benchmark coverage

**Definition of done**

- At least one policy uses a relationship helper instead of a raw list scan
- Large plans do not regress badly in evaluation time

---

## P1-2. Additional outputs: JUnit and native annotations

**Goal**

Support more CI systems without forcing SARIF everywhere.

**Why this matters**

SARIF is strong, but not universal. JUnit and lightweight annotation formats are often easier to wire into build summaries.

**Scope**

- Add `--format junit`
- Add GitHub annotation format
- Add GitLab code quality or annotation-style output if feasible
- Keep output packages parallel to current text/JSON/SARIF pattern

**Repository changes**

- Extend `cmd/norn/main.go`
- Add `internal/report/junit.go`
- Add `internal/report/annotations.go`
- Add tests beside existing report tests

**Definition of done**

- At least one CI system can render Norn failures as test failures via JUnit
- GitHub annotation output highlights policy violations in workflow logs

---

## P1-3. Parallel evaluation and benchmarks

**Goal**

Scale cleanly to large plans and larger policy sets.

**Why this matters**

The current engine is simple and correct, but it is a nested loop. That is fine for MVP, but eventually users will run dozens of policies against hundreds or thousands of resources.

**Scope**

- Parallelize evaluation across resource changes and/or policy batches
- Preserve deterministic output ordering
- Add benchmarks for small, medium, and large plans
- Measure memory overhead of resource indexing and helper layers

**Repository changes**

- Update `internal/engine/engine.go`
- Add `*_bench_test.go` under `internal/engine/`
- Add a representative large synthetic or sanitized plan under `testdata/corpus/`

**Definition of done**

- Benchmark results are documented in PRs or docs
- Parallel execution improves throughput without changing result order
- No data races under `go test -race ./...`

---

## P1-4. Policy authoring guide and examples

**Goal**

Make policy writing self-serve.

**Why this matters**

Even strong helpers and outputs will underperform if authors do not know the data model, unknown semantics, and common plan JSON traps.

**Scope**

- Add a dedicated guide for:
  - `before`, `after`, `resource`, `resources`
  - unknown values and `after_unknown`
  - deletes and replaces
  - helper functions
  - fixture testing with `norn test`
- Add several complete policy examples with compliant and non-compliant plans

**Repository changes**

- Add `docs/authoring.md`
- Link it from `README.md`
- Possibly add `testdata/examples/`

**Definition of done**

- A new contributor can write and test a policy without reading engine code
- README stays concise and links to deeper docs

---

## Recommended execution order

1. **Explainable findings**
2. **Exceptions**
3. **Terraform-specific CEL helpers**
4. **CI packaging and GitHub Action**
5. **Rule-pack quality and compatibility corpus**
6. **Relationship queries and indexing**
7. **Additional outputs**
8. **Parallel evaluation and benchmarks**
9. **Authoring guide**

This order front-loads trust and usability, then invests in scale and ecosystem fit.

## Suggested first milestone

A good "v0.2 viable trial" milestone would include:

- explainable findings
- exceptions
- Terraform helpers
- GitHub Action
- at least 20 Azure policies with corpus-backed tests

That would be enough for a real team to trial Norn in CI without immediately hitting the most obvious adoption blockers.
