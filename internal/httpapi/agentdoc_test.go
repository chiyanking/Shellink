package httpapi

import "testing"

func TestAgentDocIncludesCLIContract(t *testing.T) {
	if AgentDoc == "" {
		t.Fatal("embedded agent documentation is empty")
	}
	commands, ok := AgentDocJSON["commands"].(map[string][]string)
	if !ok {
		t.Fatalf("commands have type %T", AgentDocJSON["commands"])
	}
	for _, command := range []string{"exec-status", "exec-cancel", "upload", "download", "edit"} {
		found := false
		for _, candidate := range commands["session"] {
			if candidate == command {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("session command %q is missing from agent reference", command)
		}
	}
	longRunning, ok := AgentDocJSON["longRunning"].(map[string]any)
	if !ok || longRunning["preferDetachAboveMs"] != 25_000 {
		t.Fatalf("long-running operation contract is incomplete: %#v", AgentDocJSON["longRunning"])
	}
}
