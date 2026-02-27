package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

const defaultGateway = "https://jennah-gateway-382915581671.asia-northeast1.run.app"

// GatewayClient sends requests to the Jennah gateway API.
type GatewayClient struct {
	baseURL  string
	email    string
	userID   string
	tenantID string
	provider string
	http     *http.Client
}

// newGatewayClient builds a GatewayClient from flags, env vars, or saved config.
func newGatewayClient(cmd *cobra.Command) (*GatewayClient, error) {
	gateway, _ := cmd.Flags().GetString("gateway")
	email, _ := cmd.Flags().GetString("email")
	userID, _ := cmd.Flags().GetString("user-id")
	provider, _ := cmd.Flags().GetString("provider")

	if gateway == "" {
		gateway = os.Getenv("JENNAH_GATEWAY")
	}
	if email == "" {
		email = os.Getenv("JENNAH_EMAIL")
	}
	if userID == "" {
		userID = os.Getenv("JENNAH_USER_ID")
	}
	if provider == "" {
		provider = os.Getenv("JENNAH_PROVIDER")
	}

	// Fall back to saved config from `jennah login`
	tenantID := ""
	if email == "" || userID == "" {
		if cfg, err := loadConfig(); err == nil && cfg != nil {
			if email == "" {
				email = cfg.Email
			}
			if userID == "" {
				userID = cfg.UserID
			}
			if provider == "" && cfg.Provider != "" {
				provider = cfg.Provider
			}
			tenantID = cfg.TenantID
		}
	}

	if gateway == "" {
		gateway = defaultGateway
	}
	if provider == "" {
		provider = "google"
	}
	if email == "" {
		return nil, fmt.Errorf("not logged in: run 'jennah login --email <email> --user-id <id>'")
	}
	if userID == "" {
		return nil, fmt.Errorf("not logged in: run 'jennah login --email <email> --user-id <id>'")
	}

	return &GatewayClient{
		baseURL:  gateway,
		email:    email,
		userID:   userID,
		tenantID: tenantID,
		provider: provider,
		http:     &http.Client{},
	}, nil
}

// postRaw sends a JSON POST and returns the HTTP status code and raw body.
func (c *GatewayClient) postRaw(path string, body interface{}) (int, []byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest("POST", c.baseURL+path, &buf)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-OAuth-Email", c.email)
	req.Header.Set("X-OAuth-UserId", c.userID)
	req.Header.Set("X-OAuth-Provider", c.provider)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}

// post sends a JSON POST to the gateway and decodes the response into out.
func (c *GatewayClient) post(path string, body interface{}, out interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-OAuth-Email", c.email)
	req.Header.Set("X-OAuth-UserId", c.userID)
	req.Header.Set("X-OAuth-Provider", c.provider)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		var errResp struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("%s: %s", errResp.Code, errResp.Message)
		}
		return fmt.Errorf("gateway error %d: %s", resp.StatusCode, string(respBody))
	}

	if out != nil {
		return json.Unmarshal(respBody, out)
	}
	return nil
}
