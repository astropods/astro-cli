package create

import (
	"fmt"
	"strings"
)

func (m model) View() string {
	if m.quitting || m.done {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("  Creating agent: %s", m.name)))
	b.WriteString("\n\n")

	// Step indicator
	steps := []string{"Description", "Interfaces", "Model", "Knowledge", "Tools", "Ingestion", "Confirm"}
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
	case screenDescription:
		b.WriteString(promptStyle.Render("  Description"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  A short summary of what your agent does."))
		b.WriteString("\n\n")
		b.WriteString("  " + m.descInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  enter confirm"))

	case screenInterface:
		b.WriteString(promptStyle.Render("  Interfaces"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  How users interact with your agent."))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptionList(interfaceOptions(), false))
		if m.err != "" {
			b.WriteString(errorStyle.Render("  " + m.err))
			b.WriteString("\n")
		}
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenModel:
		b.WriteString(promptStyle.Render("  Model"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  LLM provider(s) for your agent."))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptionList(modelOptions(), false))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenOllamaModel:
		b.WriteString(promptStyle.Render("  Ollama model"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Choose one model."))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptionList(ollamaModelOptions(), false))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenKnowledge:
		b.WriteString(promptStyle.Render("  Knowledge"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Vector stores, caches, and graph DBs for your agent."))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptionList(knowledgeOptions(), false))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenIntegrations:
		b.WriteString(promptStyle.Render("  Tools"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Tool integrations (e.g. GitHub) for your agent."))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptionList(toolsOptions(), false))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenIntegrationKey:
		name := m.pendingKeys[m.keyIndex]
		envVar := integrationKeyEnvVar[name]
		label := integrationKeyLabel[name]
		b.WriteString(promptStyle.Render(fmt.Sprintf("  %s API key", label)))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render(fmt.Sprintf("  Will be saved as %s in .env — you can also set it later.", envVar)))
		b.WriteString("\n\n")
		b.WriteString("  " + m.keyInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  enter confirm · leave empty to skip"))

	case screenIngestion:
		b.WriteString(promptStyle.Render("  Data ingestion"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  How to trigger your data pipeline that populates knowledge stores."))
		b.WriteString("\n\n")
		b.WriteString(m.renderOptionList(ingestionOptions(), false))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenConfirm:
		b.WriteString(m.renderSummary())
		b.WriteString("\n")
		b.WriteString(promptStyle.Render("  Create this agent? "))
		b.WriteString(dimStyle.Render("(Y/n)"))
	}

	b.WriteString("\n")
	return b.String()
}

// screenStep maps the current screen to the step index for the progress indicator.
func (m model) screenStep() int {
	switch m.screen {
	case screenDescription:
		return 0
	case screenInterface:
		return 1
	case screenModel, screenOllamaModel:
		return 2
	case screenKnowledge:
		return 3
	case screenIntegrations, screenIntegrationKey:
		return 4
	case screenIngestion:
		return 5
	case screenConfirm:
		return 6
	}
	return 0
}

// renderOptionList renders a list of options. When radio is true (e.g. Ollama model picker),
// ● only on the cursor row (single-select). When radio is false (checkbox/multi-select),
// ● on each selected row, ○ elsewhere.
func (m model) renderOptionList(opts []option, radio bool) string {
	var b strings.Builder
	for i, opt := range opts {
		if opt.isHeader {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString("  " + dimStyle.Render(opt.label) + "\n")
			continue
		}
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("❯ ")
		}
		filled := false
		if radio {
			filled = i == m.cursor
		} else {
			filled = m.selected[i]
		}
		marker := "○"
		if filled {
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
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %-14s", label)))
		b.WriteString(value + "\n")
	}

	row("Name", m.config.Name)
	row("Description", m.config.Description)

	if len(m.config.Interfaces) > 0 {
		row("Interfaces", strings.Join(m.config.Interfaces, ", "))
	} else {
		row("Interfaces", "web")
	}

	var infra []string
	if m.config.ModelProvider != "" {
		modelDisplay := m.config.ModelProvider
		if m.config.Model != "" {
			modelDisplay += "/" + m.config.Model
		}
		infra = append(infra, modelDisplay)
	}
	infra = append(infra, m.config.Knowledge...)
	infra = append(infra, m.config.Integrations...)
	if len(infra) > 0 {
		row("Infrastructure", strings.Join(infra, ", "))
	} else {
		row("Infrastructure", dimStyle.Render("none"))
	}

	if len(m.config.Ingestions) > 0 {
		row("Ingestion", strings.Join(m.config.Ingestions, ", "))
	} else {
		row("Ingestion", dimStyle.Render("none"))
	}

	return b.String()
}
