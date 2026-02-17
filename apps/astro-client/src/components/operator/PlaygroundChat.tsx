import { useState, useRef, useEffect, useCallback } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  Send,
  Loader2,
  MessageSquare,
  ChevronDown,
  Wrench,
  Check,
  RotateCcw,
} from "lucide-react";
import type { AgentDeployment } from "../../lib/api";

type Step = {
  id: string;
  name: string;
  status: "running" | "completed";
};

type Message = {
  id: string;
  role: "user" | "assistant";
  content: string;
  steps?: Step[];
  reasoning?: string;
  isStreaming?: boolean;
};

interface PlaygroundChatProps {
  deployments: AgentDeployment[];
}

function getMessagingWebUrl(deployment: AgentDeployment): string | null {
  const ep = deployment.external_urls?.find((u) => u.name === "messaging-web");
  return ep?.url ?? null;
}

export function PlaygroundChat({ deployments }: PlaygroundChatProps) {
  const deploymentsWithUrl = deployments
    .map((d) => ({ deployment: d, url: getMessagingWebUrl(d) }))
    .filter((d): d is { deployment: AgentDeployment; url: string } => d.url !== null);

  const [selectedIndex, setSelectedIndex] = useState(0);
  const [messages, setMessages] = useState<Message[]>([]);
  const [conversationId, setConversationId] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const baseUrl = deploymentsWithUrl[selectedIndex]?.url;

  // Auto-scroll to bottom
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  // Focus input on mount
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  // Cleanup EventSource on unmount
  useEffect(() => {
    return () => {
      eventSourceRef.current?.close();
    };
  }, []);

  const resetConversation = useCallback(() => {
    eventSourceRef.current?.close();
    eventSourceRef.current = null;
    setMessages([]);
    setConversationId(null);
    setIsLoading(false);
    setInput("");
  }, []);

  const handleSelectDeployment = (index: number) => {
    setSelectedIndex(index);
    setDropdownOpen(false);
    resetConversation();
  };

  const setupEventSource = useCallback(
    (convId: string, assistantMessageId: string) => {
      eventSourceRef.current?.close();

      const es = new EventSource(`${baseUrl}/api/conversations/${convId}/stream`);
      eventSourceRef.current = es;

      const handleEvent = (event: MessageEvent) => {
        try {
          const data = JSON.parse(event.data);

          switch (data.type) {
            case "chunk":
              setMessages((prev) =>
                prev.map((msg) =>
                  msg.id === assistantMessageId
                    ? { ...msg, content: msg.content + (data.content || "") }
                    : msg
                )
              );
              break;

            case "step-start":
              setMessages((prev) =>
                prev.map((msg) =>
                  msg.id === assistantMessageId
                    ? {
                        ...msg,
                        steps: [
                          ...(msg.steps || []),
                          { id: data.step_id, name: data.name, status: "running" as const },
                        ],
                      }
                    : msg
                )
              );
              break;

            case "step-end":
              setMessages((prev) =>
                prev.map((msg) =>
                  msg.id === assistantMessageId
                    ? {
                        ...msg,
                        steps: msg.steps?.map((s) =>
                          s.id === data.step_id ? { ...s, status: "completed" as const } : s
                        ),
                      }
                    : msg
                )
              );
              break;

            case "reasoning-delta":
              setMessages((prev) =>
                prev.map((msg) =>
                  msg.id === assistantMessageId
                    ? { ...msg, reasoning: (msg.reasoning || "") + (data.content || "") }
                    : msg
                )
              );
              break;

            case "finish":
              setMessages((prev) =>
                prev.map((msg) =>
                  msg.id === assistantMessageId ? { ...msg, isStreaming: false } : msg
                )
              );
              setIsLoading(false);
              break;

            case "error":
              setMessages((prev) =>
                prev.map((msg) =>
                  msg.id === assistantMessageId
                    ? {
                        ...msg,
                        content: `Error: ${data.message || "Unknown error"}`,
                        isStreaming: false,
                      }
                    : msg
                )
              );
              setIsLoading(false);
              break;
          }
        } catch {
          // Skip invalid JSON
        }
      };

      es.addEventListener("chunk", handleEvent);
      es.addEventListener("step-start", handleEvent);
      es.addEventListener("step-end", handleEvent);
      es.addEventListener("reasoning-delta", handleEvent);
      es.addEventListener("finish", handleEvent);
      es.addEventListener("error", handleEvent);
      es.addEventListener("connected", handleEvent);
      es.onmessage = handleEvent;

      es.onerror = () => {
        setMessages((prev) =>
          prev.map((msg) =>
            msg.id === assistantMessageId ? { ...msg, isStreaming: false } : msg
          )
        );
        setIsLoading(false);
      };
    },
    [baseUrl]
  );

  const sendMessage = async () => {
    const trimmed = input.trim();
    if (!trimmed || isLoading || !baseUrl) return;

    const userMessage: Message = {
      id: crypto.randomUUID(),
      role: "user",
      content: trimmed,
    };

    const assistantMessage: Message = {
      id: crypto.randomUUID(),
      role: "assistant",
      content: "",
      isStreaming: true,
    };

    setMessages((prev) => [...prev, userMessage, assistantMessage]);
    setInput("");
    setIsLoading(true);

    try {
      let convId = conversationId;

      if (!convId) {
        const res = await fetch(`${baseUrl}/api/conversations`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({}),
        });
        if (!res.ok) throw new Error("Failed to create conversation");
        const data = await res.json();
        convId = data.conversation_id;
        setConversationId(convId);
      }

      setupEventSource(convId!, assistantMessage.id);

      const res = await fetch(`${baseUrl}/api/conversations/${convId}/messages`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: trimmed }),
      });

      if (!res.ok) throw new Error("Failed to send message");
    } catch (err) {
      setMessages((prev) =>
        prev.map((msg) =>
          msg.id === assistantMessage.id
            ? {
                ...msg,
                content: `Error: ${err instanceof Error ? err.message : "Failed to connect"}`,
                isStreaming: false,
              }
            : msg
        )
      );
      setIsLoading(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  // Empty state: no deployments with messaging-web
  if (deploymentsWithUrl.length === 0) {
    return null;
  }

  const selected = deploymentsWithUrl[selectedIndex];

  return (
    <aside className="hidden lg:flex w-[400px] shrink-0 flex-col border-l border-border bg-muted/50 h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
        <div className="flex items-center gap-2">
          <MessageSquare className="size-4 text-muted-foreground" />
          {deploymentsWithUrl.length > 1 ? (
            <div className="relative">
              <button
                onClick={() => setDropdownOpen(!dropdownOpen)}
                className="flex items-center gap-1.5 px-2.5 py-1.5 text-sm border border-border bg-background hover:bg-accent cursor-pointer rounded-md"
              >
                <span className="font-mono text-xs">{selected.deployment.build_id.slice(0, 8)}</span>
                <ChevronDown size={14} />
              </button>
              {dropdownOpen && (
                <div className="absolute top-full left-0 mt-1 bg-background border border-border shadow-lg z-10 min-w-[200px] rounded-md">
                  {deploymentsWithUrl.map((d, i) => (
                    <button
                      key={d.deployment.build_id}
                      onClick={() => handleSelectDeployment(i)}
                      className={`w-full text-left px-3 py-2 text-sm hover:bg-accent cursor-pointer border-none bg-transparent ${
                        i === selectedIndex ? "bg-accent font-medium" : ""
                      }`}
                    >
                      <span className="font-mono text-xs">{d.deployment.build_id.slice(0, 8)}</span>
                      <span className="text-muted-foreground ml-2">
                        {d.deployment.status === "running" ? "Running" : d.deployment.status}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          ) : (
            <div>
              <p className="text-sm font-medium leading-tight">Test Agent</p>
              <p className="text-xs text-muted-foreground">
                Build {selected.deployment.build_id.slice(0, 8)}
              </p>
            </div>
          )}
        </div>
        {messages.length > 0 && (
          <button
            onClick={resetConversation}
            className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs text-muted-foreground hover:text-foreground border border-border bg-background hover:bg-accent cursor-pointer rounded-md"
          >
            <RotateCcw size={12} />
            New chat
          </button>
        )}
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full gap-3">
            <div className="flex size-14 items-center justify-center rounded-xl bg-primary/10">
              <MessageSquare className="size-7 text-primary" />
            </div>
            <p className="text-sm font-semibold">Send a message to start testing</p>
            <p className="text-xs text-muted-foreground text-center">
              Messages are sent to your deployed agent in real time
            </p>
          </div>
        )}
        {messages.map((msg) => (
          <div
            key={msg.id}
            className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
          >
            <div
              className={`max-w-[80%] px-3.5 py-2.5 text-sm ${
                msg.role === "user"
                  ? "bg-primary text-primary-foreground rounded-lg rounded-br-sm"
                  : "bg-muted text-foreground rounded-lg rounded-bl-sm"
              }`}
            >
              {msg.role === "assistant" ? (
                <>
                  {msg.reasoning && (
                    <div className="text-xs text-muted-foreground italic mb-2 pb-2 border-b border-border">
                      {msg.reasoning}
                    </div>
                  )}
                  {msg.steps && msg.steps.length > 0 && (
                    <div className="mb-2 space-y-1">
                      {msg.steps.map((step) => (
                        <div
                          key={step.id}
                          className="flex items-center gap-1.5 text-xs text-muted-foreground"
                        >
                          {step.status === "running" ? (
                            <Loader2 size={10} className="animate-spin" />
                          ) : (
                            <Check size={10} className="text-green-600" />
                          )}
                          <Wrench size={10} />
                          <span>{step.name}</span>
                        </div>
                      ))}
                    </div>
                  )}
                  {msg.content ? (
                    <div className="prose prose-sm max-w-none [&>*:first-child]:mt-0 [&>*:last-child]:mb-0">
                      <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
                    </div>
                  ) : msg.isStreaming ? (
                    <span className="inline-block w-2 h-4 bg-muted-foreground animate-pulse" />
                  ) : null}
                </>
              ) : (
                <span className="whitespace-pre-wrap">{msg.content}</span>
              )}
            </div>
          </div>
        ))}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t border-border p-3 shrink-0">
        <div className="w-full rounded-lg bg-background border border-border">
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Send a message..."
            rows={2}
            className="w-full resize-none bg-transparent px-4 pt-3 pb-2 text-sm outline-none placeholder:text-muted-foreground"
            style={{ maxHeight: "120px" }}
            disabled={isLoading}
          />
          <div className="flex items-center px-3 pb-3">
            <div className="ml-auto">
              <button
                onClick={sendMessage}
                disabled={isLoading || !input.trim()}
                className="size-7 rounded-full bg-primary text-primary-foreground border-none cursor-pointer hover:bg-primary/90 disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center"
              >
                {isLoading ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
              </button>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
}
