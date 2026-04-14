import { createContext, useContext, useEffect, useReducer, useRef, type ReactNode } from "react";
import { useApiClient } from "@/lib/api-context";
import type { LogEntry } from "@/lib/log-utils";

// ── Types ─────────────────────────────────────────────────────────────────────

export type LogStreamStatus = "idle" | "connecting" | "live" | "reconnecting";

interface LogStreamState {
  lines: LogEntry[];
  status: LogStreamStatus;
  error: string | undefined;
}

type LogStreamAction =
  | { type: "connecting" }
  | { type: "live" }
  | { type: "message"; line: LogEntry }
  | { type: "stream_error"; message: string }
  | { type: "reconnecting" }
  | { type: "reset" };

export interface LogStreamContextValue {
  lines: LogEntry[];
  status: LogStreamStatus;
  error: string | undefined;
  startStream: (deploymentId: string, workloadName: string, container: string) => void;
  stopStream: () => void;
}

// ── Constants ─────────────────────────────────────────────────────────────────

const MAX_STREAM_LINES = 5000;
const initialState: LogStreamState = { lines: [], status: "idle", error: undefined };

// ── Reducer ───────────────────────────────────────────────────────────────────

function reducer(state: LogStreamState, action: LogStreamAction): LogStreamState {
  switch (action.type) {
    case "connecting":
      return { lines: [], status: "connecting", error: undefined };
    case "live":
      return { ...state, status: "live", error: undefined };
    case "message": {
      const lines = [...state.lines, action.line];
      return { ...state, lines: lines.length > MAX_STREAM_LINES ? lines.slice(-MAX_STREAM_LINES) : lines };
    }
    case "stream_error":
      return { ...state, error: action.message };
    case "reconnecting":
      return { ...state, status: "reconnecting", error: undefined };
    case "reset":
      return initialState;
  }
}

// ── Context ───────────────────────────────────────────────────────────────────

const LogStreamContext = createContext<LogStreamContextValue | null>(null);

export function useLogStream(): LogStreamContextValue {
  const ctx = useContext(LogStreamContext);
  if (!ctx) throw new Error("useLogStream must be used within LogStreamProvider");
  return ctx;
}

// ── Provider ──────────────────────────────────────────────────────────────────

export function LogStreamProvider({ children }: { children: ReactNode }) {
  const api = useApiClient();
  const [state, dispatch] = useReducer(reducer, initialState);
  const esRef = useRef<EventSource | null>(null);

  function stopStream() {
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }
    dispatch({ type: "reset" });
  }

  function startStream(deploymentId: string, workloadName: string, container: string) {
    // Close any existing connection.
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }

    dispatch({ type: "connecting" });

    const url = api.getDeploymentLogsStreamUrl(deploymentId, workloadName, container);
    const es = new EventSource(url);
    esRef.current = es;

    // Track whether the stream has ever reached live so onerror can distinguish
    // "first connect failed" (show error) from "dropped after going live" (show reconnecting).
    let hasBeenLive = false;

    es.onmessage = (e: MessageEvent) => {
      try {
        const parsed = JSON.parse(e.data) as { timestamp: string; level: string; message: string };
        dispatch({ type: "message", line: { timestamp: parsed.timestamp, level: parsed.level || null, message: parsed.message } });
      } catch {
        // ignore malformed events
      }
    };

    es.addEventListener("ready", () => {
      hasBeenLive = true;
      dispatch({ type: "live" });
    });

    es.addEventListener("error", (e: Event) => {
      try {
        const parsed = JSON.parse((e as MessageEvent).data) as { message?: string };
        dispatch({ type: "stream_error", message: parsed.message ?? "Stream error" });
      } catch {
        // ignore parse errors on the error event
      }
    });

    es.onerror = () => {
      if (es.readyState === EventSource.CONNECTING) {
        // Browser is auto-retrying after the server closed the SSE stream.
        if (hasBeenLive) {
          dispatch({ type: "reconnecting" });
        } else {
          dispatch({ type: "stream_error", message: "Failed to connect to log stream" });
        }
      } else if (es.readyState === EventSource.CLOSED) {
        dispatch({ type: "reset" });
      }
    };
  }

  useEffect(() => {
    return () => {
      if (esRef.current) {
        esRef.current.close();
        esRef.current = null;
      }
    };
  }, []);

  return (
    <LogStreamContext.Provider value={{ lines: state.lines, status: state.status, error: state.error, startStream, stopStream }}>
      {children}
    </LogStreamContext.Provider>
  );
}
