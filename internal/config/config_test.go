package config

import "testing"

// History is a clamp, not a validation error: a config that switches it on must
// stay loadable after the backend changes to local.
func TestHistoryEnabledClampedForLocal(t *testing.T) {
	cases := []struct {
		provider string
		history  bool
		want     bool
	}{
		{"mistral", true, true},
		{"mistral", false, false},
		{"local", true, false},
		{"local", false, false},
	}

	for _, tc := range cases {
		cfg := Config{Provider: tc.provider, History: tc.history}
		if got := cfg.HistoryEnabled(); got != tc.want {
			t.Errorf("Config{%s, history=%v}.HistoryEnabled() = %v, want %v",
				tc.provider, tc.history, got, tc.want)
		}
	}
}

func TestLoadHistoryEnvOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AI_SH_HISTORY", "on")
	t.Setenv("AI_SH_HISTORY_TURNS", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.History {
		t.Error("AI_SH_HISTORY=on did not enable history")
	}
	if cfg.HistoryTurns != 3 {
		t.Errorf("HistoryTurns = %d, want 3", cfg.HistoryTurns)
	}
}

// An unparseable value leaves the configured setting alone rather than
// silently flipping it off.
func TestLoadIgnoresUnparseableHistoryEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AI_SH_HISTORY", "maybe")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.History {
		t.Error("history should stay at its default")
	}
}

func TestParseBool(t *testing.T) {
	for _, v := range []string{"1", "true", "YES", " on "} {
		if on, ok := parseBool(v); !ok || !on {
			t.Errorf("parseBool(%q) = (%v, %v), want (true, true)", v, on, ok)
		}
	}
	for _, v := range []string{"0", "false", "NO", "off"} {
		if on, ok := parseBool(v); !ok || on {
			t.Errorf("parseBool(%q) = (%v, %v), want (false, true)", v, on, ok)
		}
	}
	if _, ok := parseBool("banana"); ok {
		t.Error("parseBool accepted a non-boolean")
	}
}
