import type { FC } from "react";
import { Streamdown, type BundledTheme } from "streamdown";
import "katex/dist/katex.min.css";
import { useAuiState } from "@assistant-ui/react";
import { useDeploymentChatStreamingMessageId } from "@/components/chat/deployment-chat-streaming-context";
import { proseClasses } from "@/components/StyledMarkdown";
import {
  deploymentChatStreamdownControls,
  deploymentChatStreamdownPlugins,
} from "@/lib/chat/streamdown";

const SHIKI_THEME: [BundledTheme, BundledTheme] = [
  "github-light",
  "github-dark",
];

/**
 * Streaming markdown via Streamdown (Vercel's react-markdown drop-in for AI
 * streaming). Config matches astropods/playground#28: block memoization,
 * remend for open delimiters while streaming, math/mermaid plugins, and
 * selective controls (no fullscreen on diagram/table panels).
 *
 * Rendering decisions (each fixes a concrete streaming artifact):
 * - mode and parseIncompleteMarkdown follow isStreaming only; remend must not
 *   run on completed text (latched streaming mode left stray * delimiters).
 * - Animation is a fast per-word fadeIn with no stagger.
 *   The [data-sd-animate] rule in index.css provides the keyframes.
 * - Image alignment uses proseClasses overrides from the same PR (vertical-align
 *   middle, zero margin inside paragraphs). No custom mermaid panel CSS —
 *   playground#28 dropped those after they clipped diagrams.
 */
const ANIMATE_OPTIONS = {
  animation: "fadeIn",
  duration: 120,
  sep: "word",
  stagger: 0,
} as const;

export const DeploymentChatText: FC = () => {
  const messageId = useAuiState((s) => s.message.id);
  const streamingMessageId = useDeploymentChatStreamingMessageId();
  const text = useAuiState((s) => (s.part.type === "text" ? s.part.text : ""));

  const isStreaming =
    streamingMessageId !== null && messageId === streamingMessageId;

  if (!text.trim()) return null;

  return (
    <Streamdown
      mode={isStreaming ? "streaming" : "static"}
      parseIncompleteMarkdown={isStreaming}
      isAnimating={isStreaming}
      animated={ANIMATE_OPTIONS}
      plugins={deploymentChatStreamdownPlugins}
      controls={deploymentChatStreamdownControls}
      shikiTheme={SHIKI_THEME}
      className={proseClasses}
    >
      {text}
    </Streamdown>
  );
};
