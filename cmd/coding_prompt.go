package cmd

import "strings"

func buildCodingPrompt(agentName, goal string) string {
	var sb strings.Builder
	sb.WriteString("You are helping implement an AI agent on the Astropods platform.\n\n")
	sb.WriteString("Astropods agents are defined by an `astropods.yml` file that connects:\n")
	sb.WriteString("- models (LLMs)\n")
	sb.WriteString("- knowledge stores\n")
	sb.WriteString("- tools (external actions/APIs)\n")
	sb.WriteString("- interfaces (how users interact with the agent)\n\n")
	sb.WriteString("The full spec reference is at https://astropods.com/schema/package.json.\n")
	sb.WriteString("The project's `AGENTS.md` explains the directory layout and conventions.\n\n")
	sb.WriteString("The agent name is **" + agentName + "**.")
	if goal != "" {
		sb.WriteString("\n" + wordWrap("Initial description (confirm before treating as final): "+goal+".", 80))
	}
	sb.WriteString("\n\n")
	sb.WriteString("Your job is to:\n")
	sb.WriteString("1. Inspect the project and identify:\n")
	sb.WriteString("   - where the agent logic lives (entrypoints, handlers, tool bindings)\n")
	sb.WriteString("   - how `astropods.yml` wires components together\n")
	sb.WriteString("2. Help design and implement the agent's behavior.\n\n")
	sb.WriteString("Before making any changes, gather requirements.\n\n")
	sb.WriteString("Ask me targeted questions to clarify:\n")
	sb.WriteString("- whether the initial description is accurate and complete\n")
	sb.WriteString("- expected inputs and outputs\n")
	sb.WriteString("- required tools or integrations\n")
	sb.WriteString("- whether the agent needs to ingest or query data from a knowledge base\n")
	sb.WriteString("- constraints (latency, cost, determinism, etc.)\n")
	sb.WriteString("- example use cases\n")
	sb.WriteString("- how success will be validated once the agent is implemented\n\n")
	sb.WriteString("After I respond:\n")
	sb.WriteString("- propose a concrete design\n")
	sb.WriteString("- map it to specific files in the repo\n")
	sb.WriteString("- then implement incrementally\n\n")
	sb.WriteString("Do not make assumptions about the agent's purpose without confirming them.")
	return sb.String()
}

func wordWrap(line string, width int) string {
	var out strings.Builder
	col := 0
	for i, word := range strings.Fields(line) {
		if i == 0 {
			out.WriteString(word)
			col = len(word)
		} else if col+1+len(word) <= width {
			out.WriteString(" " + word)
			col += 1 + len(word)
		} else {
			out.WriteString("\n" + word)
			col = len(word)
		}
	}
	return out.String()
}
