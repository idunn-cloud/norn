// Package report renders findings for humans (text) and machines (JSON).
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/idunn-cloud/norn/internal/engine"
	"github.com/idunn-cloud/norn/internal/policy"
)

// Outcome is what a finding means for this run, after enforcement and overrides.
type Outcome string

const (
	Fail       Outcome = "FAIL"
	Error      Outcome = "ERROR"
	Warn       Outcome = "WARN"
	Overridden Outcome = "OVERRIDDEN"
	Unknown    Outcome = "UNKNOWN"
	Pass       Outcome = "PASS"
)

// Summary counts outcomes. Blocking > 0 means the run should fail.
type Summary struct {
	Blocking   int `json:"blocking"`
	Warnings   int `json:"warnings"`
	Overridden int `json:"overridden"`
	Unknown    int `json:"unknown"`
	Passed     int `json:"passed"`
}

// OutcomeOf applies enforcement, onUnknown and overrides to a raw finding.
func OutcomeOf(f engine.Finding, overrides map[string]bool) Outcome {
	if f.Status == engine.StatusUnknown && f.OnUnknown != "fail" {
		if f.OnUnknown == "pass" {
			return Pass
		}
		return Unknown
	}
	switch {
	case f.Status == engine.StatusPass:
		return Pass
	case f.Blocking(overrides) && f.Status == engine.StatusError:
		return Error
	case f.Blocking(overrides):
		return Fail
	case f.Enforcement == policy.Overridable && f.Status != engine.StatusError:
		return Overridden
	}
	return Warn
}

// Summarize counts outcomes across all findings.
func Summarize(fs []engine.Finding, overrides map[string]bool) Summary {
	var s Summary
	for _, f := range fs {
		switch OutcomeOf(f, overrides) {
		case Fail, Error:
			s.Blocking++
		case Warn:
			s.Warnings++
		case Overridden:
			s.Overridden++
		case Unknown:
			s.Unknown++
		case Pass:
			s.Passed++
		}
	}
	return s
}

// Text prints findings grouped under their outcome, then a one-line summary.
func Text(w io.Writer, fs []engine.Finding, overrides map[string]bool, verbose bool) Summary {
	for _, f := range fs {
		o := OutcomeOf(f, overrides)
		if o == Pass && !verbose {
			continue
		}
		fmt.Fprintf(w, "%-10s %s  [%s/%s]\n", o, f.PolicyID, f.Severity, f.Enforcement)
		fmt.Fprintf(w, "%-10s %s (%s)\n", "", f.Address, f.Action)
		switch {
		case f.Error != "":
			fmt.Fprintf(w, "%-10s evaluation error: %s\n", "", f.Error)
		case o == Unknown:
			fmt.Fprintf(w, "%-10s %s (depends on values known only after apply)\n", "", f.Message)
		case o != Pass:
			fmt.Fprintf(w, "%-10s %s\n", "", f.Message)
		}
		fmt.Fprintln(w)
	}
	s := Summarize(fs, overrides)
	fmt.Fprintf(w, "norn: %d blocking, %d warning(s), %d overridden, %d unknown, %d passed\n",
		s.Blocking, s.Warnings, s.Overridden, s.Unknown, s.Passed)
	return s
}

type jsonFinding struct {
	engine.Finding
	Outcome  Outcome `json:"outcome"`
	Blocking bool    `json:"blocking"`
}

// JSON writes every finding (passes included) plus the summary.
func JSON(w io.Writer, fs []engine.Finding, overrides map[string]bool) (Summary, error) {
	out := struct {
		Findings []jsonFinding `json:"findings"`
		Summary  Summary       `json:"summary"`
	}{Findings: make([]jsonFinding, 0, len(fs)), Summary: Summarize(fs, overrides)}
	for _, f := range fs {
		o := OutcomeOf(f, overrides)
		out.Findings = append(out.Findings, jsonFinding{Finding: f, Outcome: o, Blocking: o == Fail || o == Error})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return out.Summary, enc.Encode(out)
}
