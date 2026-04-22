import { useState, useCallback, useEffect, useRef } from "react";
import { LogViewer, type LogTimeRange } from "@/components/LogViewer";
import { useKnowledgeLogs } from "@/api/queries/knowledge";
import { useApiClient } from "@/lib/api-context";
import type { LogEntry } from "@/lib/log-utils";

const MAX_TAIL_LINES = 5000;

function useKnowledgeLogStream(account: string, storeName: string) {
  const api = useApiClient();
  const [lines, setLines] = useState<LogEntry[]>([]);
  const [status, setStatus] = useState<"idle" | "connecting" | "tailing" | "reconnecting">("idle");
  const [error, setError] = useState<string>();
  const esRef = useRef<EventSource | null>(null);

  const stop = useCallback(() => {
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }
    setStatus("idle");
    setLines([]);
    setError(undefined);
  }, []);

  const start = useCallback(() => {
    stop();
    setStatus("connecting");

    const url = api.getKnowledgeLogsStreamUrl(account, storeName);
    const es = new EventSource(url);
    esRef.current = es;
    let hasBeenLive = false;

    es.onmessage = (e: MessageEvent) => {
      try {
        const parsed = JSON.parse(e.data) as { timestamp: string; level: string; message: string };
        setLines((prev) => {
          const next = [...prev, { timestamp: parsed.timestamp, level: parsed.level || null, message: parsed.message }];
          return next.length > MAX_TAIL_LINES ? next.slice(-MAX_TAIL_LINES) : next;
        });
      } catch { /* ignore */ }
    };

    es.addEventListener("ready", () => {
      hasBeenLive = true;
      setStatus("tailing");
    });

    es.addEventListener("error", (e: Event) => {
      if ("data" in e) {
        try {
          const parsed = JSON.parse((e as MessageEvent).data) as { message?: string };
          setError(parsed.message ?? "Stream error");
        } catch { /* ignore */ }
      }
    });

    es.onerror = () => {
      if (es.readyState === EventSource.CONNECTING) {
        if (hasBeenLive) setStatus("reconnecting");
        else setError("Failed to connect to log stream");
      } else if (es.readyState === EventSource.CLOSED) {
        setStatus("idle");
      }
    };
  }, [api, account, storeName, stop]);

  useEffect(() => {
    return () => { esRef.current?.close(); esRef.current = null; };
  }, []);

  return { lines, status, error, start, stop };
}

export function LogsTab({ account, storeName }: { account: string; storeName: string }) {
  const [timeRange, setTimeRange] = useState<LogTimeRange>("1h");
  const [tailing, setTailing] = useState(false);
  const { data: historyLogs, isLoading, isError } = useKnowledgeLogs(account, storeName, timeRange, { enabled: !tailing });
  const stream = useKnowledgeLogStream(account, storeName);

  const handleTailToggle = useCallback(() => {
    if (tailing) {
      stream.stop();
      setTailing(false);
    } else {
      stream.start();
      setTailing(true);
    }
  }, [tailing, stream]);

  const logs = tailing ? stream.lines : (historyLogs ?? []);
  const loading = tailing ? stream.status === "connecting" : isLoading;
  const errorMsg = tailing ? stream.error : (isError ? "Failed to load logs" : undefined);

  return (
    <div className="h-[600px]">
      <LogViewer
        logs={logs}
        isLoading={loading}
        timeRange={timeRange}
        onTimeRangeChange={setTimeRange}
        error={errorMsg}
        isTailing={tailing && stream.status === "tailing"}
        isReconnecting={stream.status === "reconnecting"}
        onTailToggle={handleTailToggle}
      />
    </div>
  );
}
