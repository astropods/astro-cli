package cmd

import (
	"fmt"
	"net/http"
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
	// Projects started before the dev label existed still have a messaging sidecar.
	var items []container.Summary
	seen := map[string]bool{}
	for _, f := range []string{composeBuilder.DevAgentLabel, api.ServiceLabel + "=astro-messaging"} {
		res, err := cli.ContainerList(cmd.Context(), client.ContainerListOptions{Filters: make(client.Filters).Add("label", f)})
		if err != nil {
			return fmt.Errorf("list containers: %w", err)
		}
		for _, c := range res.Items {
			if !seen[c.ID] {
				seen[c.ID] = true
				items = append(items, c)
			}
		}
	}
	projects := localAgentsFromContainers(items)

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
			if !chatUIServing(httpClient, a.ChatURL) {
				chat += " (not serving)"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", a.Name, chat, a.Containers, shortenHome(a.Dir, home)) //nolint:errcheck,gosec
	}
	return tw.Flush()
}

func localAgentsFromContainers(items []container.Summary) []localAgent {
	byProject := map[string]*localAgent{}
	for _, c := range items {
		project := c.Labels[api.ProjectLabel]
		if project == "" {
			continue
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
	for _, a := range byProject {
		agents = append(agents, *a)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents
}

func chatUIServing(c *http.Client, baseURL string) bool {
	resp, err := c.Get(strings.TrimRight(baseURL, "/") + chatui.HealthPath) //nolint:gosec,noctx // localhost URL from our own label, short timeout
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
