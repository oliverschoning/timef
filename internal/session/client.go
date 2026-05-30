package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const BaseURL = "https://go.poweroffice.net"

type Client struct {
	http     *http.Client
	clientID string
}

func clientIDPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "timef", "client_id")
}

func loadClientID() (string, error) {
	if id := strings.TrimSpace(os.Getenv("TIMEF_CLIENT_ID")); id != "" {
		return id, nil
	}
	data, err := os.ReadFile(clientIDPath())
	if err != nil {
		return "", fmt.Errorf("client ID not set — set TIMEF_CLIENT_ID env var or write it to %s\n(grab from DevTools: any /api/* request → request headers → `goclientid`)", clientIDPath())
	}
	return strings.TrimSpace(string(data)), nil
}

func NewClient() (*Client, error) {
	clientID, err := loadClientID()
	if err != nil {
		return nil, err
	}

	cookies, err := LoadCookies()
	if err != nil {
		return nil, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(BaseURL)
	jar.SetCookies(u, cookies)

	return &Client{
		http: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		clientID: clientID,
	}, nil
}

func (c *Client) Do(method, path string, body interface{}) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-CSRF", "1")
	req.Header.Set("__requestverificationtoken", "null")
	req.Header.Set("goclientid", c.clientID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		hint := "log in via your browser, then retry"
		if resp.StatusCode == 401 {
			hint = "run `timef login` (browser may still be flushing the session cookie to disk — wait a few seconds)"
		}
		return nil, fmt.Errorf("not authenticated (HTTP %d) — %s\nbody: %s", resp.StatusCode, hint, string(data))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}
