package skill

import "testing"

func TestLoadDir(t *testing.T) {
	registry, err := LoadDir("../../skills")
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Names()) != 3 {
		t.Fatalf("unexpected skills: %+v", registry.Names())
	}
	spec, ok := registry.Get("query_faulty_hosts")
	if !ok {
		t.Fatal("query_faulty_hosts skill not found")
	}
	if spec.Workflow != "query_faulty_hosts" || !spec.ReadOnly {
		t.Fatalf("unexpected skill spec: %+v", spec)
	}
}

func TestSelectSkillFunctionTool(t *testing.T) {
	registry, err := LoadDir("../../skills")
	if err != nil {
		t.Fatal(err)
	}
	tool := SelectSkillFunctionTool(registry)
	if tool.Function.Name != SelectSkillFunctionName {
		t.Fatalf("unexpected function name: %s", tool.Function.Name)
	}
	properties := tool.Function.Parameters["properties"].(map[string]any)
	skillField := properties["skill"].(map[string]any)
	enum := skillField["enum"].([]string)
	if len(enum) != 3 {
		t.Fatalf("unexpected skill enum: %+v", enum)
	}
}

func TestDecodeSelection(t *testing.T) {
	selection, err := DecodeSelection(`{"skill":"diagnose_host","parameters":{"host_id":"host-001","amount":5},"confidence":0.8}`)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Skill != "diagnose_host" || selection.Parameters["host_id"] != "host-001" || selection.Parameters["amount"] != "5" {
		t.Fatalf("unexpected selection: %+v", selection)
	}
}
