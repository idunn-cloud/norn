// Package engine compiles CEL policies and evaluates them against plan resource changes.
package engine

import (
	"errors"
	"fmt"
	"path"
	"slices"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/ext"
	tfjson "github.com/hashicorp/terraform-json"

	"github.com/idunn/norn/internal/policy"
)

// Status is the raw result of evaluating one policy against one resource change.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusUnknown Status = "unknown" // result depends on values known only after apply
	StatusError   Status = "error"
)

// Finding is one policy evaluated against one resource change.
type Finding struct {
	PolicyID    string             `json:"policy_id"`
	Address     string             `json:"address"`
	Type        string             `json:"type"`
	Action      string             `json:"action"`
	Status      Status             `json:"status"`
	Severity    string             `json:"severity"`
	Enforcement policy.Enforcement `json:"enforcement"`
	OnUnknown   string             `json:"on_unknown"`
	Message     string             `json:"message"`
	Error       string             `json:"error,omitempty"`
	Source      string             `json:"source"`
}

// Blocking reports whether the finding should fail the run.
// Evaluation errors fail closed unless the policy is advisory.
func (f Finding) Blocking(overrides map[string]bool) bool {
	switch f.Status {
	case StatusError:
		return f.Enforcement != policy.Advisory
	case StatusFail:
		return f.enforced(overrides)
	case StatusUnknown:
		return f.OnUnknown == "fail" && f.enforced(overrides)
	}
	return false
}

func (f Finding) enforced(overrides map[string]bool) bool {
	switch f.Enforcement {
	case policy.Mandatory:
		return true
	case policy.Overridable:
		return !overrides[f.PolicyID]
	}
	return false
}

type compiled struct {
	policy.Policy
	prg cel.Program
}

// Engine holds compiled policies. It is safe for concurrent use.
type Engine struct {
	policies []compiled
}

// NewEnv returns the CEL environment policies are compiled in. Variables:
//
//	before    the resource's attributes before the change (null on create)
//	after     the planned attributes (null on delete)
//	resource  metadata: address, type, name, mode, provider, module, action
//	resources every resource change in the plan, for cross-resource checks
func NewEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("before", cel.DynType),
		cel.Variable("after", cel.DynType),
		cel.Variable("resource", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("resources", cel.ListType(cel.MapType(cel.StringType, cel.DynType))),
		ext.Strings(),
		ext.Sets(),
	)
}

// New compiles every policy up front so type and syntax errors surface before any plan is read.
func New(ps []policy.Policy) (*Engine, error) {
	env, err := NewEnv()
	if err != nil {
		return nil, err
	}
	e := &Engine{}
	var errs []error
	for _, p := range ps {
		ast, iss := env.Compile(p.Expression)
		if iss != nil && iss.Err() != nil {
			errs = append(errs, fmt.Errorf("%s: policy %q: %w", p.Source, p.ID, iss.Err()))
			continue
		}
		if t := ast.OutputType().String(); t != "bool" && t != "dyn" {
			errs = append(errs, fmt.Errorf("%s: policy %q: expression returns %s, want bool", p.Source, p.ID, t))
			continue
		}
		prg, err := env.Program(ast, cel.EvalOptions(cel.OptPartialEval))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: policy %q: %w", p.Source, p.ID, err))
			continue
		}
		e.policies = append(e.policies, compiled{Policy: p, prg: prg})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return e, nil
}

// Evaluate runs every matching policy against every resource change, in plan order.
func (e *Engine) Evaluate(p *tfjson.Plan) []Finding {
	all := resourceList(p)
	var out []Finding
	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		action := ActionKind(rc.Change.Actions)
		for _, cp := range e.policies {
			if matches(cp.Match, rc, action) {
				out = append(out, evaluate(cp, rc, action, all))
			}
		}
	}
	return out
}

func evaluate(cp compiled, rc *tfjson.ResourceChange, action string, all []any) Finding {
	f := Finding{
		PolicyID:    cp.ID,
		Address:     rc.Address,
		Type:        rc.Type,
		Action:      action,
		Severity:    cp.Severity,
		Enforcement: cp.Enforcement,
		OnUnknown:   cp.OnUnknown,
		Message:     cp.Message,
		Source:      cp.Source,
	}

	vars, err := cel.PartialVars(map[string]any{
		"before":    rc.Change.Before,
		"after":     rc.Change.After,
		"resource":  resourceMeta(rc, action),
		"resources": all,
	}, unknownPatterns("after", rc.Change.AfterUnknown)...)
	if err != nil {
		f.Status, f.Error = StatusError, err.Error()
		return f
	}

	val, _, err := cp.prg.Eval(vars)
	switch {
	case err != nil:
		f.Status, f.Error = StatusError, err.Error()
	case types.IsUnknown(val):
		f.Status = StatusUnknown
	default:
		b, ok := val.Value().(bool)
		switch {
		case !ok:
			f.Status, f.Error = StatusError, fmt.Sprintf("expression returned %s, want bool", val.Type().TypeName())
		case b:
			f.Status = StatusPass
		default:
			f.Status = StatusFail
		}
	}
	return f
}

func matches(m policy.Match, rc *tfjson.ResourceChange, action string) bool {
	if string(rc.Mode) != m.Mode || !slices.Contains(m.Actions, action) {
		return false
	}
	for _, t := range m.Types {
		if ok, _ := path.Match(t, rc.Type); ok {
			return true
		}
	}
	return false
}

// ActionKind collapses Terraform's action list into a single word.
func ActionKind(a tfjson.Actions) string {
	switch {
	case a.Replace():
		return "replace"
	case a.Create():
		return "create"
	case a.Update():
		return "update"
	case a.Delete():
		return "delete"
	case a.Read():
		return "read"
	case a.NoOp():
		return "no-op"
	}
	return "unknown"
}

func resourceMeta(rc *tfjson.ResourceChange, action string) map[string]any {
	return map[string]any{
		"address":  rc.Address,
		"type":     rc.Type,
		"name":     rc.Name,
		"mode":     string(rc.Mode),
		"provider": rc.ProviderName,
		"module":   rc.ModuleAddress,
		"action":   action,
	}
}

func resourceList(p *tfjson.Plan) []any {
	out := make([]any, 0, len(p.ResourceChanges))
	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		m := resourceMeta(rc, ActionKind(rc.Change.Actions))
		m["after"] = rc.Change.After
		out = append(out, m)
	}
	return out
}
