package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wrinfotel/pastlog/internal/discovery"
)

// Config is the on-disk settings file: the only state the app is allowed to
// write, living in the OS app-config dir — never in agent storage (spec §2.1).
type Config struct {
	Home  string `json:"home,omitempty"`  // "" = auto-discover
	Theme string `json:"theme,omitempty"` // "" = system
}

// configName is the settings file inside the config dir.
const configName = "config.json"

// defaultConfigDir resolves the OS app-config dir; the dir itself is created
// lazily on the first save.
func defaultConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve the app config directory: %v", err)
	}
	return filepath.Join(base, "pastlog-desktop"), nil
}

// loadConfig reads the settings file; a missing, unreadable or corrupt file
// falls back to zero config — the app must work on first launch (spec §2.4)
// and a broken file must never become a fatal error.
func loadConfig(dir string) Config {
	if dir == "" {
		var err error
		if dir, err = defaultConfigDir(); err != nil {
			return Config{}
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, configName))
	if err != nil {
		return Config{}
	}
	var cfg Config
	if json.Unmarshal(raw, &cfg) != nil {
		return Config{}
	}
	return cfg
}

// save persists the config atomically: write next to the target, then rename.
// An empty dir defers to the default location, creating it on demand.
func (c Config) save(dir string) error {
	if dir == "" {
		var err error
		if dir, err = defaultConfigDir(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create the config directory %s: %v", dir, err)
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, configName+".tmp")
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("cannot write the config file: %v", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, configName)); err != nil {
		return fmt.Errorf("cannot store the config file: %v", err)
	}
	return nil
}

// effectiveHomeFor validates a candidate home override exactly like the CLI
// would (discovery.Home), without touching the stored config.
func (a *App) effectiveHomeFor(override string) (string, error) {
	return discovery.Home(override)
}

// effectiveHome resolves the home the services scan: the stored override
// when set, else auto-discovery — the zero-config default.
func (a *App) effectiveHome() (string, error) {
	a.mu.Lock()
	override := a.cfg.Home
	a.mu.Unlock()
	return discovery.Home(override)
}

// errInvalidTheme builds the theme validation error.
func errInvalidTheme(theme string) error {
	return fmt.Errorf("invalid theme %q: use system, dark or light", theme)
}
