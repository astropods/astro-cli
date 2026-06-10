import { createContext, useContext } from "react";

export type DeploymentChatViewportState = {
  streamingMessageId: string | null;
  conversationId: string | null;
  historyLoading: boolean;
  isStreaming: boolean;
  streamError: string | null;
};

const defaultViewportState: DeploymentChatViewportState = {
  streamingMessageId: null,
  conversationId: null,
  historyLoading: false,
  isStreaming: false,
  streamError: null,
};

export const DeploymentChatStreamingContext =
  createContext<DeploymentChatViewportState>(defaultViewportState);

export function useDeploymentChatStreamingMessageId(): string | null {
  return useContext(DeploymentChatStreamingContext).streamingMessageId;
}

export function useDeploymentChatViewport(): DeploymentChatViewportState {
  return useContext(DeploymentChatStreamingContext);
}
