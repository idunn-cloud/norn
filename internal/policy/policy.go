// Package policy defines Norn's policy file format and loads it from disk.
package policy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	APIVersion = "norn.idunn.cloud/v1alpha1"
	Kind       = "PolicySet"
)

// Enforcement decides what a failing policy does to the run.
type Enforcement string

const (
	Advisory    Enforcement = "advisory"    // report only
	Overridable Enforcement = "overridable" // blocks unless explicitly overridden
	Mandatory   Enforcement = "mandatory"   // always blocks
)

var (
	idPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	validSeverities = []string{"low", "medium", "high", "critical"}
	validActions    = []string{"create", "update", "delete", "replace", "read", "no-op"}
	validOnUnknown  = []string{"warn", "fail", "pass"}
	defaultActions  = []string{"create", "update", "replace"}
)

// Match selects which resource changes a policy applies to.
type Match struct {
	Types   []string `yaml:"types"`   // resource types, glob patterns allowed (azurerm_*)
	Actions []string `yaml:"actions"` // defaults to create, update, replace
	Mode    string   `yaml:"mode"`    // managed (default) or data
}

// Policy is a single rule. Expression must evaluate to true when the change is compliant.
type Policy struct {
	ID          string      `yaml:"id"`
	Description string      `yaml:"description"`
	Severity    string      `yaml:"severity"`
	Enforcement Enforcement `yaml:"enforcement"`
	OnUnknown   string      `yaml:"onUnknown"` // what to do when the result depends on values known only after apply
	Match       Match       `yaml:"match"`
	Expression  string      `yaml:"expression"`
	Message     string      `yaml:"message"`

	Source string `yaml:"-"` // file the policy was loaded from
}

// PolicySet is the top-level document in a policy file.
type PolicySet struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Policies []Policy `yaml:"policies"`
}

// Load reads a single policy file or every .yaml/.yml file under a directory.
// Unknown fields are rejected so that a typo can never silently weaken a policy.
func Load(root string) ([]Policy, error) {
	files, err := policyFiles(root)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no policy files found in %s", root)
	}

	var (
		out  []Policy
		errs []error
		seen = map[string]string{}
	)
	for _, file := range files {
		sets, err := decodeFile(file)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, set := range sets {
			if set.APIVersion != APIVersion || set.Kind != Kind {
				errs = append(errs, fmt.Errorf("%s: expected apiVersion %q and kind %q", file, APIVersion, Kind))
				continue
			}
			for _, p := range set.Policies {
				p.Source = file
				if err := normalize(&p); err != nil {
					errs = append(errs, fmt.Errorf("%s: policy %q: %w", file, p.ID, err))
					continue
				}
				if prev, dup := seen[p.ID]; dup {
					errs = append(errs, fmt.Errorf("%s: duplicate policy id %q (first defined in %s)", file, p.ID, prev))
					continue
				}
				seen[p.ID] = file
				out = append(out, p)
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

func policyFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("policies: %w", err)
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

func decodeFile(file string) ([]PolicySet, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var sets []PolicySet
	for {
		var set PolicySet
		err := dec.Decode(&set)
		if errors.Is(err, io.EOF) {
			return sets, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		sets = append(sets, set)
	}
}

func normalize(p *Policy) error {
	switch {
	case !idPattern.MatchString(p.ID):
		return errors.New("id must be lowercase letters, digits and dashes")
	case p.Expression == "":
		return errors.New("expression is required")
	case p.Message == "":
		return errors.New("message is required")
	case !slices.Contains(validSeverities, p.Severity):
		return fmt.Errorf("severity must be one of %v", validSeverities)
	}
	switch p.Enforcement {
	case Advisory, Overridable, Mandatory:
	default:
		return errors.New("enforcement must be advisory, overridable or mandatory")
	}

	if p.OnUnknown == "" {
		p.OnUnknown = "warn"
	}
	if !slices.Contains(validOnUnknown, p.OnUnknown) {
		return fmt.Errorf("onUnknown must be one of %v", validOnUnknown)
	}

	if len(p.Match.Types) == 0 {
		return errors.New("match.types must list at least one resource type")
	}
	for _, t := range p.Match.Types {
		if _, err := path.Match(t, ""); err != nil {
			return fmt.Errorf("match.types: bad pattern %q", t)
		}
	}
	if len(p.Match.Actions) == 0 {
		p.Match.Actions = slices.Clone(defaultActions)
	}
	for _, a := range p.Match.Actions {
		if !slices.Contains(validActions, a) {
			return fmt.Errorf("match.actions: %q is not one of %v", a, validActions)
		}
	}
	if p.Match.Mode == "" {
		p.Match.Mode = "managed"
	}
	if p.Match.Mode != "managed" && p.Match.Mode != "data" {
		return errors.New("match.mode must be managed or data")
	}
	return nil
}
