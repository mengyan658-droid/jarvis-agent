package skill

import (
	"fmt"
	"os"
	"path/filepath"
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
		Intents:      fields.stringList("intents"),
		Workflow:     fields.stringValue("workflow"),
		Tools:        fields.stringList("tools"),
		ReadOnly:     fields.boolValue("read_only"),
		OutputPolicy: fields.stringValue("output_policy"),
		Body:         strings.TrimSpace(body),
		Path:         path,
	}
	if spec.Name == "" {
		return Spec{}, fmt.Errorf("name is required")
	}
	if spec.Workflow == "" {
		return Spec{}, fmt.Errorf("workflow is required")
	}
	return spec, nil
}

type frontMatter map[string][]string

func parseFrontMatter(content string) (frontMatter, string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", fmt.Errorf("front matter is required")
	}
	fields := frontMatter{}
	currentKey := ""
	end := -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			end = i
			break
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if currentKey == "" {
				return nil, "", fmt.Errorf("list item without key")
			}
			fields[currentKey] = append(fields[currentKey], cleanFrontMatterValue(strings.TrimSpace(strings.TrimPrefix(line, "- "))))
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("invalid front matter line %q", line)
		}
		currentKey = strings.TrimSpace(key)
		value = cleanFrontMatterValue(strings.TrimSpace(value))
		if value == "" {
			if _, ok := fields[currentKey]; !ok {
				fields[currentKey] = []string{}
			}
			continue
		}
		fields[currentKey] = []string{value}
	}
	if end < 0 {
		return nil, "", fmt.Errorf("front matter end marker is required")
	}
	return fields, strings.Join(lines[end+1:], "\n"), nil
}

func cleanFrontMatterValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return value
}

func (f frontMatter) stringValue(key string) string {
	values := f[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (f frontMatter) stringList(key string) []string {
	values := f[key]
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
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
