package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	deployspec "github.com/astropods/astro-spec/deploy"
)

// deployPlanRow is one setting the deploy file sets. Key names the entry for
// a per-entry setting such as a model. Before is empty for a new agent, and
// NotCompared marks a value the template does not echo back.
type deployPlanRow struct {
	Label       string
	Key         string
	Before      string
	After       string
	NotCompared bool
}

// deployPlanSizing is one component's sizing, split into table cells. HA is
// empty for a component without high availability.
type deployPlanSizing struct {
	Mode   string
	CPU    string
	Memory string
	HA     string
}

// deployPlanResource is one component under resources. Before is nil for a
// new agent.
type deployPlanResource struct {
	Name   string
	Before *deployPlanSizing
	After  deployPlanSizing
}

type deployPlan struct {
	New    bool
	Pinned bool
	Build  deployPlanRow
	// Latest is the newest registered build, set only when it differs from Build.After.
	Latest    string
	Rows      []deployPlanRow
	Resources []deployPlanResource
	// Kept names what the file leaves out.
	Kept []string
}

// storedDeployTemplate is the template a deployment runs today. With no
// deploymentID it is a new agent's template, whose build is the latest registered one.
func storedDeployTemplate(cmd *cobra.Command, at AccountToken, verbose bool, blueprint, deploymentID string) (*deployTemplateResponse, error) {
	req := deployTemplateRequest{DeploymentID: deploymentID, TargetAccount: at.Account}
	var resp deployTemplateResponse
	u := apiPath(blueprintBaseURL(), at.Account, "agents", blueprint, "deployment-template")
	if _, err := apiCall(cmd.Context(), http.MethodPost, u, req, at.Token, verbose, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func buildDeployPlan(entry deployspec.Environment, stored *deployTemplateResponse, proposed deployTemplateResponse, pinned bool, latestBuild string) deployPlan {
	plan := deployPlan{New: stored == nil, Pinned: pinned}
	before := func(f func(deployTemplateResponse) string) string {
		if stored == nil {
			return ""
		}
		return f(*stored)
	}
	add := func(label, key string, f func(deployTemplateResponse) string) {
		plan.Rows = append(plan.Rows, deployPlanRow{Label: label, Key: key, Before: before(f), After: f(proposed)})
	}

	plan.Build = deployPlanRow{Label: "Build", Before: before(templateBuild), After: templateBuild(proposed)}
	if latestBuild != plan.Build.After {
		plan.Latest = latestBuild
	}

	if entry.Cluster != "" {
		add("Cluster", "", templateCluster)
	} else {
		plan.Kept = append(plan.Kept, "cluster")
	}
	if len(entry.Models) > 0 {
		for _, name := range sortedKeys(entry.Models) {
			add("Model", name, func(t deployTemplateResponse) string { return templateModel(t, name) })
		}
	} else {
		plan.Kept = append(plan.Kept, "models")
	}
	if entry.Interfaces != nil {
		add("Interfaces", "", templateAdapters)
	} else {
		plan.Kept = append(plan.Kept, "interfaces")
	}
	if entry.Public != nil {
		add("Public", "", templatePublic)
	} else {
		plan.Kept = append(plan.Kept, "public")
	}
	if len(entry.Variables) > 0 {
		plan.Rows = append(plan.Rows, deployPlanRow{Label: "Variables", After: variableNames(entry.Variables), NotCompared: true})
	} else {
		plan.Kept = append(plan.Kept, "variables")
	}
	if len(entry.Schedules) > 0 {
		for _, name := range sortedKeys(entry.Schedules) {
			add("Schedule", name, func(t deployTemplateResponse) string { return t.Schedules[name] })
		}
	} else {
		plan.Kept = append(plan.Kept, "schedules")
	}
	if len(entry.Knowledge) > 0 {
		for _, name := range sortedKeys(entry.Knowledge) {
			add("Knowledge", name, func(t deployTemplateResponse) string { return templateKnowledge(t, name) })
		}
	} else {
		plan.Kept = append(plan.Kept, "knowledge bindings")
	}
	if entry.Resources != nil {
		keys := resourceKeys(*entry.Resources)
		for _, key := range keys {
			r := deployPlanResource{Name: resourceName(key, keys), After: templateSizing(proposed, key)}
			if stored != nil {
				b := templateSizing(*stored, key)
				r.Before = &b
			}
			plan.Resources = append(plan.Resources, r)
		}
	} else {
		plan.Kept = append(plan.Kept, "resources")
	}
	return plan
}

// resourceName is the entry name, or group.name when two groups share it.
func resourceName(key string, keys []string) string {
	_, name, found := strings.Cut(key, ".")
	if !found {
		return key
	}
	for _, other := range keys {
		if _, n, ok := strings.Cut(other, "."); ok && other != key && n == name {
			return key
		}
	}
	return name
}

func templateBuild(t deployTemplateResponse) string {
	var tmpl struct {
		Source struct {
			Build string `json:"build"`
		} `json:"source"`
	}
	_ = json.Unmarshal(t.Template, &tmpl)
	return tmpl.Source.Build
}

func templateCluster(t deployTemplateResponse) string {
	var tmpl struct {
		Target struct {
			ClusterID string `json:"cluster_id"`
		} `json:"target"`
	}
	_ = json.Unmarshal(t.Template, &tmpl)
	return tmpl.Target.ClusterID
}

func templateModel(t deployTemplateResponse, name string) string {
	for _, m := range t.Models {
		if m.Name == name {
			return m.Selected
		}
	}
	return ""
}

func templateAdapters(t deployTemplateResponse) string {
	if len(t.Interfaces.Adapters) == 0 {
		return "none"
	}
	return strings.Join(t.Interfaces.Adapters, ", ")
}

func templatePublic(t deployTemplateResponse) string {
	onOff := func(p *deployTemplatePublic) string {
		if p != nil && p.Public {
			return "on"
		}
		return "off"
	}
	var web, custom *deployTemplatePublic
	if t.Interfaces.Auth != nil {
		web, custom = t.Interfaces.Auth.Web, t.Interfaces.Auth.Custom
	}
	return "web " + onOff(web) + ", custom " + onOff(custom)
}

func templateKnowledge(t deployTemplateResponse, name string) string {
	if t.Bindings == nil {
		return ""
	}
	b, ok := t.Bindings.Knowledge[name]
	if !ok {
		return ""
	}
	if b.Name != "" {
		return b.Name
	}
	return b.ARN
}

func templateSizing(t deployTemplateResponse, key string) deployPlanSizing {
	p := t.Provisioning
	var c *deployTemplateComponent
	group, name, _ := strings.Cut(key, ".")
	if key == "agent" {
		c = p.Agent
	} else {
		var m map[string]deployTemplateComponent
		switch group {
		case "models":
			m = p.Models
		case "knowledge":
			m = p.Knowledge
		case "integrations":
			m = p.Integrations
		case "ingestion":
			m = p.Ingestion
		}
		if v, ok := m[name]; ok {
			c = &v
		}
	}
	if c == nil {
		return deployPlanSizing{Mode: "default"}
	}
	s := deployPlanSizing{Mode: deployspec.SizingFixed}
	if c.Sizing != nil && c.Sizing.Mode == deployspec.SizingAuto {
		s.Mode = deployspec.SizingAuto
		s.CPU = sizeRange(c.Sizing.Min, c.Sizing.Max, func(s *deployTemplateSize) string { return s.CPU })
		s.Memory = sizeRange(c.Sizing.Min, c.Sizing.Max, func(s *deployTemplateSize) string { return s.Memory })
	} else if c.Compute != nil {
		s.CPU, s.Memory = c.Compute.CPU, c.Compute.Memory
	}
	if c.HighAvailability != nil && *c.HighAvailability {
		s.HA = "on"
		if n := templateKnowledgeReplicas(t, name); group == "knowledge" && n > 1 {
			s.HA = fmt.Sprintf("on, %d instances", n)
		}
	}
	return s
}

func templateKnowledgeReplicas(t deployTemplateResponse, name string) int {
	var tmpl struct {
		Knowledge map[string]struct {
			Replicas int `json:"replicas"`
		} `json:"knowledge"`
	}
	_ = json.Unmarshal(t.Template, &tmpl)
	return tmpl.Knowledge[name].Replicas
}

func sizeRange(min, max *deployTemplateSize, get func(*deployTemplateSize) string) string {
	lo, hi := "", ""
	if min != nil {
		lo = get(min)
	}
	if max != nil {
		hi = get(max)
	}
	if lo == "" && hi == "" {
		return ""
	}
	return lo + "–" + hi
}

func variableNames(vars map[string]deployspec.Variable) string {
	names := make([]string, 0, len(vars))
	for _, name := range sortedKeys(vars) {
		if vars[name].Ref != "" {
			names = append(names, name+" (ref "+vars[name].Ref+")")
		} else {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func resourceKeys(r deployspec.Resources) []string {
	var keys []string
	if r.Agent != nil {
		keys = append(keys, "agent")
	}
	groups := []struct {
		name string
		m    map[string]deployspec.Compute
	}{{"models", r.Models}, {"knowledge", r.Knowledge}, {"integrations", r.Integrations}, {"ingestion", r.Ingestion}}
	for _, g := range groups {
		for _, name := range sortedKeys(g.m) {
			keys = append(keys, g.name+"."+name)
		}
	}
	return keys
}
