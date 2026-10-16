package cmd

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Checks loopback too: a loopback-only listener would shadow the port Docker publishes.
func hostPortBusy(port string) bool {
	for _, addr := range []string{":" + port, "127.0.0.1:" + port} {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return true
		}
		_ = l.Close()
	}
	return false
}

// Free ports keep their well-known numbers, so a lone project keeps the documented addresses.
func releaseBusyHostPorts(p *types.Project, busy func(string) bool) []string {
	var moved []string
	for name, svc := range p.Services {
		ports := make([]types.ServicePortConfig, len(svc.Ports))
		copy(ports, svc.Ports)
		changed := false
		for i, port := range ports {
			if port.Published == "" || !busy(port.Published) {
				continue
			}
			moved = append(moved, name+":"+port.Published)
			ports[i].Published = ""
			changed = true
		}
		if changed {
			svc.Ports = ports
			p.Services[name] = svc
		}
	}
	sort.Strings(moved)
	return moved
}

// Docker reports a lost bind race in one of these two wordings; the compose SDK has no typed error.
// A port running on another number than requested was assigned by Docker at start.
// Requesting it as "" again keeps the service hash, so Compose leaves the container alone.
func matchRunningHostPorts(p *types.Project, running devHostPorts) {
	for name, svc := range p.Services {
		ports := make([]types.ServicePortConfig, len(svc.Ports))
		copy(ports, svc.Ports)
		changed := false
		for i, port := range ports {
			if got := running.host(name, port.Target); port.Published != "" && got != "" && got != port.Published {
				ports[i].Published = ""
				changed = true
			}
		}
		if changed {
			svc.Ports = ports
			p.Services[name] = svc
		}
	}
}

func isPortAllocatedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "port is already allocated") || strings.Contains(msg, "address already in use")
}

// devHostPorts maps service name, then container port, to the published host port.
type devHostPorts map[string]map[uint32]uint16

// A missing port returns "", never a well-known default: after a move, that default belongs to another project.
func (p devHostPorts) host(service string, target uint32) string {
	if port, ok := p[service][target]; ok {
		return strconv.Itoa(int(port))
	}
	return ""
}

func hostPortsFromContainers(items []container.Summary) devHostPorts {
	ports := devHostPorts{}
	for _, c := range items {
		service := c.Labels[api.ServiceLabel]
		for _, p := range c.Ports {
			if p.PublicPort == 0 {
				continue
			}
			if ports[service] == nil {
				ports[service] = map[uint32]uint16{}
			}
			// Docker reports one entry per address family; they share a port.
			target := uint32(p.PrivatePort)
			if _, seen := ports[service][target]; !seen || p.IP.Is4() {
				ports[service][target] = p.PublicPort
			}
		}
	}
	return ports
}

func lookupHostPorts(ctx context.Context, projectName string) (devHostPorts, error) {
	cli, err := newDockerClient()
	if err != nil {
		return nil, err
	}
	opts := client.ContainerListOptions{Filters: make(client.Filters).Add("label", api.ProjectLabel+"="+projectName)}
	// The containers are already up, so a transient Docker error should not fail the start.
	for attempt := 1; ; attempt++ {
		res, err := cli.ContainerList(ctx, opts)
		if err == nil {
			return hostPortsFromContainers(res.Items), nil
		}
		if attempt == 3 {
			return nil, fmt.Errorf("list containers: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
		}
	}
}

func chooseChatUIAddr(astDir string, reserved map[string]bool) (string, error) {
	stopChatUI(astDir)
	reclaimOrphanedChatUIPort(chatUIAddr)
	if port := pickChatUIPort(reserved, hostPortBusy); port != "" {
		return "127.0.0.1:" + port, nil
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("find a free port for the chat UI: %w", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr, nil
}

// The range sits below the OS ephemeral range, so a port Docker assigns during Up cannot take it.
const (
	chatUIFirstPort = 3100
	chatUILastPort  = 3199
)

func pickChatUIPort(reserved map[string]bool, busy func(string) bool) string {
	for p := chatUIFirstPort; p <= chatUILastPort; p++ {
		port := strconv.Itoa(p)
		if !reserved[port] && !busy(port) {
			return port
		}
	}
	return ""
}

func publishedHostPorts(p *types.Project) map[string]bool {
	ports := map[string]bool{}
	for _, svc := range p.Services {
		for _, port := range svc.Ports {
			if port.Published != "" {
				ports[port.Published] = true
			}
		}
	}
	return ports
}

// startChatUIOnFreePort returns the address the chat UI serves on, or "" when it does
// not serve. Another project can take the picked port before this worker binds it,
// so a taken port gets one retry on the next free one.
func startChatUIOnFreePort(astDir string, reserved map[string]bool, messagingURL, agentName string, exitWithParent bool) string {
	for attempt := 1; attempt <= 2; attempt++ {
		addr, err := chooseChatUIAddr(astDir, reserved)
		if err != nil {
			fmt.Print(msgDevChatUINotStarted(err.Error()))
			return ""
		}
		switch startChatUI(astDir, addr, messagingURL, agentName, exitWithParent) {
		case chatUIReady:
			return addr
		case chatUIPortBusy:
			continue
		default:
			return ""
		}
	}
	fmt.Print(msgDevChatUINotStarted("each port it picked was taken by another process"))
	return ""
}

func chatURLForAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	return "http://localhost:" + port
}

// A worker whose sidecar still answers belongs to another running project, so it keeps the port.
func reclaimOrphanedChatUIPort(addr string) {
	pid, ok := chatUIListenerPID(addr)
	if !ok || !isChatUIProcess(pid) {
		return
	}
	cmdline, err := processCommandLine(pid)
	if err != nil || sidecarAnswers(messagingURLFromCmdline(cmdline)) {
		return
	}
	signalProcessGroup(pid, false)
	if portFreeWithin(addr, 2*time.Second) {
		return
	}
	signalProcessGroup(pid, true)
	portFreeWithin(addr, 2*time.Second)
}

func messagingURLFromCmdline(cmdline string) string {
	fields := strings.Fields(cmdline)
	for i, f := range fields {
		if f == "--messaging-url" && i+1 < len(fields) {
			return fields[i+1]
		}
		if v, ok := strings.CutPrefix(f, "--messaging-url="); ok {
			return v
		}
	}
	return ""
}

func sidecarAnswers(baseURL string) bool {
	host := strings.TrimPrefix(strings.TrimPrefix(baseURL, "http://"), "https://")
	host, _, _ = strings.Cut(host, "/")
	if host == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", host, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
