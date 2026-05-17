export interface IconEntry {
  id: string;
  brand?: string;
}

export interface IconsResponse {
  icons: IconEntry[];
}

export interface SourceCandidate {
  id: string;
  brand?: string;
  label: string;
  lightSvg: string;
  darkSvg: string;
  sourceUrl?: string;
  notes?: string;
}

export type ChatRole = "user" | "assistant";

export interface ChatMessage {
  role: ChatRole;
  text: string;
  candidates?: SourceCandidate[];
}

export interface AssistantTurn {
  text: string;
  candidates?: SourceCandidate[];
}

export interface SourceResponse {
  sessionId: string;
  turn: AssistantTurn;
}
