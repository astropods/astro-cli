package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/postman/astro/apps/astro-queen/internal/config"
	"github.com/postman/astro/apps/astro-queen/internal/opauth"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate via 1Password and fetch mTLS certificates",
	RunE:  runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print(beeArt)

	// 1Password account name
	fmt.Print("1Password account name: ")
	scanner.Scan()
	opAccount := strings.TrimSpace(scanner.Text())
	if opAccount == "" {
		return fmt.Errorf("1Password account name is required")
	}

	// Fetch certs from 1Password
	fmt.Println("Fetching certificates from 1Password...")
	ctx := context.Background()
	certPEM, keyPEM, caPEM, err := opauth.FetchCerts(ctx, opAccount)
	if err != nil {
		return fmt.Errorf("fetch certs from 1password: %w", err)
	}
	fmt.Println("Certificates fetched successfully.")

	// Write cert files to conventional paths
	certFiles := map[string]string{
		config.CertFile(): certPEM,
		config.KeyFile():  keyPEM,
		config.CAFile():   caPEM,
	}
	for path, pem := range certFiles {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}
		if err := os.WriteFile(path, []byte(pem), 0600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	// Write config.yaml
	cfg := config.Config{
		OPAccount: opAccount,
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	configPath := config.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("Configuration written to %s\n", configPath)
	return nil
}
