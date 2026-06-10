import { createMathPlugin } from "@streamdown/math";
import { mermaid } from "@streamdown/mermaid";

// Ported from astropods/playground#28 (memory-box chat model). Math + diagrams
// are opt-in Streamdown plugins. Single-dollar inline math matches what agents
// emit; mermaid uses the pre-configured plugin.
const mathPlugin = createMathPlugin({ singleDollarTextMath: true });

export const deploymentChatStreamdownPlugins = {
  math: mathPlugin,
  mermaid,
} as const;

// Fullscreen on mermaid/table panels misbehaved in the playground embed; disable
// it so toolbars stay [download, copy] like code blocks.
export const deploymentChatStreamdownControls = {
  mermaid: { fullscreen: false },
  table: { fullscreen: false },
} as const;
