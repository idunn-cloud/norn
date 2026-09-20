// Package policytest runs fixture-based tests for Norn policies.
package policytest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/idunn/norn/internal/engine"
	"github.com/idunn/norn/internal/plan"
	"github.com/idunn/norn/internal/policy"
	"github.com/idunn/norn/internal/report"
	"gopkg.in/yaml.v3"
)

const (
	APIVersion = "norn.idunn.cloud/v1alpha1"
	Kind       = "PolicyTest"
)

// Suite is a fixture file containing one or more evaluation cases.
type Suite struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Cases []Case `yaml:"cases"`

	Source string `yaml:"-"`
}

// Case runs the loaded policies against a single plan and checks expected outcomes.
type Case struct {
	Name      string        `yaml:"name"`
	Plan      string        `yaml:"plan"`
	Overrides []string      `yaml:"overrides"`
	Want      []Expectation `yaml:"want"`
}

// Expectation describes one expected policy result.
type Expectation struct {
	Policy  string `yaml:"policy"`
	Address string `yaml:"address"`
	Outcome string `yaml:"outcome"`
}

type key struct {
	policy  string
	address string
}

// Failure describes one mismatch in a test case.
type Failure struct {
	Policy  string `json:"policy"`
	Address string `json:"address"`
	Want    string `json:"want,omitempty"`
	Got     string `json:"got,omitempty"`
	Problem string `json:"problem"`
}

// CaseResult is the evaluation result for one test case.
type CaseResult struct {
	Suite    string    `json:"suite"`
	Name     string    `json:"name"`
	Plan     string    `json:"plan"`
	Passed   bool      `json:"passed"`
	Failures []Failure `json:"failures,omitempty"`
}

// Summary aggregates the test run.
type Summary struct {
	Cases  int `json:"cases"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	Suites int `json:"suites"`
}

// RunResult contains every case result plus totals.
type RunResult struct {
	Cases   []CaseResult `json:"cases"`
	Summary Summary      `json:"summary"`
}

// LoadSuites reads a single test file or every .yaml/.yml file under a directory.
func LoadSuites(root string) ([]Suite, error) {
	files, err := yamlFiles(root)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no test files found in %s", root)
	}

	var (
		suites []Suite
		errs   []error
	)
	for _, file := range files {
		suite, err := decodeSuite(file)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		suite.Source = file
		if err := normalizeSuite(&suite); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		base := filepath.Dir(file)
		for i := range suite.Cases {
			if !filepath.IsAbs(suite.Cases[i].Plan) {
				suite.Cases[i].Plan = filepath.Clean(filepath.Join(base, suite.Cases[i].Plan))
			}
		}
		suites = append(suites, suite)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return suites, nil
}

func yamlFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("tests: %w", err)
	}
	if !info.IsDir() {
		return []string{root}, nil
	}
	var files []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ext := filepath.Ext(p); !d.IsDir() && (ext == ".yaml" || ext == ".yml") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func decodeSuite(file string) (Suite, error) {
	f, err := os.Open(file)
	if err != nil {
		return Suite{}, err
	}
	defer f.Close()

	var suite Suite
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&suite); err != nil {
		return Suite{}, fmt.Errorf("%s: %w", file, err)
	}
	return suite, nil
}

func normalizeSuite(s *Suite) error {
	if s.APIVersion != APIVersion || s.Kind != Kind {
		return fmt.Errorf("expected apiVersion %q and kind %q", APIVersion, Kind)
	}
	if len(s.Cases) == 0 {
		return errors.New("cases must contain at least one test case")
	}
	seen := map[string]bool{}
	for i := range s.Cases {
		c := &s.Cases[i]
		if c.Name == "" {
			return errors.New("each case must have a name")
		}
		if seen[c.Name] {
			return fmt.Errorf("duplicate case name %q", c.Name)
		}
		seen[c.Name] = true
		if c.Plan == "" {
			return fmt.Errorf("case %q: plan is required", c.Name)
		}
		for _, w := range c.Want {
			switch strings.ToUpper(w.Outcome) {
			case "FAIL", "ERROR", "WARN", "OVERRIDDEN", "UNKNOWN", "PASS":
			default:
				return fmt.Errorf("case %q: outcome %q is invalid", c.Name, w.Outcome)
			}
			if w.Policy == "" || w.Address == "" {
				return fmt.Errorf("case %q: each expectation needs policy and address", c.Name)
			}
		}
	}
	return nil
}

// Run evaluates all suites against the provided policies.
func Run(w io.Writer, suites []Suite, policies []policy.Policy) (RunResult, error) {
	eng, err := engine.New(policies)
	if err != nil {
		return RunResult{}, err
	}

	result := RunResult{Summary: Summary{Suites: len(suites)}}
	for _, suite := range suites {
		fmt.Fprintf(w, "suite %s\n", suite.Source)
		for _, c := range suite.Cases {
			cr, err := runCase(eng, suite, c, policies)
			if err != nil {
				return RunResult{}, err
			}
			result.Cases = append(result.Cases, cr)
			result.Summary.Cases++
			if cr.Passed {
				result.Summary.Passed++
				fmt.Fprintf(w, "  PASS %s\n", c.Name)
				continue
			}
			result.Summary.Failed++
			fmt.Fprintf(w, "  FAIL %s\n", c.Name)
			for _, f := range cr.Failures {
				fmt.Fprintf(w, "       - %s on %s", f.Policy, f.Address)
				if f.Want != "" || f.Got != "" {
					fmt.Fprintf(w, ": want %s, got %s", f.Want, f.Got)
				}
				if f.Problem != "" {
					fmt.Fprintf(w, " (%s)", f.Problem)
				}
				fmt.Fprintln(w)
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "norn test: %d case(s), %d passed, %d failed\n", result.Summary.Cases, result.Summary.Passed, result.Summary.Failed)
	return result, nil
}

func runCase(eng *engine.Engine, suite Suite, c Case, policies []policy.Policy) (CaseResult, error) {
	overrides, err := parseOverrides(c.Overrides, policies)
	if err != nil {
		return CaseResult{}, fmt.Errorf("%s case %q: %w", suite.Source, c.Name, err)
	}
	p, err := plan.Load(c.Plan)
	if err != nil {
		return CaseResult{}, fmt.Errorf("%s case %q: %w", suite.Source, c.Name, err)
	}
	findings := eng.Evaluate(p)
	actual := map[key]report.Outcome{}
	for _, f := range findings {
		actual[key{policy: f.PolicyID, address: f.Address}] = report.OutcomeOf(f, overrides)
	}
	expected := map[key]string{}
	for _, w := range c.Want {
		expected[key{policy: w.Policy, address: w.Address}] = strings.ToUpper(w.Outcome)
	}

	res := CaseResult{Suite: suite.Source, Name: c.Name, Plan: c.Plan, Passed: true}
	for k, want := range expected {
		got, ok := actual[k]
		if !ok {
			res.Passed = false
			res.Failures = append(res.Failures, Failure{Policy: k.policy, Address: k.address, Want: want, Problem: "missing result"})
			continue
		}
		if string(got) != want {
			res.Passed = false
			res.Failures = append(res.Failures, Failure{Policy: k.policy, Address: k.address, Want: want, Got: string(got), Problem: "unexpected outcome"})
		}
	}
	for k, got := range actual {
		if got == report.Pass {
			continue
		}
		if _, ok := expected[k]; ok {
			continue
		}
		res.Passed = false
		res.Failures = append(res.Failures, Failure{Policy: k.policy, Address: k.address, Got: string(got), Problem: "unexpected non-pass result"})
	}
	sort.Slice(res.Failures, func(i, j int) bool {
		if res.Failures[i].Policy != res.Failures[j].Policy {
			return res.Failures[i].Policy < res.Failures[j].Policy
		}
		return res.Failures[i].Address < res.Failures[j].Address
	})
	return res, nil
}

func parseOverrides(ids []string, ps []policy.Policy) (map[string]bool, error) {
	byID := make(map[string]policy.Enforcement, len(ps))
	for _, p := range ps {
		byID[p.ID] = p.Enforcement
	}
	out := map[string]bool{}
	for _, id := range ids {
		enf, ok := byID[id]
		switch {
		case !ok:
			return nil, fmt.Errorf("override: no policy with id %q", id)
		case enf != policy.Overridable:
			return nil, fmt.Errorf("override: policy %q is %s and cannot be overridden", id, enf)
		}
		out[id] = true
	}
	return out, nil
}
