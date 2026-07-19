package skill

import (
	"fmt"
	"sort"
)

type Registry struct {
	byName   map[string]Spec
	byIntent map[string]string
}

func NewRegistry(specs ...Spec) (*Registry, error) {
	r := &Registry{
		byName:   map[string]Spec{},
		byIntent: map[string]string{},
	}
	for _, spec := range specs {
		if err := r.Register(spec); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(spec Spec) error {
	if spec.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	spec.Executor = spec.ExecutorOrDefault()
	if !isSupportedExecutor(spec.Executor) {
		return fmt.Errorf("skill %q unsupported executor %q", spec.Name, spec.Executor)
	}
	if spec.Executor == ExecutorWorkflow && spec.Workflow == "" {
		return fmt.Errorf("skill %q workflow is required", spec.Name)
	}
	if _, exists := r.byName[spec.Name]; exists {
		return fmt.Errorf("duplicate skill %q", spec.Name)
	}
	r.byName[spec.Name] = spec
	for _, intent := range spec.Intents {
		if intent == "" {
			continue
		}
		if existing, exists := r.byIntent[intent]; exists {
			return fmt.Errorf("intent %q is already mapped to skill %q", intent, existing)
		}
		r.byIntent[intent] = spec.Name
	}
	return nil
}

func isSupportedExecutor(executor string) bool {
	switch executor {
	case ExecutorWorkflow, ExecutorToolLoop, ExecutorGuidedSteps, ExecutorSubAgent:
		return true
	default:
		return false
	}
}

func (r *Registry) Get(name string) (Spec, bool) {
	if r == nil {
		return Spec{}, false
	}
	spec, ok := r.byName[name]
	return spec, ok
}

func (r *Registry) GetByIntent(intent string) (Spec, bool) {
	if r == nil {
		return Spec{}, false
	}
	name, ok := r.byIntent[intent]
	if !ok {
		return Spec{}, false
	}
	return r.Get(name)
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.byName))
	for name := range r.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Summaries() []Summary {
	names := r.Names()
	out := make([]Summary, 0, len(names))
	for _, name := range names {
		out = append(out, r.byName[name].Summary())
	}
	return out
}

func (r *Registry) ValidateTools(hasTool func(string) bool) error {
	if r == nil || hasTool == nil {
		return nil
	}
	for _, name := range r.Names() {
		spec := r.byName[name]
		for _, toolName := range spec.Tools {
			if !hasTool(toolName) {
				return fmt.Errorf("skill %q requires missing tool %q", spec.Name, toolName)
			}
		}
	}
	return nil
}

func (r *Registry) ValidateExecutionTargets(hasWorkflow func(string) bool) error {
	if r == nil || hasWorkflow == nil {
		return nil
	}
	for _, name := range r.Names() {
		spec := r.byName[name]
		switch spec.ExecutorOrDefault() {
		case ExecutorWorkflow:
			if spec.Workflow == "" {
				return fmt.Errorf("skill %q workflow is required", spec.Name)
			}
			if !hasWorkflow(spec.Workflow) {
				return fmt.Errorf("skill %q requires missing workflow %q", spec.Name, spec.Workflow)
			}
		case ExecutorToolLoop, ExecutorGuidedSteps:
			if spec.Workflow != "" && !hasWorkflow(spec.Workflow) {
				return fmt.Errorf("skill %q requires missing workflow %q", spec.Name, spec.Workflow)
			}
		}
	}
	return nil
}
