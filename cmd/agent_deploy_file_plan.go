package cmd

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	deployspec "github.com/astropods/astro-spec/deploy"
)

// deployPlanRow is one field the deploy file sets. Before is empty for a new
// agent, and NotCompared marks a value the template does not echo back.
type deployPlanRow struct {
	Label       string
	Before      string
	After       string
	NotCompared bool
}

type deployPlan struct {
	New    bool
	Pinned bool
	Build  deployPlanRow
	// Latest is the newest registered build, set only when it differs from Build.After.
	Latest string
	Rows   []deployPlanRow
	Kept   []string
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
	add := func(label string, f func(deployTemplateResponse) string) {
		plan.Rows = append(plan.Rows, deployPlanRow{Label: label, Before: before(f), After: f(proposed)})
	}

	plan.Build = deployPlanRow{Label: "build", Before: before(templateBuild), After: templateBuild(proposed)}
	if latestBuild != plan.Build.After {
		plan.Latest = latestBuild
	}

	if entry.Cluster != "" {
		add("cluster", templateCluster)
	} else {
		plan.Kept = append(plan.Kept, "cluster")
	}
	if len(entry.Models) > 0 {
		for _, name := range sortedKeys(entry.Models) {
			add("model "+name, func(t deployTemplateResponse) string { return templateModel(t, name) })
		}
	} else {
		plan.Kept = append(plan.Kept, "models")
	}
	if entry.Interfaces != nil {
		add("interfaces", templateAdapters)
	} else {
		plan.Kept = append(plan.Kept, "interfaces")
	}
	if entry.Public != nil {
		add("public", templatePublic)
	} else {
		plan.Kept = append(plan.Kept, "public")
	}
	if len(entry.Variables) > 0 {
		plan.Rows = append(plan.Rows, deployPlanRow{Label: "variables", After: variableNames(entry.Variables), NotCompared: true})
	} else {
		plan.Kept = append(plan.Kept, "variables")
	}
	if len(entry.Schedules) > 0 {
		for _, name := range sortedKeys(entry.Schedules) {
			add("schedule "+name, func(t deployTemplateResponse) string { return t.Schedules[name] })
		}
	} else {
		plan.Kept = append(plan.Kept, "schedules")
	}
	if len(entry.Knowledge) > 0 {
		for _, name := range sortedKeys(entry.Knowledge) {
			add("knowledge "+name, func(t deployTemplateResponse) string { return templateKnowledge(t, name) })
		}
	} else {
		plan.Kept = append(plan.Kept, "knowledge")
	}
	if entry.Resources != nil {
		for _, key := range resourceKeys(*entry.Resources) {
			add("sizing "+key, func(t deployTemplateResponse) string { return templateSizing(t, key) })
		}
	} else {
		plan.Kept = append(plan.Kept, "resources")
	}
	return plan
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

func templateSizing(t deployTemplateResponse, key string) string {
	p := t.Provisioning
	var c *deployTemplateComponent
	if key == "agent" {
		c = p.Agent
	} else {
		group, name, _ := strings.Cut(key, ".")
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
		return "default"
	}
	if c.Sizing != nil && c.Sizing.Mode == deployspec.SizingAuto {
		bounds := []string{}
		if s := sizeRange("cpu", c.Sizing.Min, c.Sizing.Max, func(s *deployTemplateSize) string { return s.CPU }); s != "" {
			bounds = append(bounds, s)
		}
		if s := sizeRange("memory", c.Sizing.Min, c.Sizing.Max, func(s *deployTemplateSize) string { return s.Memory }); s != "" {
			bounds = append(bounds, s)
		}
		if len(bounds) == 0 {
			return "auto"
		}
		return "auto (" + strings.Join(bounds, ", ") + ")"
	}
	if c.Compute == nil {
		return "fixed"
	}
	var parts []string
	if c.Compute.CPU != "" {
		parts = append(parts, "cpu "+c.Compute.CPU)
	}
	if c.Compute.Memory != "" {
		parts = append(parts, "memory "+c.Compute.Memory)
	}
	return "fixed (" + strings.Join(parts, ", ") + ")"
}

func sizeRange(label string, min, max *deployTemplateSize, get func(*deployTemplateSize) string) string {
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
	return label + " " + lo + "–" + hi
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
