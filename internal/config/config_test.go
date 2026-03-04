package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempConfig writes content to a temporary config file and sets
// XDG_CONFIG_HOME so Load() picks it up.  It returns a cleanup function.
func writeTempConfig(t *testing.T, content string) func() {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "pinentry-go")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	return func() {}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty dir — no config file
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Defaults.Color != "#888888" {
		t.Errorf("default color = %q, want #888888", cfg.Defaults.Color)
	}
	if cfg.Defaults.Name != "Unknown key" {
		t.Errorf("default name = %q, want \"Unknown key\"", cfg.Defaults.Name)
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	const toml = `
[defaults]
color = "#333333"
name  = "My default"

[[keys]]
match = "n/AABBCCDD"
name  = "Work SSH"
color = "#0066cc"

[[keys]]
match = "s/"
name  = "Signing"
color = "#007700"
`
	writeTempConfig(t, toml)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Defaults.Color != "#333333" {
		t.Errorf("defaults.color = %q, want #333333", cfg.Defaults.Color)
	}
	if len(cfg.Keys) != 2 {
		t.Fatalf("len(keys) = %d, want 2", len(cfg.Keys))
	}
	if cfg.Keys[0].Match != "n/AABBCCDD" {
		t.Errorf("keys[0].match = %q", cfg.Keys[0].Match)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	writeTempConfig(t, "[[[ not valid toml")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}

func TestFindStyle(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{Color: "#888888", Name: "Unknown"},
		Keys: []KeyRule{
			{Match: "n/AABBCCDD", Name: "Work SSH", Color: "#0066cc"},
			{Match: "s/", Name: "Signing", Color: "#007700"},
			{Match: "n/11223344", Name: "Pass store", Color: "#cc0000"},
		},
	}

	tests := []struct {
		keyID     string
		wantName  string
		wantColor string
	}{
		{"n/AABBCCDD1122", "Work SSH", "#0066cc"},   // exact prefix match
		{"n/AABBCCDDFFEE", "Work SSH", "#0066cc"},   // longer id, same prefix
		{"s/DEADBEEF", "Signing", "#007700"},        // prefix "s/" matches
		{"n/11223344CAFE", "Pass store", "#cc0000"},  // third rule
		{"u/CAFECAFE", "u/CAFECAFE", "#888888"},      // no match → show keygrip
		{"", "Unknown", "#888888"},                   // empty id → defaults
	}

	for _, tt := range tests {
		s := cfg.FindStyle(tt.keyID)
		if s.Name != tt.wantName {
			t.Errorf("FindStyle(%q).Name = %q, want %q", tt.keyID, s.Name, tt.wantName)
		}
		if s.Color != tt.wantColor {
			t.Errorf("FindStyle(%q).Color = %q, want %q", tt.keyID, s.Color, tt.wantColor)
		}
	}
}

func TestFindStyle_FirstMatchWins(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{Color: "#888888", Name: "Unknown"},
		Keys: []KeyRule{
			{Match: "n/", Name: "All encryption", Color: "#0000ff"},
			{Match: "n/AABBCCDD", Name: "Specific key", Color: "#ff0000"},
		},
	}
	// The first rule ("n/") should win even though the second is more specific.
	s := cfg.FindStyle("n/AABBCCDD")
	if s.Name != "All encryption" {
		t.Errorf("expected first rule to win, got name %q", s.Name)
	}
}

func TestLoad_MissingFieldsFallToDefaults(t *testing.T) {
	const toml = `
[defaults]
color = "#111111"
name  = "Default"

[[keys]]
match = "n/AABB"
# no name or color — should inherit from defaults
`
	writeTempConfig(t, toml)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := cfg.FindStyle("n/AABB1234")
	if s.Color != "#111111" {
		t.Errorf("color = %q, want #111111 (from defaults)", s.Color)
	}
	if s.Name != "Default" {
		t.Errorf("name = %q, want \"Default\" (from defaults)", s.Name)
	}
}
