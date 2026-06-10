package manager

import (
	"reflect"
	"testing"
)

func TestApplyDefaultsEmptyConfigYieldsDefaultKeys(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{name: "nil available_keys", keys: nil},
		{name: "empty available_keys", keys: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := TmuxConfig{AvailableKeys: tt.keys}
			cfg.ApplyDefaults()

			if len(cfg.AvailableKeys) == 0 {
				t.Fatal("expected non-empty available_keys after ApplyDefaults")
			}
			if !reflect.DeepEqual(cfg.AvailableKeys, DefaultAvailableKeys()) {
				t.Errorf("ApplyDefaults() = %v, want %v", cfg.AvailableKeys, DefaultAvailableKeys())
			}
		})
	}
}

func TestApplyDefaultsConfiguredKeysWin(t *testing.T) {
	configured := []string{"a", "b", "c"}
	cfg := TmuxConfig{AvailableKeys: []string{"a", "b", "c"}}
	cfg.ApplyDefaults()

	if !reflect.DeepEqual(cfg.AvailableKeys, configured) {
		t.Errorf("ApplyDefaults() overwrote configured keys: got %v, want %v", cfg.AvailableKeys, configured)
	}
}

func TestDefaultAvailableKeysExcludesReservedUIKeys(t *testing.T) {
	keys := DefaultAvailableKeys()
	if len(keys) == 0 {
		t.Fatal("default key set must not be empty")
	}
	// 'q' is bound by nav's generated tmux key table as an escape hatch
	// (core/pkg/tmux/keygen.GenerateEscapeHatches) and must not be handed
	// out as a project shortcut by default.
	for _, k := range keys {
		if k == "q" {
			t.Error("default key set must not contain reserved key 'q'")
		}
	}
}

func TestManagerResolvesDefaultKeysFromEmptyConfig(t *testing.T) {
	// Simulates a manager built from a config that has no keys configured:
	// after ApplyDefaults (called in NewManager), GetAvailableKeys must
	// return the built-in default set.
	cfg := TmuxConfig{}
	cfg.ApplyDefaults()
	m := &Manager{tmuxConfig: &cfg}

	got := m.GetAvailableKeys()
	if len(got) == 0 {
		t.Fatal("manager resolved an empty key set from empty config")
	}
	if !reflect.DeepEqual(got, DefaultAvailableKeys()) {
		t.Errorf("GetAvailableKeys() = %v, want default %v", got, DefaultAvailableKeys())
	}
}
