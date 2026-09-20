package report

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/idunn-cloud/norn/internal/engine"
	"github.com/idunn-cloud/norn/internal/policy"
)

func TestSARIF(t *testing.T) {
	findings := []engine.Finding{
		{
			PolicyID:    "az-storage-min-tls12",
			Address:     "azurerm_storage_account.logs",
			Type:        "azurerm_storage_account",
			Action:      "create",
			Status:      engine.StatusFail,
			Severity:    "medium",
			Enforcement: policy.Mandatory,
			OnUnknown:   "warn",
			Message:     "Storage accounts must require TLS 1.2 or newer.",
			Source:      "policies/azure/storage.yaml",
		},
		{
			PolicyID:    "az-keyvault-no-delete",
			Address:     "azurerm_key_vault.legacy",
			Type:        "azurerm_key_vault",
			Action:      "delete",
			Status:      engine.StatusPass,
			Severity:    "high",
			Enforcement: policy.Overridable,
			OnUnknown:   "warn",
			Message:     "This plan destroys a Key Vault.",
		},
	}

	var buf bytes.Buffer
	summary, err := SARIF(&buf, findings, nil, "v0.1.0", "plan.json")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Blocking != 1 {
		t.Fatalf("blocking = %d, want 1", summary.Blocking)
	}

	var got struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != "2.1.0" {
		t.Fatalf("version = %q, want 2.1.0", got.Version)
	}
	if len(got.Runs) != 1 || got.Runs[0].Tool.Driver.Name != "norn" {
		t.Fatalf("unexpected tool metadata: %+v", got.Runs)
	}
	if len(got.Runs[0].Results) != 1 {
		t.Fatalf("results = %d, want 1", len(got.Runs[0].Results))
	}
	if got.Runs[0].Results[0].RuleID != "az-storage-min-tls12" || got.Runs[0].Results[0].Level != "error" {
		t.Fatalf("unexpected result: %+v", got.Runs[0].Results[0])
	}
}
