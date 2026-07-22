import { createContext, useContext } from "react";

export type DeploymentChatViewportState = {
  streamingMessageId: string | null;
  conversationId: string | null;
  historyLoading: boolean;
  isStreaming: boolean;
  streamError: string | null;
  hasMoreHistory: boolean;
  loadOlderMessages: () => Promise<void>;
  // Composer shows its upload affordance only when the agent supports files
  // (sidecar has storage AND the agent declared it consumes attachments).
  // Defaults false so the button stays hidden until config confirms otherwise.
  filesEnabled: boolean;
};

const defaultViewportState: DeploymentChatViewportState = {
  streamingMessageId: null,
  conversationId: null,
  historyLoading: false,
  isStreaming: false,
  streamError: null,
  hasMoreHistory: false,
  loadOlderMessages: async () => {},
  filesEnabled: false,
};

export const DeploymentChatStreamingContext =
  createContext<DeploymentChatViewportState>(defaultViewportState);

export function useDeploymentChatStreamingMessageId(): string | null {
  return useContext(DeploymentChatStreamingContext).streamingMessageId;
}

export function useDeploymentChatViewport(): DeploymentChatViewportState {
  return useContext(DeploymentChatStreamingContext);
}
