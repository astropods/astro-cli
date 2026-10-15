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

func isPortAllocatedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "port is already allocated")
}

// devHostPorts maps service name, then container port, to the published host port.
type devHostPorts map[string]map[uint32]uint16

func (p devHostPorts) host(service string, target uint32, fallback string) string {
	if port, ok := p[service][target]; ok {
		return strconv.Itoa(int(port))
	}
	return fallback
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
	res, err := cli.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", api.ProjectLabel+"="+projectName),
	})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return hostPortsFromContainers(res.Items), nil
}

func chooseChatUIAddr(astDir string) (string, error) {
	stopChatUI(astDir)
	reclaimOrphanedChatUIPort(chatUIAddr)
	if portFreeWithin(chatUIAddr, 0) {
		return chatUIAddr, nil
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("find a free port for the chat UI: %w", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr, nil
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
