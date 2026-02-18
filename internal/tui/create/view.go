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
	steps := []string{"Description", "Interfaces", "Infrastructure", "Ingestion", "Confirm"}
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
		b.WriteString(m.renderMultiSelectOptions(interfaceOptions()))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenInfrastructure:
		b.WriteString(promptStyle.Render("  Infrastructure"))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Models, knowledge stores, and tools for your agent."))
		b.WriteString("\n\n")
		b.WriteString(m.renderMultiSelectOptions(infrastructureOptions()))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · space toggle · enter confirm"))

	case screenModelName:
		b.WriteString(promptStyle.Render(fmt.Sprintf("  Model name (%s)", m.config.ModelProvider)))
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  e.g. llama3, mistral, codellama"))
		b.WriteString("\n\n")
		b.WriteString("  " + m.modelInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  enter confirm · leave empty to skip"))

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
		b.WriteString(m.renderOptions(ingestionOptions()))
		b.WriteString(dimStyle.Render("  ↑/↓ navigate · enter select"))

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
	case screenInfrastructure, screenModelName, screenIntegrationKey:
		return 2
	case screenIngestion:
		return 3
	case screenConfirm:
		return 4
	}
	return 0
}

func (m model) renderOptions(opts []option) string {
	var b strings.Builder
	for i, opt := range opts {
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("  ❯ " + opt.label))
		} else {
			b.WriteString("    " + opt.label)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m model) renderMultiSelectOptions(opts []option) string {
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
		checkbox := "○"
		if m.selected[i] {
			checkbox = selectedStyle.Render("●")
		}
		b.WriteString("    " + cursor + checkbox + " " + opt.label + "\n")
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

	row("Ingestion", m.config.Ingestion)

	return b.String()
}
