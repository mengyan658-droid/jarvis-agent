package skill

const (
	ExecutorWorkflow    = "workflow"
	ExecutorToolLoop    = "tool_loop"
	ExecutorGuidedSteps = "guided_steps"
	ExecutorSubAgent    = "sub_agent"
)

type Spec struct {
	Name         string
	Version      string
	Description  string
	Executor     string
	Intents      []string
	Triggers     []string
	Workflow     string
	Tools        []string
	Parameters   []ParameterSpec
	ReadOnly     bool
	OutputPolicy string
	OutputSchema map[string]string
	Guardrails   []string
	Body         string
	Path         string
}

type ParameterSpec struct {
	Name        string
	Type        string
	Required    bool
	Enum        []string
	Pattern     string
	Description string
}

type Summary struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Executor     string   `json:"executor"`
	Intents      []string `json:"intents"`
	Triggers     []string `json:"triggers"`
	Workflow     string   `json:"workflow,omitempty"`
	Tools        []string `json:"tools"`
	ReadOnly     bool     `json:"read_only"`
	OutputPolicy string   `json:"output_policy,omitempty"`
}

func (s Spec) Summary() Summary {
	return Summary{
		Name:         s.Name,
		Description:  s.Description,
		Executor:     s.ExecutorOrDefault(),
		Intents:      append([]string(nil), s.Intents...),
		Triggers:     append([]string(nil), s.Triggers...),
		Workflow:     s.Workflow,
		Tools:        append([]string(nil), s.Tools...),
		ReadOnly:     s.ReadOnly,
		OutputPolicy: s.OutputPolicy,
	}
}

func (s Spec) ExecutorOrDefault() string {
	if s.Executor != "" {
		return s.Executor
	}
	return ExecutorWorkflow
}

type Selection struct {
	Skill      string
	Parameters map[string]string
	Confidence float64
}
