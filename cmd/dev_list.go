package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/chatui"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
)

var devListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "ps"},
	Short:   "List local agents running on this machine",
	Long: `List the local agents running on this machine, from any directory: the
agent name, its chat UI address, how many of its containers run, and the
project directory.`,
	Args: cobra.NoArgs,
	RunE: runDevList,
}

func init() {
	devCmd.AddCommand(devListCmd)
}

type localAgent struct {
	Name       string
	Dir        string
	ChatURL    string
	Containers int
}

func runDevList(cmd *cobra.Command, _ []string) error {
	cli, err := newDockerClient()
	if err != nil {
		return err
	}
	res, err := cli.ContainerList(cmd.Context(), client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", api.ProjectLabel),
	})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	projects := localAgentsFromContainers(res.Items)

	w := cmd.OutOrStdout()
	if len(projects) == 0 {
		fmt.Fprint(w, msgDevListEmpty()) //nolint:errcheck,gosec
		return nil
	}
	home, _ := os.UserHomeDir()
	httpClient := &http.Client{Timeout: 500 * time.Millisecond}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tCHAT\tCONTAINERS\tDIRECTORY") //nolint:errcheck,gosec
	for _, a := range projects {
		chat := "-"
		if a.ChatURL != "" {
			chat = a.ChatURL
			if !isLocalHTTPURL(a.ChatURL) {
				chat += " (not local, unchecked)"
			} else if !chatUIServing(httpClient, a.ChatURL) {
				chat += " (not serving)"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", a.Name, chat, a.Containers, shortenHome(a.Dir, home)) //nolint:errcheck,gosec
	}
	return tw.Flush()
}

// Projects started before the dev label existed are recognized by their messaging sidecar.
func localAgentsFromContainers(items []container.Summary) []localAgent {
	byProject := map[string]*localAgent{}
	isDev := map[string]bool{}
	for _, c := range items {
		project := c.Labels[api.ProjectLabel]
		if project == "" {
			continue
		}
		if c.Labels[composeBuilder.DevAgentLabel] != "" || c.Labels[api.ServiceLabel] == "astro-messaging" {
			isDev[project] = true
		}
		a := byProject[project]
		if a == nil {
			a = &localAgent{Name: project}
			byProject[project] = a
		}
		a.Containers++
		if name := c.Labels[composeBuilder.DevAgentLabel]; name != "" {
			a.Name = name
		}
		if dir := c.Labels[api.WorkingDirLabel]; dir != "" {
			a.Dir = dir
		}
		if url := c.Labels[composeBuilder.DevChatURLLabel]; url != "" {
			a.ChatURL = url
		}
	}
	agents := make([]localAgent, 0, len(byProject))
	for project, a := range byProject {
		if isDev[project] {
			agents = append(agents, *a)
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents
}

// Any container can set the label, so only a loopback http URL is fetched.
func isLocalHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1"
}

func chatUIServing(c *http.Client, baseURL string) bool {
	resp, err := c.Get(strings.TrimRight(baseURL, "/") + chatui.HealthPath) //nolint:gosec,noctx // loopback URL (isLocalHTTPURL), short timeout
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func shortenHome(path, home string) string {
	if home == "" || path == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}
