// Package client is a small HTTP client for the locci API, used by the CLI.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is saved by `locci login` in the user's config dir.
type Config struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	Env   string `json:"env,omitempty"` // environment chosen with `locci env use`; empty = server default
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "locci", "config.json"), nil
}

// LoadSavedConfig reads only the config file, without environment overrides.
// Use it before SaveConfig so overrides aren't written to disk.
func LoadSavedConfig() Config {
	var c Config
	if p, err := configPath(); err == nil {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	}
	return c
}

// LoadConfig reads the saved config. LOCCI_URL, LOCCI_TOKEN and LOCCI_ENV override it.
func LoadConfig() Config {
	c := LoadSavedConfig()
	if v := os.Getenv("LOCCI_URL"); v != "" {
		c.URL = v
	}
	if v := os.Getenv("LOCCI_TOKEN"); v != "" {
		c.Token = v
	}
	if v := os.Getenv("LOCCI_ENV"); v != "" {
		c.Env = v
	}
	if c.URL == "" {
		c.URL = "http://localhost:8998"
	}
	return c
}

func SaveConfig(c Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

type Client struct {
	URL   string
	Token string
	Env   string // when set, Docker resource paths go to /environments/<Env>/...
	HTTP  *http.Client
}

func New(c Config) *Client {
	return &Client{URL: strings.TrimRight(c.URL, "/"), Token: c.Token, Env: c.Env, HTTP: &http.Client{Timeout: 10 * time.Minute}}
}

// envScoped are the API paths that belong to a Docker environment.
var envScoped = []string{"containers", "images", "networks", "volumes", "system"}

// Scope rewrites /api/v1/<resource>... to /api/v1/environments/<env>/<resource>...
// when an environment is chosen, so every command works against any host
// without knowing about environments. Other paths are left alone.
func (c *Client) Scope(path string) string {
	if c.Env == "" {
		return path
	}
	rest, ok := strings.CutPrefix(path, "/api/v1/")
	if !ok {
		return path
	}
	head, _, _ := strings.Cut(rest, "/")
	head, _, _ = strings.Cut(head, "?")
	for _, r := range envScoped {
		if head == r {
			return "/api/v1/environments/" + url.PathEscape(c.Env) + "/" + rest
		}
	}
	return path
}

// Do sends a JSON request and decodes the JSON response into out (if not nil).
func (c *Client) Do(method, path string, body, out any) error {
	resp, err := c.send(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Raw returns the raw JSON response body.
func (c *Client) Raw(method, path string, body any) (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.Do(method, path, body, &raw)
	return raw, err
}

// Stream returns the response body for streaming endpoints (e.g. logs).
func (c *Client) Stream(path string) (io.ReadCloser, error) {
	resp, err := c.send(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) send(method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.URL+c.Scope(path), r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, errors.New(e.Error + " (run `locci login`)")
		}
		return nil, fmt.Errorf("%s", e.Error)
	}
	return resp, nil
}
