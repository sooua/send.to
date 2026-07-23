package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config is the on-disk client configuration: a set of named servers plus
// which one to use by default.
type Config struct {
	Default  string             `json:"default"`
	Profiles map[string]Profile `json:"profiles"`
}

// Profile is one server the client knows about.
type Profile struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// ConfigDir returns the directory holding config.json and history.json.
// SENDTO_CONFIG_DIR overrides it, which keeps tests off the real profile.
func ConfigDir() (string, error) {
	if dir := os.Getenv("SENDTO_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not locate a config directory: %w", err)
	}

	return filepath.Join(base, "sendto"), nil
}

func configPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadConfig reads the configuration, returning an empty one when no file
// exists yet.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's own config dir
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Profiles: map[string]Profile{}}, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}

	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}

	return &cfg, nil
}

// Save writes the configuration, creating the directory if needed. The file
// may hold a server password, so it is written 0600.
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0600)
}

// ProfileNames returns the configured profile names in a stable order.
func (c *Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Resolve picks the server to talk to. Precedence, highest first:
//
//	explicit --url flag
//	explicit --profile flag
//	SENDTO_URL environment variable
//	the configured default profile
//
// SENDTO_USER and SENDTO_PASS override credentials at any level, so a CI job
// can authenticate without writing a config file.
func (c *Config) Resolve(urlFlag, profileFlag string) (Profile, error) {
	var p Profile

	switch {
	case urlFlag != "":
		p = Profile{URL: urlFlag}

	case profileFlag != "":
		found, ok := c.Profiles[profileFlag]
		if !ok {
			return p, fmt.Errorf("unknown profile %q (configured: %s)", profileFlag, strings.Join(c.ProfileNames(), ", "))
		}
		p = found

	case os.Getenv("SENDTO_URL") != "":
		p = Profile{URL: os.Getenv("SENDTO_URL")}

	case c.Default != "":
		found, ok := c.Profiles[c.Default]
		if !ok {
			return p, fmt.Errorf("default profile %q is not configured", c.Default)
		}
		p = found

	default:
		return p, errors.New("no server configured — run `send config add <name> <url>`, set SENDTO_URL, or pass --url")
	}

	if v := os.Getenv("SENDTO_USER"); v != "" {
		p.Username = v
	}
	if v := os.Getenv("SENDTO_PASS"); v != "" {
		p.Password = v
	}

	if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
		p.URL = "https://" + p.URL
	}

	return p, nil
}
