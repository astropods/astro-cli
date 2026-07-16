## Summary

The chat experience now presents agent configuration details with a denser, more consistent inspector layout. System prompts stay contained, tools are easier to scan and expand, and adjacent chat controls use the same visual language as the Details panel.

## Design

The Config tab treats the system prompt as a compact readable block: long prompts wrap within the inspector, expanded prompts scroll internally, and collapsing returns the prompt to the top. The system prompt and expanded tool rows share a soft filled surface so expanded content has a clear reading area without reintroducing heavy card borders.

Tool configuration moved from a static descriptive list to a row-based disclosure pattern. Tool names remain compact, descriptions are revealed on demand, and expanded descriptions are rendered outside the row button so users can select and copy text.

Inspector section headers now share one treatment across Usage, Recent traces, System prompt, and Tools: icon, foreground label, and muted mono count where relevant. The chat history dropdown uses the same header treatment, and the single-agent selector footer uses a leading plus icon for "Deploy more agents" to make the action distinct from navigation links.

Focused regression tests cover long prompt containment, expanded prompt scrolling and reset, tool disclosure behavior, selectable tool descriptions, chat history header styling, and the agent selector deploy-more affordance.

## Migration

No migration is required.
