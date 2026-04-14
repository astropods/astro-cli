import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { type ReactNode } from "react";
import { LogStreamProvider, useLogStream } from "./LogStreamProvider";

class MockEventSource {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  static instances: MockEventSource[] = [];

  url: string;
  readyState = MockEventSource.CONNECTING;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  private listeners = new Map<string, Array<(e: Event) => void>>();
  closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (e: Event) => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, []);
    this.listeners.get(type)!.push(listener);
  }

  emitMessage(data: string) {
    this.onmessage?.(new MessageEvent("message", { data }));
  }

  emitEvent(type: string, data?: string) {
    const event = data != null ? new MessageEvent(type, { data }) : new Event(type);
    this.listeners.get(type)?.forEach((l) => l(event));
  }

  emitError(readyState: number) {
    this.readyState = readyState;
    this.onerror?.(new Event("error"));
  }

  close() {
    this.closed = true;
    this.readyState = MockEventSource.CLOSED;
  }

  static reset() {
    MockEventSource.instances = [];
  }

  static latest(): MockEventSource {
    const last = MockEventSource.instances.at(-1);
    if (!last) throw new Error("No MockEventSource instances");
    return last;
  }
}

function wrapper({ children }: { children: ReactNode }) {
  return <LogStreamProvider>{children}</LogStreamProvider>;
}

beforeEach(() => {
  MockEventSource.reset();
  vi.stubGlobal("EventSource", MockEventSource);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("LogStreamProvider", () => {
  it("starts in idle state", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });
    expect(result.current.status).toBe("idle");
    expect(result.current.lines).toHaveLength(0);
    expect(result.current.error).toBeUndefined();
  });

  it("startStream opens an EventSource and sets status to connecting", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "agent-workload", "app"));

    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.latest().url).toContain("dep-1");
    expect(result.current.status).toBe("connecting");
    expect(result.current.lines).toHaveLength(0);
  });

  it("event: ready sets status to live", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => MockEventSource.latest().emitEvent("ready"));

    expect(result.current.status).toBe("tailing");
    expect(result.current.error).toBeUndefined();
  });

  it("onmessage appends parsed log lines", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => MockEventSource.latest().emitEvent("ready"));
    act(() => MockEventSource.latest().emitMessage(
      JSON.stringify({ timestamp: "2026-01-01T00:00:01Z", level: "info", message: "hello" }),
    ));
    act(() => MockEventSource.latest().emitMessage(
      JSON.stringify({ timestamp: "2026-01-01T00:00:02Z", level: "error", message: "world" }),
    ));

    expect(result.current.lines).toHaveLength(2);
    expect(result.current.lines[0].message).toBe("hello");
    expect(result.current.lines[1].level).toBe("error");
  });

  it("malformed messages are ignored", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => MockEventSource.latest().emitMessage("not-json"));

    expect(result.current.lines).toHaveLength(0);
  });

  it("server error event sets error string", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => MockEventSource.latest().emitEvent("error", JSON.stringify({ message: "failed to connect to log stream" })));

    expect(result.current.error).toBe("failed to connect to log stream");
  });

  it("onerror with CONNECTING before going live sets error", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => MockEventSource.latest().emitError(MockEventSource.CONNECTING));

    expect(result.current.status).toBe("connecting");
    expect(result.current.error).toBe("Failed to connect to log stream");
  });

  it("onerror with CONNECTING after going live sets status to reconnecting", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => MockEventSource.latest().emitEvent("ready"));
    act(() => MockEventSource.latest().emitError(MockEventSource.CONNECTING));

    expect(result.current.status).toBe("reconnecting");
    expect(result.current.error).toBeUndefined();
  });

  it("onerror with CLOSED resets to idle", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => MockEventSource.latest().emitError(MockEventSource.CLOSED));

    expect(result.current.status).toBe("idle");
  });

  it("stopStream closes the EventSource and resets state", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => MockEventSource.latest().emitEvent("ready"));

    const es = MockEventSource.latest();
    act(() => result.current.stopStream());

    expect(es.closed).toBe(true);
    expect(result.current.status).toBe("idle");
    expect(result.current.lines).toHaveLength(0);
  });

  it("startStream while already streaming closes the old connection", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    const first = MockEventSource.latest();

    act(() => result.current.startStream("dep-1", "wl", "sidecar"));

    expect(first.closed).toBe(true);
    expect(MockEventSource.instances).toHaveLength(2);
    expect(MockEventSource.latest().url).toContain("sidecar");
  });

  it("lines are capped at 5000", () => {
    const { result } = renderHook(() => useLogStream(), { wrapper });

    act(() => result.current.startStream("dep-1", "wl", "app"));
    act(() => {
      for (let i = 0; i < 5002; i++) {
        MockEventSource.latest().emitMessage(
          JSON.stringify({ timestamp: "", level: "info", message: `line ${i}` }),
        );
      }
    });

    expect(result.current.lines).toHaveLength(5000);
    expect(result.current.lines[0].message).toBe("line 2");
  });

  it("throws when used outside provider", () => {
    expect(() => renderHook(() => useLogStream())).toThrow("useLogStream must be used within LogStreamProvider");
  });
});
