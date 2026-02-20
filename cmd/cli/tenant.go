package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Manage tenants",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var tenantCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new tenant",
	Long:  "jennah tenant create --name <name> --email <email>",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		email, _ := cmd.Flags().GetString("email")

		if name == "" {
			return fmt.Errorf("--name flag is required")
		}
		if email == "" {
			return fmt.Errorf("--email flag is required")
		}

		db, closeDB, err := newDBClient(cmd)
		if err != nil {
			return err
		}
		defer closeDB()

		tenantID := newJobID() // auto-generate UUID
		if err := db.InsertTenant(context.Background(), tenantID, email, "local", name); err != nil {
			return fmt.Errorf("failed to create tenant: %w", err)
		}

		fmt.Printf("\u2713 Tenant created\n")
		fmt.Printf("  Name:      %s\n", name)
		fmt.Printf("  Email:     %s\n", email)
		fmt.Printf("  Tenant ID: %s\n", tenantID)
		return nil
	},
}

var tenantListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tenants",
	Long:  "jennah tenant list",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, closeDB, err := newDBClient(cmd)
		if err != nil {
			return err
		}
		defer closeDB()

		tenants, err := db.ListTenants(context.Background())
		if err != nil {
			return fmt.Errorf("failed to list tenants: %w", err)
		}

		if len(tenants) == 0 {
			fmt.Println("No tenants found.")
			return nil
		}

		fmt.Printf("%-20s  %-30s  %-36s  %s\n", "NAME", "EMAIL", "TENANT ID", "CREATED")
		fmt.Println(strings.Repeat("\u2500", 96))
		for _, t := range tenants {
			name := ""
			if t.OAuthUserId.Valid {
				name = t.OAuthUserId.StringVal
			}
			email := ""
			if t.UserEmail.Valid {
				email = t.UserEmail.StringVal
			}
			fmt.Printf("%-20s  %-30s  %-36s  %s\n", name, email, t.TenantId, t.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		return nil
	},
}

var tenantDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a tenant and all its jobs",
	Long:  "jennah tenant delete --tenant-id <id>",
	RunE: func(cmd *cobra.Command, args []string) error {
		tenantID, _ := cmd.Flags().GetString("tenant-id")
		if tenantID == "" {
			return fmt.Errorf("--tenant-id flag is required")
		}

		db, closeDB, err := newDBClient(cmd)
		if err != nil {
			return err
		}
		defer closeDB()

		if err := db.DeleteTenant(context.Background(), tenantID); err != nil {
			return fmt.Errorf("failed to delete tenant: %w", err)
		}

		fmt.Printf("✓ Tenant %q deleted (and all its jobs)\n", tenantID)
		return nil
	},
}

func init() {
	tenantCreateCmd.Flags().String("name", "", "Tenant name (required)")
	tenantCreateCmd.Flags().String("email", "", "User email address (required)")
	tenantCmd.AddCommand(tenantCreateCmd)
	tenantCmd.AddCommand(tenantListCmd)
	tenantCmd.AddCommand(tenantDeleteCmd)
}
