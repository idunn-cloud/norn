package report

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/idunn/norn/internal/engine"
	"github.com/idunn/norn/internal/policy"
)

// SARIF writes findings in SARIF 2.1.0 format for code scanning systems.
// Passing findings are omitted.
func SARIF(w io.Writer, fs []engine.Finding, overrides map[string]bool, toolVersion, planPath string) (Summary, error) {
	summary := Summarize(fs, overrides)
	rules := sarifRules(fs)
	results := sarifResults(fs, overrides, planPath)

	out := sarifLog{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "norn",
				InformationURI: "https://github.com/idunn-cloud/norn",
				Version:        toolVersion,
				Rules:          rules,
			}},
			Results: results,
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return summary, enc.Encode(out)
}

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri,omitempty"`
	Version        string      `json:"version,omitempty"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string           `json:"id"`
	Name             string           `json:"name,omitempty"`
	ShortDescription sarifMessage     `json:"shortDescription,omitempty"`
	FullDescription  *sarifMessage    `json:"fullDescription,omitempty"`
	Help             *sarifMessage    `json:"help,omitempty"`
	DefaultConfig    *sarifRuleConfig `json:"defaultConfiguration,omitempty"`
	Properties       sarifProperties  `json:"properties,omitempty"`
}

type sarifRuleConfig struct {
	Level string `json:"level,omitempty"`
}

type sarifProperties struct {
	Severity    string `json:"severity,omitempty"`
	Enforcement string `json:"enforcement,omitempty"`
	OnUnknown   string `json:"onUnknown,omitempty"`
}

type sarifResult struct {
	RuleID           string                 `json:"ruleId"`
	Level            string                 `json:"level,omitempty"`
	Kind             string                 `json:"kind,omitempty"`
	Message          sarifMessage           `json:"message"`
	Locations        []sarifLocation        `json:"locations,omitempty"`
	LogicalLocations []sarifLogicalLocation `json:"logicalLocations,omitempty"`
	Properties       map[string]any         `json:"properties,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifLogicalLocation struct {
	Name string `json:"name,omitempty"`
	Kind string `json:"kind,omitempty"`
}

func sarifRules(fs []engine.Finding) []sarifRule {
	seen := map[string]sarifRule{}
	for _, f := range fs {
		if _, ok := seen[f.PolicyID]; ok {
			continue
		}
		r := sarifRule{
			ID:               f.PolicyID,
			Name:             f.PolicyID,
			ShortDescription: sarifMessage{Text: f.Message},
			DefaultConfig:    &sarifRuleConfig{Level: sarifLevel(f)},
			Properties: sarifProperties{
				Severity:    f.Severity,
				Enforcement: string(f.Enforcement),
				OnUnknown:   f.OnUnknown,
			},
		}
		if f.Source != "" {
			r.Help = &sarifMessage{Text: fmt.Sprintf("Defined in %s", f.Source)}
		}
		seen[f.PolicyID] = r
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rules := make([]sarifRule, 0, len(ids))
	for _, id := range ids {
		rules = append(rules, seen[id])
	}
	return rules
}

func sarifResults(fs []engine.Finding, overrides map[string]bool, planPath string) []sarifResult {
	uri := filepath.ToSlash(planPath)
	results := make([]sarifResult, 0, len(fs))
	for _, f := range fs {
		o := OutcomeOf(f, overrides)
		if o == Pass {
			continue
		}
		msg := f.Message
		if f.Error != "" {
			msg = fmt.Sprintf("%s: %s", msg, f.Error)
		}
		if o == Unknown {
			msg = fmt.Sprintf("%s (depends on values known only after apply)", msg)
		}
		res := sarifResult{
			RuleID:           f.PolicyID,
			Level:            sarifLevelForOutcome(o),
			Kind:             "fail",
			Message:          sarifMessage{Text: fmt.Sprintf("%s [%s %s] on %s (%s)", msg, f.Severity, f.Enforcement, f.Address, f.Action)},
			LogicalLocations: []sarifLogicalLocation{{Name: f.Address, Kind: "resource"}},
			Properties: map[string]any{
				"address":      f.Address,
				"resourceType": f.Type,
				"action":       f.Action,
				"severity":     f.Severity,
				"enforcement":  f.Enforcement,
				"status":       f.Status,
				"outcome":      o,
				"onUnknown":    f.OnUnknown,
			},
		}
		if uri != "" {
			res.Locations = []sarifLocation{{PhysicalLocation: sarifPhysicalLocation{ArtifactLocation: sarifArtifactLocation{URI: uri}}}}
		}
		results = append(results, res)
	}
	return results
}

func sarifLevel(f engine.Finding) string {
	switch f.Enforcement {
	case policy.Mandatory:
		return "error"
	case policy.Overridable:
		return "warning"
	default:
		return "note"
	}
}

func sarifLevelForOutcome(o Outcome) string {
	switch o {
	case Fail, Error:
		return "error"
	case Unknown, Warn:
		return "warning"
	default:
		return "note"
	}
}
