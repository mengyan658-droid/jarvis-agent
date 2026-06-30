package tool

import (
	"context"
	"testing"

	"jarvis-agent/internal/client"
	"jarvis-agent/internal/client/jarvis"
	"jarvis-agent/internal/domain"
)

func TestRegistryRecordsToolCalls(t *testing.T) {
	store := client.NewMockStore()
	registry := NewRegistry(QueryHostsTool{Client: jarvis.NewMockClient(store)})
	recorder := &Recorder{}
	out, err := registry.Execute(context.Background(), QueryHostsToolName, client.HostQuery{Region: "east-china"}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.([]domain.Host)) == 0 {
		t.Fatal("unexpected empty output")
	}
	if len(recorder.Calls()) != 1 || recorder.Calls()[0].Name != QueryHostsToolName {
		t.Fatalf("unexpected calls: %+v", recorder.Calls())
	}
}
