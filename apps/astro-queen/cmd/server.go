package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-queen/internal/client"
	"github.com/astropods/astro/apps/astro-queen/internal/config"
	"github.com/astropods/astro/apps/astro-queen/internal/server"
)

var (
	serverPort   int
	serverNoOpen bool
)

var environments = map[string]string{
	"prod":    "admin.astropods.ai:443",
	"preview": "admin.astropod.ai:443",
}

func init() {
	for env, addr := range environments {
		envCmd := &cobra.Command{
			Use:   env,
			Short: fmt.Sprintf("Commands for %s (%s)", env, addr),
		}

		adminCmd := newAdminCmd(env, addr)
		adminCmd.Flags().IntVarP(&serverPort, "port", "p", 8888, "HTTP server port")
		adminCmd.Flags().BoolVar(&serverNoOpen, "no-open", false, "Don't open browser on start")
		envCmd.AddCommand(adminCmd)

		rootCmd.AddCommand(envCmd)
	}
}

func newAdminCmd(env, addr string) *cobra.Command {
	return &cobra.Command{
		Use:   "admin",
		Short: fmt.Sprintf("Start Queen admin UI connected to %s (%s)", env, addr),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(beeArt)
			fmt.Printf("  Connecting to %s (%s)\n\n", env, addr)

			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg.Server = addr

			c, err := client.New(cfg)
			if err != nil {
				return fmt.Errorf("connect to %s: %w", addr, err)
			}
			defer c.Close() //nolint:errcheck

			// Strip the "web/dist" prefix from the embedded FS
			webContent, err := fs.Sub(WebFS, "web/dist")
			if err != nil {
				return fmt.Errorf("embedded web fs: %w", err)
			}

			srv := server.New(c.AdminService(), webContent, serverPort, OpenAPIJSON)

			// Open browser (skip with --no-open for dev/reload workflows)
			if !serverNoOpen {
				url := fmt.Sprintf("http://127.0.0.1:%d", serverPort)
				go openBrowser(url)
			}

			return srv.ListenAndServe()
		},
	}
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
