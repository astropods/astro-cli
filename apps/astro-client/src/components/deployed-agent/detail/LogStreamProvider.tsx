import { createContext, useCallback, useContext, useEffect, useMemo, useReducer, useRef, type ReactNode } from "react";
import { useApiClient } from "@/lib/api-context";
import type { LogEntry } from "@/lib/log-utils";

export type LogStreamStatus = "idle" | "connecting" | "tailing" | "reconnecting";

interface LogStreamState {
  lines: LogEntry[];
  status: LogStreamStatus;
  error: string | undefined;
}

type LogStreamAction =
  | { type: "connecting" }
  | { type: "tailing" }
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

function isMessageEvent(e: Event): e is MessageEvent {
  return "data" in e;
}

const MAX_STREAM_LINES = 5000;
const initialState: LogStreamState = { lines: [], status: "idle", error: undefined };

function reducer(state: LogStreamState, action: LogStreamAction): LogStreamState {
  switch (action.type) {
    case "connecting":
      return { lines: [], status: "connecting", error: undefined };
    case "tailing":
      return { ...state, status: "tailing", error: undefined };
    case "message": {
      const lines = [...state.lines, action.line];
      return { ...state, lines: lines.length > MAX_STREAM_LINES ? lines.slice(-MAX_STREAM_LINES) : lines };
    }
    case "stream_error":
      return { ...state, error: action.message };
    case "reconnecting":
      return { lines: [], status: "reconnecting", error: undefined };
    case "reset":
      return initialState;
  }
}

const LogStreamContext = createContext<LogStreamContextValue | null>(null);

export function useLogStream(): LogStreamContextValue {
  const ctx = useContext(LogStreamContext);
  if (!ctx) throw new Error("useLogStream must be used within LogStreamProvider");
  return ctx;
}

export function LogStreamProvider({ children }: { children: ReactNode }) {
  const api = useApiClient();
  const [state, dispatch] = useReducer(reducer, initialState);
  const esRef = useRef<EventSource | null>(null);

  const stopStream = useCallback(() => {
    if (esRef.current) {
      esRef.current.onmessage = null;
      esRef.current.onerror = null;
      esRef.current.close();
      esRef.current = null;
    }
    dispatch({ type: "reset" });
  }, []);

  const startStream = useCallback((deploymentId: string, workloadName: string, container: string) => {
    if (esRef.current) {
      esRef.current.onmessage = null;
      esRef.current.onerror = null;
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
      dispatch({ type: "tailing" });
    });

    es.addEventListener("error", (e: Event) => {
      if (!isMessageEvent(e)) return;
      try {
        const parsed = JSON.parse(e.data) as { message?: string };
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
  }, [api]);

  const contextValue = useMemo(() => ({
    lines: state.lines,
    status: state.status,
    error: state.error,
    startStream,
    stopStream,
  }), [state.lines, state.status, state.error, startStream, stopStream]);

  useEffect(() => {
    return () => {
      esRef.current?.close();
      esRef.current = null;
    };
  }, []);

  return (
    <LogStreamContext.Provider value={contextValue}>
      {children}
    </LogStreamContext.Provider>
  );
}
