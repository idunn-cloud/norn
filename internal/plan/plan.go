// Package plan loads Terraform and OpenTofu plans in their JSON representation.
package plan

import (
	"encoding/json"
	"fmt"
	"os"

	tfjson "github.com/hashicorp/terraform-json"
)

// Load reads a plan produced by `terraform show -json tfplan` (or `tofu show -json`).
func Load(path string) (*tfjson.Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	var p tfjson.Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse plan %s: %w (generate it with: terraform show -json tfplan > plan.json)", path, err)
	}
	return &p, nil
}
