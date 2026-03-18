package spec

import (
	"encoding/json"
	"fmt"
)

// CloneDeploymentSpec returns a deep copy of a deployment spec by marshalling
// through JSON.  Used to snapshot the server-generated template before user
// values are applied, enabling Rule 19 editable enforcement.
func CloneDeploymentSpec(ds *AstroDeploymentSpec) *AstroDeploymentSpec {
	data, err := json.Marshal(ds)
	if err != nil {
		panic(fmt.Sprintf("CloneDeploymentSpec: %v", err))
	}
	var clone AstroDeploymentSpec
	_ = json.Unmarshal(data, &clone)
	return &clone
}

// EnforceEditable compares a user-filled submitted spec against the server-generated
// template and returns error strings for every server-owned field that was changed.
//
// Server-owned fields are all fields NOT listed in template.Editable.  Rather than
// parsing the path strings generically, this function encodes the invariants directly
// from the canonical editable set produced by defaultEditableFields:
//
//	Editable:   agent.replicas, agent.resources, agent.environment,
//	            agent.healthcheck, agent.update, agent.endpoints.*.expose,
//	            models/knowledge/tools.*.replicas/resources/gpu/environment/healthcheck/update,
//	            knowledge.*.storage, ingestion.*.resources/trigger.schedule/environment,
//	            interfaces.adapters, interfaces.resources, interfaces.endpoints.*.expose,
//	            variables.*.value, variables.*.targets,
//	            observability.enabled/resources/environment
//
//	Server-owned (checked here): source.*, agent.image, agent.endpoints.*.port/protocol,
//	            component images and endpoint ports/protocols, ingestion image/trigger.type,
//	            interfaces.image, interfaces.endpoints.*.port/protocol,
//	            variables.*.secret/optional, set of components/variables/ingestion entries.
func EnforceEditable(template, submitted *AstroDeploymentSpec) []string {
	var errs []string

	// source.* — entirely server-owned
	if template.Source.Name != submitted.Source.Name {
		errs = append(errs, "source.name: server-owned field cannot be changed")
	}
	if template.Source.Build != submitted.Source.Build {
		errs = append(errs, "source.build: server-owned field cannot be changed")
	}
	if template.Source.Registry != submitted.Source.Registry {
		errs = append(errs, "source.registry: server-owned field cannot be changed")
	}
	if template.Source.Account != submitted.Source.Account {
		errs = append(errs, "source.account: server-owned field cannot be changed")
	}

	// agent.image
	if template.Agent.Image != submitted.Agent.Image {
		errs = append(errs, "agent.image: server-owned field cannot be changed")
	}
	// agent.endpoints.*.port and *.protocol (expose is editable)
	errs = append(errs, enforceEndpoints("agent", template.Agent.Endpoints, submitted.Agent.Endpoints)...)

	// models — image and endpoint ports/protocols are server-owned
	for name, tmpl := range template.Models {
		subm, ok := submitted.Models[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("models.%s: server-owned component cannot be removed", name))
			continue
		}
		if tmpl.Image != subm.Image {
			errs = append(errs, fmt.Sprintf("models.%s.image: server-owned field cannot be changed", name))
		}
		errs = append(errs, enforceEndpoints(fmt.Sprintf("models.%s", name), tmpl.Endpoints, subm.Endpoints)...)
	}
	for name := range submitted.Models {
		if _, ok := template.Models[name]; !ok {
			errs = append(errs, fmt.Sprintf("models.%s: cannot add components not present in template", name))
		}
	}

	// knowledge
	for name, tmpl := range template.Knowledge {
		subm, ok := submitted.Knowledge[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("knowledge.%s: server-owned component cannot be removed", name))
			continue
		}
		if tmpl.Image != subm.Image {
			errs = append(errs, fmt.Sprintf("knowledge.%s.image: server-owned field cannot be changed", name))
		}
		if tmpl.Persistent != subm.Persistent {
			errs = append(errs, fmt.Sprintf("knowledge.%s.persistent: server-owned field cannot be changed", name))
		}
		errs = append(errs, enforceEndpoints(fmt.Sprintf("knowledge.%s", name), tmpl.Endpoints, subm.Endpoints)...)
	}
	for name := range submitted.Knowledge {
		if _, ok := template.Knowledge[name]; !ok {
			errs = append(errs, fmt.Sprintf("knowledge.%s: cannot add components not present in template", name))
		}
	}

	// tools
	for name, tmpl := range template.Tools {
		subm, ok := submitted.Tools[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("tools.%s: server-owned component cannot be removed", name))
			continue
		}
		if tmpl.Image != subm.Image {
			errs = append(errs, fmt.Sprintf("tools.%s.image: server-owned field cannot be changed", name))
		}
		errs = append(errs, enforceEndpoints(fmt.Sprintf("tools.%s", name), tmpl.Endpoints, subm.Endpoints)...)
	}
	for name := range submitted.Tools {
		if _, ok := template.Tools[name]; !ok {
			errs = append(errs, fmt.Sprintf("tools.%s: cannot add components not present in template", name))
		}
	}

	// ingestion — image and trigger.type are server-owned; schedule is editable
	for name, tmpl := range template.Ingestion {
		subm, ok := submitted.Ingestion[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("ingestion.%s: server-owned pipeline cannot be removed", name))
			continue
		}
		if tmpl.Image != subm.Image {
			errs = append(errs, fmt.Sprintf("ingestion.%s.image: server-owned field cannot be changed", name))
		}
		if tmpl.Trigger.Type != subm.Trigger.Type {
			errs = append(errs, fmt.Sprintf("ingestion.%s.trigger.type: server-owned field cannot be changed", name))
		}
		errs = append(errs, enforceEndpoints(fmt.Sprintf("ingestion.%s", name), tmpl.Endpoints, subm.Endpoints)...)
	}
	for name := range submitted.Ingestion {
		if _, ok := template.Ingestion[name]; !ok {
			errs = append(errs, fmt.Sprintf("ingestion.%s: cannot add pipelines not present in template", name))
		}
	}

	// interfaces — image and endpoint ports/protocols are server-owned;
	// adapters and endpoints.*.expose are editable
	if template.Interfaces != nil && submitted.Interfaces != nil {
		if template.Interfaces.Image != submitted.Interfaces.Image {
			errs = append(errs, "interfaces.image: server-owned field cannot be changed")
		}
		errs = append(errs, enforceEndpoints("interfaces", template.Interfaces.Endpoints, submitted.Interfaces.Endpoints)...)
	}

	// variables — value and targets are editable; secret and optional are server-owned
	for name, tmpl := range template.Variables {
		subm, ok := submitted.Variables[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("variables.%s: cannot remove variable from template", name))
			continue
		}
		if tmpl.Secret != subm.Secret {
			errs = append(errs, fmt.Sprintf("variables.%s.secret: server-owned field cannot be changed", name))
		}
		if tmpl.Optional != subm.Optional {
			errs = append(errs, fmt.Sprintf("variables.%s.optional: server-owned field cannot be changed", name))
		}
	}
	for name := range submitted.Variables {
		if _, ok := template.Variables[name]; !ok {
			delete(submitted.Variables, name)
		}
	}

	return errs
}

// enforceEndpoints checks that server-owned endpoint fields (port, protocol) were not
// changed between template and submitted.  The expose sub-object is editable and is
// not checked here.
func enforceEndpoints(prefix string, tmpl, subm map[string]Endpoint) []string {
	var errs []string
	for name, te := range tmpl {
		se, ok := subm[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s.endpoints.%s: server-owned endpoint cannot be removed", prefix, name))
			continue
		}
		if te.Port != se.Port {
			errs = append(errs, fmt.Sprintf("%s.endpoints.%s.port: server-owned field cannot be changed", prefix, name))
		}
		if te.Protocol != se.Protocol {
			errs = append(errs, fmt.Sprintf("%s.endpoints.%s.protocol: server-owned field cannot be changed", prefix, name))
		}
	}
	for name := range subm {
		if _, ok := tmpl[name]; !ok {
			errs = append(errs, fmt.Sprintf("%s.endpoints.%s: cannot add endpoints not present in template", prefix, name))
		}
	}
	return errs
}
