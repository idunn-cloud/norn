package engine

import (
	"slices"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/interpreter"
)

// unknownPatterns turns Terraform's after_unknown tree into CEL attribute patterns.
//
// Terraform marks every value that will only be known after apply with `true`
// in after_unknown. Declaring those paths unknown lets CEL's partial evaluation
// decide what it can: `false && <unknown>` is still false, while a result that
// genuinely depends on an unknown value comes back as unknown instead of as a
// wrong pass or a confusing "no such key" error.
func unknownPatterns(root string, afterUnknown any) []*interpreter.AttributePattern {
	var out []*interpreter.AttributePattern
	var walk func(path []any, v any)
	walk = func(path []any, v any) {
		switch t := v.(type) {
		case bool:
			if t {
				out = append(out, buildPattern(root, path))
			}
		case map[string]any:
			for k, child := range t {
				walk(append(slices.Clone(path), k), child)
			}
		case []any:
			for i, child := range t {
				walk(append(slices.Clone(path), int64(i)), child)
			}
		}
	}
	walk(nil, afterUnknown)
	return out
}

func buildPattern(root string, path []any) *interpreter.AttributePattern {
	p := cel.AttributePattern(root)
	for _, q := range path {
		switch q := q.(type) {
		case string:
			p = p.QualString(q)
		case int64:
			p = p.QualInt(q)
		}
	}
	return p
}
