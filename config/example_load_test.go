package config

import "testing"

// TestExampleConfig_LoadsOpenAICompatInstances guards that the shipped
// config.example.yaml uses the map shape for openai-compat (Task 1+2 migration):
// a reserved `default` instance plus named LAN instances box1/box2. Before the
// example is migrated from the old single-struct block this fails (box1/box2
// keys absent, or a struct→map parse error).
func TestExampleConfig_LoadsOpenAICompatInstances(t *testing.T) {
	cfg, err := Load("../config.example.yaml")
	if err != nil {
		t.Fatalf("Load(config.example.yaml): unexpected error: %v", err)
	}

	want := map[string]string{
		"default": "http://localhost:8000/v1",
		"box1":    "http://192.168.1.10:1234/v1",
		"box2":    "http://192.168.1.11:1234/v1",
	}
	for name, endpoint := range want {
		inst, ok := cfg.LLM.OpenAICompat[name]
		if !ok {
			t.Errorf("OpenAICompat missing instance %q", name)
			continue
		}
		if inst.Endpoint != endpoint {
			t.Errorf("OpenAICompat[%q].Endpoint: got %q, want %q", name, inst.Endpoint, endpoint)
		}
	}
}
