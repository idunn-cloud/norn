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

## Exit codes

`0` means no blocking findings, `1` means blocking findings, and `2` means a usage, policy or plan error.

## Roadmap

- SARIF output and GitHub/GitLab annotations
- `norn test`: unit tests for policies against fixture plans
- Exceptions with owners and expiry dates
- Helper functions: CIDR/IP checks, port-range expansion, `changed("attr")`, relationship lookups
- Policy bundles distributed as OCI artifacts
- AWS and GCP rule packs

## License

Apache-2.0
