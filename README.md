# Norn

[![GitHub Release](https://img.shields.io/github/v/release/idunn-cloud/norn?include_prereleases)](https://github.com/idunn-cloud/norn/releases)

**Decide your plan's fate before apply.**

Norn checks Terraform and OpenTofu plans against policies written in [CEL](https://cel.dev),
the same expression language Kubernetes uses for ValidatingAdmissionPolicy. It ships as one
binary with no server and no Rego, and it runs anywhere a plan file does.

> **Status:** Norn is an **early alpha**.
> The core evaluator works, but policy format, helper functions, outputs, and packaging may
> change before a stable `v1`. Today the project is Azure-first and aimed at early adopters who
> want to evaluate CEL against Terraform/OpenTofu plan JSON and help shape the tool.

```yaml
- id: az-storage-min-tls12
  severity: medium
  enforcement: mandatory
  match:
    types: [azurerm_storage_account]
  expression: after.min_tls_version in ["TLS1_2", "TLS1_3"]
  message: Storage accounts must require TLS 1.2 or newer.
```

## Why Norn exists

Terraform policy tooling is still dominated by Rego-based systems, Sentinel, or scanners that
are not built around authoring and evaluating CEL policy directly against plan JSON. Norn takes a
simpler path:

- **CEL instead of Rego** for straightforward, readable policies
- **local plan evaluation** with no server dependency
- **Terraform and OpenTofu support** through `show -json`
- **honest unknown handling** for values only known after apply
- **open-source enforcement semantics** with `advisory`, `overridable`, and `mandatory`

## What works today

- `norn check` for Terraform/OpenTofu plan JSON
- YAML policy sets with compile-time validation
- CEL-based policy evaluation over `before`, `after`, `resource`, and `resources`
- Enforcement levels: `advisory`, `overridable`, `mandatory`
- Unknown-after-apply handling via CEL partial evaluation
- Output formats:
  - `text`
  - `json`
  - `sarif`
- `norn test` for fixture-based policy testing
- Starter Azure policy pack under `policies/azure/`

## Not here yet

These are planned, but not finished in the current alpha:

- exception files with owner and expiry
- richer Terraform-specific helper functions like `changed("field")`
- relationship helpers for cross-resource rules
- GitHub Action and packaged releases
- OCI policy bundles
- broader AWS/GCP rule packs
- polished explainable findings with exact attribute/value breakdowns

## Quickstart

Until the first packaged release exists, run Norn from source or install it with Go.

```sh
go install github.com/idunn-cloud/norn/cmd/norn@latest

terraform plan -out tfplan
terraform show -json tfplan > plan.json
norn check --plan plan.json --policies ./policies
norn check --plan plan.json --policies ./policies --format sarif > results.sarif
```

Example output:

```text
FAIL       az-nsg-no-public-admin-ports  [high/mandatory]
           azurerm_network_security_rule.ssh_open (create)
           SSH/RDP must not be reachable from the internet; use Bastion or a restricted source range.

UNKNOWN    az-nsg-no-public-admin-ports  [high/mandatory]
           azurerm_network_security_rule.from_lb (create)
           SSH/RDP must not be reachable from the internet; use Bastion or a restricted source range. (depends on values known only after apply)
```

## Writing policies

A policy's `expression` returns `true` when the change is compliant. Available variables:

| Variable    | Contents                                                                  |
|-------------|---------------------------------------------------------------------------|
| `after`     | Planned attributes (`null` on delete)                                     |
| `before`    | Attributes before the change (`null` on create)                           |
| `resource`  | `address`, `type`, `name`, `mode`, `provider`, `module`, `action`         |
| `resources` | Every resource change in the plan, each with its metadata and `after`     |

`match.types` accepts globs (`azurerm_*`). `match.actions` defaults to `create`, `update`
and `replace`; set it to `[delete, replace]` to guard destructive changes.

### Enforcement

- `advisory`: reported as a warning and never blocks.
- `overridable`: blocks unless the run passes `--override <policy-id>`.
- `mandatory`: always blocks.

Only overridable policies can be overridden, so overriding a mandatory policy or an unknown
ID is an error. If a policy fails to evaluate, the run fails closed unless the policy is advisory.

### Values known only after apply

Norn maps Terraform's `after_unknown` onto CEL partial evaluation. Checks that can be decided
from known values are decided; checks that genuinely depend on unknown values report
`UNKNOWN` instead of a wrong pass. `onUnknown: warn | fail | pass` sets how each policy
treats that case (default `warn`).

## Testing policies

Fixture tests let policy authors lock behavior against real plan JSON.

```sh
norn test --policies ./policies --tests ./testdata/tests
```

```yaml
apiVersion: norn.idunn.cloud/v1alpha1
kind: PolicyTest
metadata:
  name: smoke
cases:
  - name: default-fixture
    plan: ../plan.json
    want:
      - policy: az-nsg-no-public-admin-ports
        address: azurerm_network_security_rule.ssh_open
        outcome: FAIL
```

Expectations are matched by `policy` + `address`. Unlisted `PASS` results are ignored, but any
unexpected non-pass result fails the test, so new warnings and failures do not slip in silently.

## Output formats

`norn check --format text|json|sarif`

- `text`: human-readable CLI output.
- `json`: all findings plus the run summary.
- `sarif`: SARIF 2.1.0 for GitHub code scanning and other scanners.

## Development

```sh
go test ./...
go run ./cmd/norn check --plan testdata/plan.json --policies ./policies
go run ./cmd/norn test --policies ./policies --tests ./testdata/tests
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for contribution workflow notes.

## Exit codes

`norn check`: `0` means no blocking findings, `1` means blocking findings, and `2` means a usage, policy or plan error.

`norn test`: `0` means all test cases passed, `1` means at least one case failed, and `2` means a usage, policy or fixture error.

## Roadmap

See [`docs/roadmap.md`](docs/roadmap.md) for the concrete repo-mapped delivery plan.

- GitHub/GitLab annotations
- Exceptions with owners and expiry dates
- Helper functions: CIDR/IP checks, port-range expansion, `changed("attr")`, relationship lookups
- Policy bundles distributed as OCI artifacts
- AWS and GCP rule packs

## Contributing

Issues and early feedback are welcome. If you want to contribute rules or engine changes, please
read [`CONTRIBUTING.md`](CONTRIBUTING.md) first.

## License

Apache-2.0. See [`LICENSE`](LICENSE).
