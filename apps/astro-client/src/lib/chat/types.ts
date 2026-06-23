import type { AgentDeploymentSummary } from "@/lib/api";

/** Sidebar / session list entry (messages loaded per thread from the server). */
export interface ChatSession {
  conversationId: string;
  deploymentId: string;
  title: string;
  updatedAt: string;
  assistantStreaming?: boolean;
}

/**
 * One chat-eligible agent. Cross-account: each entry carries the account it is
 * deployed to (the page lists every agent the user can chat with, regardless of
 * org), so avatars and identity resolve per row.
 */
export interface ChatAgent {
  deployment: AgentDeploymentSummary;
  account: string;
}
