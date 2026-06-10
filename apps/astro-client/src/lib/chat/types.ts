/** Sidebar / session list entry (messages loaded per thread from the server). */
export interface ChatSession {
  conversationId: string;
  deploymentId: string;
  title: string;
  updatedAt: string;
}
