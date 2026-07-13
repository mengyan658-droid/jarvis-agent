package skill

type Spec struct {
	Name         string
	Version      string
	Description  string
	Intents      []string
	Workflow     string
	Tools        []string
	ReadOnly     bool
	OutputPolicy string
	Body         string
	Path         string
}

type Summary struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Intents     []string `json:"intents"`
	Workflow    string   `json:"workflow"`
	Tools       []string `json:"tools"`
	ReadOnly    bool     `json:"read_only"`
}

func (s Spec) Summary() Summary {
	return Summary{
		Name:        s.Name,
		Description: s.Description,
		Intents:     append([]string(nil), s.Intents...),
		Workflow:    s.Workflow,
		Tools:       append([]string(nil), s.Tools...),
		ReadOnly:    s.ReadOnly,
	}
}

type Selection struct {
	Skill      string
	Parameters map[string]string
	Confidence float64
}
