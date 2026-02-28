package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/postman/astro/apps/astro-queen/internal/config"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Interactively set up server address and mTLS certificates",
	RunE:  runConfigure,
}

func init() {
	rootCmd.AddCommand(configureCmd)
}

func runConfigure(cmd *cobra.Command, args []string) error {
	scanner := bufio.NewScanner(os.Stdin)

	// Server address
	fmt.Print("Server address [localhost:9091]: ")
	scanner.Scan()
	server := strings.TrimSpace(scanner.Text())
	if server == "" {
		server = "localhost:9091"
	}

	// Derive config directory from DefaultPath
	configDir := filepath.Dir(config.DefaultPath())
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Client certificate
	fmt.Println("Client certificate PEM (paste, then press Enter on a blank line after -----END):")
	certPEM := readPEM(scanner)

	// Client key
	fmt.Println("Client key PEM (paste, then press Enter on a blank line after -----END):")
	keyPEM := readPEM(scanner)

	// CA certificate
	fmt.Println("CA certificate PEM (paste, then press Enter on a blank line after -----END):")
	caPEM := readPEM(scanner)

	// Write cert files
	files := map[string]string{
		"client.crt": certPEM,
		"client.key": keyPEM,
		"ca.crt":     caPEM,
	}
	for name, pem := range files {
		path := filepath.Join(configDir, name)
		if err := os.WriteFile(path, []byte(pem), 0600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	// Write config.yaml
	cfg := config.Config{
		Server:   server,
		CertFile: filepath.Join(configDir, "client.crt"),
		KeyFile:  filepath.Join(configDir, "client.key"),
		CAFile:   filepath.Join(configDir, "ca.crt"),
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	configPath := config.DefaultPath()
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("Configuration written to %s\n", configPath)
	return nil
}

// readPEM reads lines from the scanner until a blank line appears after an -----END line.
func readPEM(scanner *bufio.Scanner) string {
	var lines []string
	sawEnd := false
	for scanner.Scan() {
		line := scanner.Text()
		if sawEnd && strings.TrimSpace(line) == "" {
			break
		}
		sawEnd = strings.Contains(line, "-----END")
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n"
}
