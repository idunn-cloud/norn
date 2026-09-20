package policytest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/idunn-cloud/norn/internal/policy"
)

func TestLoadSuites(t *testing.T) {
	suites, err := LoadSuites("../../testdata/tests")
	if err != nil {
		t.Fatal(err)
	}
	if len(suites) != 1 {
		t.Fatalf("got %d suites, want 1", len(suites))
	}
	if len(suites[0].Cases) != 2 {
		t.Fatalf("got %d cases, want 2", len(suites[0].Cases))
	}
	if !strings.HasSuffix(suites[0].Cases[0].Plan, "testdata/plan.json") {
		t.Fatalf("plan path was not resolved relative to the suite: %q", suites[0].Cases[0].Plan)
	}
}

func TestRun(t *testing.T) {
	ps, err := policy.Load("../../policies")
	if err != nil {
		t.Fatal(err)
	}
	suites, err := LoadSuites("../../testdata/tests")
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	result, err := Run(&out, suites, ps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Failed != 0 {
		t.Fatalf("expected green fixture run, got %+v\n%s", result.Summary, out.String())
	}
	if !strings.Contains(out.String(), "PASS default-fixture") {
		t.Fatalf("expected human output to mention passing cases, got:\n%s", out.String())
	}
}
