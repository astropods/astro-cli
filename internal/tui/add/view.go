package add

import (
	"fmt"
	"strings"

	spec "github.com/postman/astro/packages/astro-spec"
)

func (m model) View() string {
	if m.quitting || m.done {
		return ""
	}

	var b strings.Builder

	// Title with domain and provider context
	switch m.domain {
	case "ingestion":
		b.WriteString(titleStyle.Render("  Adding ingestion pipeline"))
	case "provider":
		b.WriteString(titleStyle.Render(fmt.Sprintf("  Adding custom provider: %s", m.provider)))
	default:
		b.WriteString(titleStyle.Render(fmt.Sprintf("  Adding %s: %s", m.domain, m.provider)))
	}
	b.WriteString("\n\n")

	// Step indicator
	steps := m.steps()
	stepIndex := m.screenStep()
	for i, s := range steps {
		if i == stepIndex {
			b.WriteString(selectedStyle.Render("● " + s))
		} else if i < stepIndex {
			b.WriteString(dimStyle.Render("✓ " + s))
		} else {
			b.WriteString(dimStyle.Render("○ " + s))
		}
		if i < len(steps)-1 {
			b.WriteString(dimStyle.Render(" → "))
		}
	}
	b.WriteString("\n\n")

	switch m.screen {
	case screenName:
		b.WriteString(promptStyle.Render("  Name"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(fmt.Sprintf("  Reference name for this %s in astroai.yml.", m.domain)))
		b.WriteString("\n\n")
		b.WriteString("  " + m.nameInput.View())
		b.WriteString("\n\n")
		if m.err != "" {
			b.WriteString(errorStyle.Render("  " + m.err))
			b.WriteString("\n")
		}
		b.WriteString(dimStyle.Render("  enter confirm"))

	case screenOllamaModel:
		b.WriteString(promptStyle.Render("  Ollama model"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Choose the model to run locally."))
		b.WriteString("\n\n")
		b.WriteString(m.renderRadioList(ollamaModelOptions()))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter confirm"))

	case screenPersistent:
		b.WriteString(promptStyle.Render("  Persistent storage?"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Persist data across agent restarts."))
		b.WriteString("\n\n")
		b.WriteString(m.renderRadioList(persistentOptions()))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter confirm"))

	case screenImage:
		b.WriteString(promptStyle.Render("  Container image"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Full image reference for the ingestion container."))
		b.WriteString("\n\n")
		b.WriteString("  " + m.imageInput.View())
		b.WriteString("\n\n")
		if m.err != "" {
			b.WriteString(errorStyle.Render("  " + m.err))
			b.WriteString("\n")
		}
		b.WriteString(dimStyle.Render("  enter confirm"))

	case screenTrigger:
		b.WriteString(promptStyle.Render("  Trigger type"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  When should this ingestion pipeline run?"))
		b.WriteString("\n\n")
		b.WriteString(m.renderRadioList(triggerOptions()))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter confirm"))

	case screenScope:
		b.WriteString(promptStyle.Render("  Scope"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Which sections can reference this provider?"))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptionList(scopeOptions()))
		if m.err != "" {
			b.WriteString(errorStyle.Render("  " + m.err))
			b.WriteString("\n")
		}
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenVarName:
		varCount := len(m.vars) + 1
		b.WriteString(promptStyle.Render(fmt.Sprintf("  Variable #%d suffix", varCount)))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(fmt.Sprintf("  Suffix only — the full env key will be %s_{SUFFIX} (e.g. API_KEY → %s_API_KEY).", spec.SanitizeEnvName(m.provider), spec.SanitizeEnvName(m.provider))))
		b.WriteString("\n\n")
		b.WriteString("  " + m.varNameInput.View())
		b.WriteString("\n\n")
		if m.err != "" {
			b.WriteString(errorStyle.Render("  " + m.err))
			b.WriteString("\n")
		}
		b.WriteString(dimStyle.Render("  enter confirm"))

	case screenVarSecret:
		b.WriteString(promptStyle.Render(fmt.Sprintf("  Is %q a secret?", m.currentVarName)))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Secrets are stored securely and never logged."))
		b.WriteString("\n\n")
		b.WriteString(m.renderRadioList(secretOptions()))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter confirm"))

	case screenAddAnother:
		b.WriteString(promptStyle.Render("  Add another variable?"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(fmt.Sprintf("  %d variable(s) added so far.", len(m.vars))))
		b.WriteString("\n\n")
		b.WriteString(m.renderRadioList(addAnotherOptions()))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter confirm"))

	case screenConfirm:
		b.WriteString(m.renderSummary())
		b.WriteString("\n")
		b.WriteString(promptStyle.Render("  Add this entry? "))
		b.WriteString(dimStyle.Render("(Y/n)"))
	}

	b.WriteString("\n")
	return b.String()
}

// steps returns the ordered list of step names for the current domain/provider.
func (m model) steps() []string {
	switch m.domain {
	case "model":
		if m.provider == "ollama" {
			return []string{"Name", "Model", "Confirm"}
		}
		return []string{"Name", "Confirm"}
	case "knowledge":
		return []string{"Name", "Persistent", "Confirm"}
	case "tool":
		return []string{"Name", "Confirm"}
	case "ingestion":
		return []string{"Name", "Image", "Trigger", "Confirm"}
	case "provider":
		return []string{"Scope", "Variables", "Confirm"}
	}
	return []string{"Name", "Confirm"}
}

// screenStep maps the current screen to the step indicator index.
func (m model) screenStep() int {
	switch m.screen {
	case screenName:
		return 0
	case screenOllamaModel, screenPersistent, screenImage:
		return 1
	case screenTrigger:
		return 2
	case screenScope:
		return 0
	case screenVarName, screenVarSecret, screenAddAnother:
		return 1
	case screenConfirm:
		return len(m.steps()) - 1
	}
	return 0
}

func (m model) renderOptionList(opts []option) string {
	var b strings.Builder
	for i, opt := range opts {
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("❯ ")
		}
		marker := "○"
		if m.selected[i] {
			marker = selectedStyle.Render("●")
		}
		b.WriteString("    " + cursor + marker + " " + opt.label + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m model) renderRadioList(opts []option) string {
	var b strings.Builder
	for i, opt := range opts {
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("❯ ")
		}
		marker := "○"
		if i == m.cursor {
			marker = selectedStyle.Render("●")
		}
		b.WriteString("    " + cursor + marker + " " + opt.label + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m model) renderSummary() string {
	var b strings.Builder
	b.WriteString(promptStyle.Render("  Summary"))
	b.WriteString("\n\n")

	row := func(label, value string) {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %-12s", label)))
		b.WriteString(value + "\n")
	}

	row("Domain", m.domain)
	if m.domain != "ingestion" && m.domain != "provider" {
		row("Provider", m.provider)
	}
	row("Name", m.name)

	switch m.domain {
	case "model":
		if m.provider == "ollama" {
			ollamaModel := m.ollamaModel
			if ollamaModel == "" {
				ollamaModel = ollamaModelOptions()[0].value
			}
			row("Model", ollamaModel)
		}
	case "knowledge":
		if m.persistent {
			row("Persistent", "yes")
		} else {
			row("Persistent", "no")
		}
	case "ingestion":
		row("Image", m.imageInput.Value())
		trigger := m.triggerType
		if trigger == "" {
			trigger = "manual"
		}
		row("Trigger", trigger)
	case "provider":
		row("Scope", strings.Join(m.scopeToSlice(), ", "))
		prefix := spec.SanitizeEnvName(m.provider)
		for i, v := range m.vars {
			label := fmt.Sprintf("Var #%d", i+1)
			val := prefix + "_" + v.name
			if v.secret {
				val += " (secret)"
			}
			row(label, val)
		}
	}

	return b.String()
}
