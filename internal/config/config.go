// Package config loads and queries the pinentry-go configuration file.
//
// The config file lives at $XDG_CONFIG_HOME/pinentry-go/config.toml
// (defaulting to ~/.config/pinentry-go/config.toml) and is optional.
// If it is absent or unreadable the package returns built-in defaults.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Defaults holds the fallback style used when no key rule matches.
type Defaults struct {
	// Color is a CSS color string for the dialog accent (e.g. "#888888").
	Color string `toml:"color"`
	// Name is a human-readable label shown in the dialog header.
	Name string `toml:"name"`
}

// KeyRule maps a key identifier substring to a display style.
type KeyRule struct {
	// Match is a substring matched against the SETKEYINFO value sent by
	// gpg-agent.  The value has the form "<status>/<hexkeygrip>" where
	// <status> is a cache-state letter (n=not cached, s=session cache,
	// t=TTL expired, u=in use).  Match on the hex keygrip.
	//
	// Find keygrips with:
	//   gpg --list-keys --with-keygrip          (GPG keys)
	//   gpg-connect-agent "keyinfo --list" /bye  (all keys including SSH)
	Match string `toml:"match"`
	// Name is a human-readable label shown in the dialog header.
	Name string `toml:"name"`
	// Color is a CSS color string for the dialog accent.
	Color string `toml:"color"`
}

// Config is the top-level configuration structure.
type Config struct {
	Defaults Defaults  `toml:"defaults"`
	Keys     []KeyRule `toml:"keys"`
}

// Style describes how a dialog should be presented for a particular key.
type Style struct {
	// Name is the human-readable key label shown in the header bar.
	Name string
	// Color is the CSS color string for the header bar accent.
	Color string
}

// defaultConfig is returned when the config file is absent.
var defaultConfig = Config{
	Defaults: Defaults{
		Color: "#888888",
		Name:  "Unknown key",
	},
}

// Load reads the config file and returns the parsed Config.
// If the file does not exist, it returns a Config with built-in defaults and
// no error.  Any other I/O or parse error is returned.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return &defaultConfig, nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &defaultConfig, nil
	}
	if err != nil {
		return nil, err
	}

	cfg := defaultConfig // copy defaults so missing fields keep their values
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, err
	}

	// Fill in default values for any partially-specified rules.
	for i := range cfg.Keys {
		if cfg.Keys[i].Color == "" {
			cfg.Keys[i].Color = cfg.Defaults.Color
		}
		if cfg.Keys[i].Name == "" {
			cfg.Keys[i].Name = cfg.Defaults.Name
		}
	}

	return &cfg, nil
}

// FindStyle returns the Style for the given key identifier (the raw value
// received from SETKEYINFO).  Rules are tested in order; the first whose
// Match string is a substring of keyID wins.  If no rule matches, the
// defaults are returned.
func (c *Config) FindStyle(keyID string) Style {
	for _, rule := range c.Keys {
		if strings.Contains(keyID, rule.Match) {
			return Style{Name: rule.Name, Color: rule.Color}
		}
	}
	name := c.Defaults.Name
	if keyID != "" {
		name = keyID
	}
	return Style{Name: name, Color: c.Defaults.Color}
}

// configPath returns the path to the config file, honouring XDG_CONFIG_HOME.
func configPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "pinentry-go", "config.toml"), nil
}
