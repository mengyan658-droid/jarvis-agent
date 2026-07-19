package skill

import "testing"

func TestLoadDir(t *testing.T) {
	registry, err := LoadDir("../../skills")
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Names()) != 4 {
		t.Fatalf("unexpected skills: %+v", registry.Names())
	}
	spec, ok := registry.Get("query_faulty_hosts")
	if !ok {
		t.Fatal("query_faulty_hosts skill not found")
	}
	if spec.Workflow != "query_faulty_hosts" || !spec.ReadOnly {
		t.Fatalf("unexpected skill spec: %+v", spec)
	}
	if spec.Executor != ExecutorWorkflow {
		t.Fatalf("unexpected executor: %s", spec.Executor)
	}
	if len(spec.Triggers) == 0 {
		t.Fatalf("expected triggers: %+v", spec)
	}
	if len(spec.Parameters) != 5 {
		t.Fatalf("unexpected parameters: %+v", spec.Parameters)
	}
	if spec.Parameters[0].Name != "region" || len(spec.Parameters[0].Enum) != 3 {
		t.Fatalf("unexpected first parameter: %+v", spec.Parameters[0])
	}
	if spec.OutputSchema["summary"] != "string" {
		t.Fatalf("unexpected output schema: %+v", spec.OutputSchema)
	}
	if len(spec.Guardrails) == 0 {
		t.Fatalf("expected guardrails: %+v", spec)
	}
	loopSpec, ok := registry.Get("tool_loop_investigate_host")
	if !ok {
		t.Fatal("tool_loop_investigate_host skill not found")
	}
	if loopSpec.Executor != ExecutorToolLoop {
		t.Fatalf("unexpected loop executor: %s", loopSpec.Executor)
	}
	reportSpec, ok := registry.Get("model_error_daily_report")
	if !ok {
		t.Fatal("model_error_daily_report skill not found")
	}
	if reportSpec.Workflow != "model_error_daily_report" || reportSpec.Executor != ExecutorGuidedSteps {
		t.Fatalf("unexpected report skill: %+v", reportSpec)
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
	if len(enum) != 4 {
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

func TestRegistryAllowsNonWorkflowExecutor(t *testing.T) {
	registry, err := NewRegistry(Spec{
		Name:        "example_sub_agent",
		Description: "example",
		Executor:    ExecutorSubAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := registry.Get("example_sub_agent")
	if !ok {
		t.Fatal("skill not registered")
	}
	if spec.ExecutorOrDefault() != ExecutorSubAgent {
		t.Fatalf("unexpected executor: %s", spec.ExecutorOrDefault())
	}
}
