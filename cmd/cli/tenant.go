package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Manage your tenant account",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var tenantWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show your tenant info",
	Long:  "jennah tenant whoami",
	RunE: func(cmd *cobra.Command, args []string) error {
		gw, err := newGatewayClient(cmd)
		if err != nil {
			return err
		}

		var result struct {
			TenantID      string `json:"tenantId"`
			UserEmail     string `json:"userEmail"`
			OAuthProvider string `json:"oauthProvider"`
			CreatedAt     string `json:"createdAt"`
		}
		if err := gw.post("/jennah.v1.DeploymentService/GetCurrentTenant", map[string]interface{}{}, &result); err != nil {
			return fmt.Errorf("failed to get tenant: %w", err)
		}

		fmt.Println("Tenant Info")
		fmt.Println(strings.Repeat("─", 40))
		fmt.Printf("Tenant ID: %s\n", result.TenantID)
		fmt.Printf("Email:     %s\n", result.UserEmail)
		fmt.Printf("Provider:  %s\n", result.OAuthProvider)
		fmt.Printf("Created:   %s\n", result.CreatedAt)
		return nil
	},
}

func init() {
	tenantCmd.AddCommand(tenantWhoamiCmd)
}
