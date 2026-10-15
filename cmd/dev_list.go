package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	chats := findChatUIs(chatUIPorts())
	home, _ := os.UserHomeDir()
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tCHAT\tCONTAINERS\tDIRECTORY") //nolint:errcheck,gosec
	for _, a := range projects {
		chat := chats[a.Name]
		if chat == "" {
			chat = "-"
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

func chatUIPorts() []int {
	ports := make([]int, 0, chatUILastPort-chatUIFirstPort+1)
	for p := chatUIFirstPort; p <= chatUILastPort; p++ {
		ports = append(ports, p)
	}
	return ports
}

// findChatUIs maps agent name to chat URL by asking each chat UI worker which agent
// it serves, so the address is never stale. A worker from an older CLI reports none.
func findChatUIs(ports []int) map[string]string {
	c := &http.Client{Timeout: 300 * time.Millisecond}
	found := map[string]string{}
	for _, p := range ports {
		addr := "127.0.0.1:" + strconv.Itoa(p)
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err != nil {
			continue
		}
		_ = conn.Close()
		if agent := chatUIAgent(c, "http://"+addr+chatui.HealthPath); agent != "" {
			found[agent] = chatURLForAddr(addr)
		}
	}
	return found
}

func chatUIAgent(c *http.Client, healthURL string) string {
	resp, err := c.Get(healthURL) //nolint:gosec,noctx // loopback URL built here, short timeout
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Agent string `json:"agent"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&body) != nil {
		return ""
	}
	return body.Agent
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
