package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-queen/internal/client"
	"github.com/postman/astro/apps/astro-queen/internal/config"
	"github.com/postman/astro/apps/astro-queen/internal/server"
)

var (
	serverPort   int
	serverNoOpen bool
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Queen web UI server",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		if serverAddr != "" {
			cfg.Server = serverAddr
		}

		c, err := client.New(cfg)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", cfg.Server, err)
		}
		defer c.Close() //nolint:errcheck

		// Strip the "web/dist" prefix from the embedded FS
		webContent, err := fs.Sub(WebFS, "web/dist")
		if err != nil {
			return fmt.Errorf("embedded web fs: %w", err)
		}

		srv := server.New(c.AdminService(), webContent, serverPort)

		// Open browser (skip with --no-open for dev/reload workflows)
		if !serverNoOpen {
			url := fmt.Sprintf("http://127.0.0.1:%d", serverPort)
			go openBrowser(url)
		}

		return srv.ListenAndServe()
	},
}

func init() {
	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 8888, "HTTP server port")
	serverCmd.Flags().BoolVar(&serverNoOpen, "no-open", false, "Don't open browser on start")
	rootCmd.AddCommand(serverCmd)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("failed to open browser: %v", err)
	}
}
