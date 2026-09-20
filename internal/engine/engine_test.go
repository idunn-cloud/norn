package engine

import (
	"testing"

	"github.com/idunn/norn/internal/plan"
	"github.com/idunn/norn/internal/policy"
)

func TestEvaluateFixture(t *testing.T) {
	ps, err := policy.Load("../../policies")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(ps)
	if err != nil {
		t.Fatal(err)
	}
	p, err := plan.Load("../../testdata/plan.json")
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]Status{}
	for _, f := range eng.Evaluate(p) {
		if f.Error != "" {
			t.Errorf("%s on %s: unexpected error: %s", f.PolicyID, f.Address, f.Error)
		}
		got[f.PolicyID+" "+f.Address] = f.Status
	}

	want := map[string]Status{
		"az-nsg-no-public-admin-ports azurerm_network_security_rule.ssh_open": StatusFail,
		"az-nsg-no-public-admin-ports azurerm_network_security_rule.from_lb":  StatusUnknown,
		"az-storage-https-only azurerm_storage_account.logs":                  StatusPass,
		"az-storage-min-tls12 azurerm_storage_account.logs":                   StatusFail,
		"az-storage-no-public-nested-items azurerm_storage_account.logs":      StatusPass,
		"az-keyvault-no-delete azurerm_key_vault.legacy":                      StatusFail,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: got %q, want %q", k, got[k], w)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected finding %s = %q", k, got[k])
		}
	}
}

func TestBlocking(t *testing.T) {
	overrides := map[string]bool{"ovr": true}
	cases := []struct {
		name string
		f    Finding
		want bool
	}{
		{"mandatory fail", Finding{PolicyID: "m", Status: StatusFail, Enforcement: policy.Mandatory}, true},
		{"advisory fail", Finding{PolicyID: "a", Status: StatusFail, Enforcement: policy.Advisory}, false},
		{"overridable, not overridden", Finding{PolicyID: "x", Status: StatusFail, Enforcement: policy.Overridable}, true},
		{"overridable, overridden", Finding{PolicyID: "ovr", Status: StatusFail, Enforcement: policy.Overridable}, false},
		{"unknown, warn", Finding{PolicyID: "m", Status: StatusUnknown, Enforcement: policy.Mandatory, OnUnknown: "warn"}, false},
		{"unknown, fail", Finding{PolicyID: "m", Status: StatusUnknown, Enforcement: policy.Mandatory, OnUnknown: "fail"}, true},
		{"error fails closed", Finding{PolicyID: "m", Status: StatusError, Enforcement: policy.Mandatory}, true},
		{"advisory error", Finding{PolicyID: "a", Status: StatusError, Enforcement: policy.Advisory}, false},
	}
	for _, c := range cases {
		if got := c.f.Blocking(overrides); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
