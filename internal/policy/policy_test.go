package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePolicy(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const header = "apiVersion: norn.idunn.cloud/v1alpha1\nkind: PolicySet\npolicies:\n"

func TestLoadAppliesDefaults(t *testing.T) {
	ps, err := Load(writePolicy(t, header+`
  - id: ok
    severity: low
    enforcement: advisory
    match: {types: [azurerm_*]}
    expression: "true"
    message: fine
`))
	if err != nil {
		t.Fatal(err)
	}
	p := ps[0]
	if p.OnUnknown != "warn" || p.Match.Mode != "managed" || len(p.Match.Actions) != 3 {
		t.Fatalf("defaults not applied: %+v", p)
	}
}

func TestLoadRejectsBadPolicies(t *testing.T) {
	cases := map[string]string{
		"unknown field":   "  - id: a\n    severity: low\n    enforcment: mandatory\n    match: {types: [x]}\n    expression: \"true\"\n    message: m\n",
		"bad enforcement": "  - id: a\n    severity: low\n    enforcement: sometimes\n    match: {types: [x]}\n    expression: \"true\"\n    message: m\n",
		"no types":        "  - id: a\n    severity: low\n    enforcement: mandatory\n    expression: \"true\"\n    message: m\n",
		"bad action":      "  - id: a\n    severity: low\n    enforcement: mandatory\n    match: {types: [x], actions: [destroy]}\n    expression: \"true\"\n    message: m\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writePolicy(t, header+body)); err == nil {
				t.Fatal("expected an error")
			} else if !strings.Contains(err.Error(), "p.yaml") {
				t.Fatalf("error should name the file: %v", err)
			}
		})
	}
}
