# Norn

**Decide your plan's fate before apply.**

Norn checks Terraform and OpenTofu plans against policies written in [CEL](https://cel.dev),
the same expression language Kubernetes uses for ValidatingAdmissionPolicy. It ships as one
binary with no server and no Rego, and it runs anywhere a plan file does.

```yaml
- id: az-storage-min-tls12
  severity: medium
  enforcement: mandatory
  match:
    types: [azurerm_storage_account]
  expression: after.min_tls_version in ["TLS1_2", "TLS1_3"]
  message: Storage accounts must require TLS 1.2 or newer.
```

## Quickstart

```sh
go install github.com/idunn/norn/cmd/norn@latest

terraform plan -out tfplan
terraform show -json tfplan > plan.json
norn check --plan plan.json --policies ./policies
norn check --plan plan.json --policies ./policies --format sarif > results.sarif
```

```
FAIL       az-nsg-no-public-admin-ports  [high/mandatory]
           azurerm_network_security_rule.ssh_open (create)
           SSH/RDP must not be reachable from the internet; use Bastion or a restricted source range.

UNKNOWN    az-nsg-no-public-admin-ports  [high/mandatory]
           azurerm_network_security_rule.from_lb (create)
           SSH/RDP must not be reachable from the internet; ... (depends on values known only after apply)
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

## Exit codes

`norn check`: `0` means no blocking findings, `1` means blocking findings, and `2` means a usage, policy or plan error.

`norn test`: `0` means all test cases passed, `1` means at least one case failed, and `2` means a usage, policy or fixture error.

## Roadmap

- GitHub/GitLab annotations
- Exceptions with owners and expiry dates
- Helper functions: CIDR/IP checks, port-range expansion, `changed("attr")`, relationship lookups
- Policy bundles distributed as OCI artifacts
- AWS and GCP rule packs

## License

Apache-2.0
