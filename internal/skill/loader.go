package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func LoadDir(root string) (*Registry, error) {
	matches, err := filepath.Glob(filepath.Join(root, "*", "SKILL.md"))
	if err != nil {
		return nil, err
	}
	specs := make([]Spec, 0, len(matches))
	for _, path := range matches {
		spec, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return NewRegistry(specs...)
}

func LoadFile(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	fields, body, err := parseFrontMatter(string(data))
	if err != nil {
		return Spec{}, fmt.Errorf("%s: %w", path, err)
	}
	spec := Spec{
		Name:         fields.stringValue("name"),
		Version:      fields.stringValue("version"),
		Description:  fields.stringValue("description"),
		Executor:     fields.stringValue("executor"),
		Intents:      fields.stringList("intents"),
		Triggers:     fields.stringList("triggers"),
		Workflow:     fields.stringValue("workflow"),
		Tools:        fields.stringList("tools"),
		Parameters:   fields.parameterSpecs("parameters"),
		ReadOnly:     fields.boolValue("read_only"),
		OutputPolicy: fields.stringValue("output_policy"),
		OutputSchema: fields.stringMap("output_schema"),
		Guardrails:   fields.stringList("guardrails"),
		Body:         strings.TrimSpace(body),
		Path:         path,
	}
	if spec.Executor == "" {
		spec.Executor = ExecutorWorkflow
	}
	if spec.Name == "" {
		return Spec{}, fmt.Errorf("name is required")
	}
	if spec.Executor == ExecutorWorkflow && spec.Workflow == "" {
		return Spec{}, fmt.Errorf("workflow is required")
	}
	return spec, nil
}

type frontMatter struct {
	values      map[string][]string
	maps        map[string]map[string]string
	objectLists map[string][]map[string]string
}

func parseFrontMatter(content string) (frontMatter, string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return frontMatter{}, "", fmt.Errorf("front matter is required")
	}
	fields := frontMatter{
		values:      map[string][]string{},
		maps:        map[string]map[string]string{},
		objectLists: map[string][]map[string]string{},
	}
	currentKey := ""
	currentObjectListKey := ""
	currentObjectIndex := -1
	end := -1
	for i := 1; i < len(lines); i++ {
		rawLine := strings.TrimRight(lines[i], "\r")
		line := strings.TrimSpace(rawLine)
		if line == "---" {
			end = i
			break
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := leadingSpaces(rawLine)
		if indent == 0 {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				return frontMatter{}, "", fmt.Errorf("invalid front matter line %q", line)
			}
			currentKey = strings.TrimSpace(key)
			currentObjectListKey = ""
			currentObjectIndex = -1
			value = cleanFrontMatterValue(strings.TrimSpace(value))
			if value == "" {
				if _, ok := fields.values[currentKey]; !ok {
					fields.values[currentKey] = []string{}
				}
				continue
			}
			fields.values[currentKey] = []string{value}
			continue
		}
		if currentKey == "" {
			return frontMatter{}, "", fmt.Errorf("nested value without key")
		}
		if strings.HasPrefix(line, "- ") {
			item := cleanFrontMatterValue(strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			if key, value, ok := strings.Cut(item, ":"); ok {
				object := map[string]string{strings.TrimSpace(key): cleanFrontMatterValue(strings.TrimSpace(value))}
				fields.objectLists[currentKey] = append(fields.objectLists[currentKey], object)
				currentObjectListKey = currentKey
				currentObjectIndex = len(fields.objectLists[currentKey]) - 1
				continue
			}
			fields.values[currentKey] = append(fields.values[currentKey], item)
			currentObjectListKey = ""
			currentObjectIndex = -1
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return frontMatter{}, "", fmt.Errorf("invalid front matter line %q", line)
		}
		nestedKey := strings.TrimSpace(key)
		nestedValue := cleanFrontMatterValue(strings.TrimSpace(value))
		if currentObjectListKey != "" && currentObjectIndex >= 0 {
			fields.objectLists[currentObjectListKey][currentObjectIndex][nestedKey] = nestedValue
			continue
		}
		if _, ok := fields.maps[currentKey]; !ok {
			fields.maps[currentKey] = map[string]string{}
		}
		fields.maps[currentKey][nestedKey] = nestedValue
	}
	if end < 0 {
		return frontMatter{}, "", fmt.Errorf("front matter end marker is required")
	}
	return fields, strings.Join(lines[end+1:], "\n"), nil
}

func leadingSpaces(value string) int {
	count := 0
	for _, r := range value {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}

func cleanFrontMatterValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return value
}

func (f frontMatter) stringValue(key string) string {
	values := f.values[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (f frontMatter) stringList(key string) []string {
	values := f.values[key]
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (f frontMatter) stringMap(key string) map[string]string {
	values := f.maps[key]
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, value := range values {
		k = strings.TrimSpace(k)
		if k != "" {
			out[k] = strings.TrimSpace(value)
		}
	}
	return out
}

func (f frontMatter) parameterSpecs(key string) []ParameterSpec {
	values := f.objectLists[key]
	if len(values) == 0 {
		return nil
	}
	out := make([]ParameterSpec, 0, len(values))
	for _, raw := range values {
		spec := ParameterSpec{
			Name:        strings.TrimSpace(raw["name"]),
			Type:        strings.TrimSpace(raw["type"]),
			Pattern:     strings.TrimSpace(raw["pattern"]),
			Description: strings.TrimSpace(raw["description"]),
		}
		if spec.Type == "" {
			spec.Type = "string"
		}
		spec.Required = parseBool(raw["required"])
		spec.Enum = splitCSV(raw["enum"])
		if spec.Name != "" {
			out = append(out, spec)
		}
	}
	return out
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(value)))
	if err == nil {
		return parsed
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "y", "1":
		return true
	default:
		return false
	}
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = cleanFrontMatterValue(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (f frontMatter) boolValue(key string) bool {
	switch strings.ToLower(f.stringValue(key)) {
	case "true", "yes", "y", "1":
		return true
	default:
		return false
	}
}
