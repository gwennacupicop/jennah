package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Config holds saved credentials.
type Config struct {
	Email    string `json:"email"`
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id,omitempty"`
	Provider string `json:"provider,omitempty"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jennah", "config.json"), nil
}

func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func prompt(label string) (string, error) {
	fmt.Printf("%s: ", label)
	reader := bufio.NewReader(os.Stdin)
	val, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(val), nil
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Jennah",
	Long:  "jennah login\n\nSaves your email and user ID locally so you don't need to provide them on every command.\nIf no existing account is found, you will be asked to register.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Block if already logged in.
		existing, err := loadConfig()
		if err != nil {
			return err
		}
		if existing != nil {
			fmt.Printf("Already logged in as \033[36m%s\033[0m.\n", existing.UserID)
			fmt.Println("Run 'jennah logout' first before logging in again.")
			return nil
		}

		fmt.Println("Log in to Jennah")
		fmt.Println("────────────────")

		userID, err := prompt("User ID")
		if err != nil {
			return err
		}
		if userID == "" {
			return fmt.Errorf("user ID cannot be empty")
		}

		email, err := prompt("Email")
		if err != nil {
			return err
		}
		if email == "" {
			return fmt.Errorf("email cannot be empty")
		}

		// Temporarily save config so newGatewayClient can read it.
		cfg := &Config{Email: email, UserID: userID, Provider: "google"}
		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save credentials: %w", err)
		}

		// Check if tenant already exists by listing jobs.
		// The gateway auto-creates tenants, so we track whether
		// a tenant row pre-existed by comparing the created_at timestamp.
		fmt.Println()
		fmt.Println("Checking account...")
		gw, err := newGatewayClient(cmd)
		if err != nil {
			// Clean up saved config on error.
			path, _ := configPath()
			os.Remove(path)
			return err
		}

		var tenantResult struct {
			TenantID  string `json:"tenantId"`
			UserEmail string `json:"userEmail"`
			CreatedAt string `json:"createdAt"`
		}
		if err := gw.post("/jennah.v1.DeploymentService/GetCurrentTenant", map[string]interface{}{}, &tenantResult); err != nil {
			path, _ := configPath()
			os.Remove(path)
			return fmt.Errorf("could not reach server: %w", err)
		}

		// Determine if this is a new account by checking if created_at is very recent (within 5s).
		isNew := false
		if t, parseErr := time.Parse(time.RFC3339, tenantResult.CreatedAt); parseErr == nil {
			isNew = time.Since(t) < 5*time.Second
		}

		if isNew {
			// Just created — ask for confirmation.
			fmt.Println()
			fmt.Printf("No existing account found for User ID \033[36m%s\033[0m.\n", userID)
			answer, err := prompt("Register as a new user? [y/N]")
			if err != nil {
				path, _ := configPath()
				os.Remove(path)
				return err
			}
			if strings.ToLower(strings.TrimSpace(answer)) != "y" && strings.ToLower(strings.TrimSpace(answer)) != "yes" {
				path, _ := configPath()
				os.Remove(path)
				fmt.Println("Login cancelled.")
				return nil
			}
			fmt.Println()
			fmt.Println("✅ Account registered.")
		} else {
			fmt.Println("✅ Existing account found.")
		}

		// Save tenant ID into config.
		cfg.TenantID = tenantResult.TenantID
		if err := saveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save tenant id: %w", err)
		}

		fmt.Println()
		fmt.Printf("Logged in as \033[36m%s\033[0m\n", userID)
		fmt.Println()
		rootCmd.Help()
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out of Jennah",
	Long:  "jennah logout\n\nRemoves your locally saved credentials.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		if cfg == nil {
			fmt.Println("Not logged in.")
			return nil
		}

		path, err := configPath()
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}

		fmt.Println("✅ Logged out successfully.")
		return nil
	},
}
