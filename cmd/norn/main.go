// Command norn decides the fate of a Terraform or OpenTofu plan before apply.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/idunn/norn/internal/engine"
	"github.com/idunn/norn/internal/plan"
	"github.com/idunn/norn/internal/policy"
	"github.com/idunn/norn/internal/report"
)

var version = "dev" // set at build time: -ldflags "-X main.version=v0.1.0"

const usage = `norn: policy checks for Terraform and OpenTofu plans, written in CEL

Usage:
  norn check --plan plan.json [--policies ./policies] [flags]
  norn version

Create the plan file with:
  terraform plan -out tfplan && terraform show -json tfplan > plan.json

Exit codes: 0 no blocking findings, 1 blocking findings, 2 usage or configuration error.
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "check":
		return check(args[1:], stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "norn: unknown command %q\n\n%s", args[0], usage)
	return 2
}

func check(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	planPath := fs.String("plan", "", "plan JSON from `terraform show -json` (required)")
	policyPath := fs.String("policies", "policies", "policy file or directory")
	format := fs.String("format", "text", "output format: text or json")
	overrideList := fs.String("override", "", "comma-separated IDs of overridable policies to override for this run")
	verbose := fs.Bool("verbose", false, "also list passing checks (text output)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	fail := func(err error) int {
		fmt.Fprintf(stderr, "norn: %v\n", err)
		return 2
	}
	if *planPath == "" {
		return fail(errors.New("--plan is required"))
	}
	if *format != "text" && *format != "json" {
		return fail(fmt.Errorf("unknown --format %q (want text or json)", *format))
	}

	policies, err := policy.Load(*policyPath)
	if err != nil {
		return fail(err)
	}
	overrides, err := parseOverrides(*overrideList, policies)
	if err != nil {
		return fail(err)
	}
	eng, err := engine.New(policies)
	if err != nil {
		return fail(err)
	}
	p, err := plan.Load(*planPath)
	if err != nil {
		return fail(err)
	}

	findings := eng.Evaluate(p)
	var sum report.Summary
	if *format == "json" {
		if sum, err = report.JSON(stdout, findings, overrides); err != nil {
			return fail(err)
		}
	} else {
		sum = report.Text(stdout, findings, overrides, *verbose)
	}
	if sum.Blocking > 0 {
		return 1
	}
	return 0
}

// parseOverrides only accepts IDs of overridable policies, so a typo or an
// attempt to override a mandatory policy is an error rather than a silent no-op.
func parseOverrides(list string, ps []policy.Policy) (map[string]bool, error) {
	byID := make(map[string]policy.Enforcement, len(ps))
	for _, p := range ps {
		byID[p.ID] = p.Enforcement
	}
	out := map[string]bool{}
	for _, id := range strings.Split(list, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		enf, ok := byID[id]
		switch {
		case !ok:
			return nil, fmt.Errorf("--override: no policy with id %q", id)
		case enf != policy.Overridable:
			return nil, fmt.Errorf("--override: policy %q is %s and cannot be overridden", id, enf)
		}
		out[id] = true
	}
	return out, nil
}
