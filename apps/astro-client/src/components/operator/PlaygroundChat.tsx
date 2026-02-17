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
    return (
      <div className="p-8 border border-stone-300 bg-stone-50 text-center">
        <MessageSquare size={32} className="mx-auto text-stone-400 mb-2" />
        <p className="text-stone-600 text-sm">No messaging-web endpoint available</p>
        <p className="text-stone-500 text-xs mt-1">
          Deploy with web adapter enabled to use the playground
        </p>
      </div>
    );
  }

  const selected = deploymentsWithUrl[selectedIndex];

  return (
    <div className="border border-stone-300 bg-white flex flex-col" style={{ height: "min(500px, calc(100vh - 280px))" }}>
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2.5 border-b border-stone-300 bg-stone-50 shrink-0">
        <div className="flex items-center gap-2">
          {deploymentsWithUrl.length > 1 ? (
            <div className="relative">
              <button
                onClick={() => setDropdownOpen(!dropdownOpen)}
                className="flex items-center gap-1.5 px-2.5 py-1.5 text-sm border border-stone-300 bg-white hover:bg-stone-50 cursor-pointer"
              >
                <span className="font-mono text-xs">{selected.deployment.build_id.slice(0, 8)}</span>
                <ChevronDown size={14} />
              </button>
              {dropdownOpen && (
                <div className="absolute top-full left-0 mt-1 bg-white border border-stone-300 shadow-lg z-10 min-w-[200px]">
                  {deploymentsWithUrl.map((d, i) => (
                    <button
                      key={d.deployment.build_id}
                      onClick={() => handleSelectDeployment(i)}
                      className={`w-full text-left px-3 py-2 text-sm hover:bg-stone-50 cursor-pointer border-none bg-transparent ${
                        i === selectedIndex ? "bg-stone-100 font-medium" : ""
                      }`}
                    >
                      <span className="font-mono text-xs">{d.deployment.build_id.slice(0, 8)}</span>
                      <span className="text-stone-500 ml-2">
                        {d.deployment.status === "running" ? "Running" : d.deployment.status}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          ) : (
            <span className="text-sm text-stone-600">
              Build <span className="font-mono text-xs">{selected.deployment.build_id.slice(0, 8)}</span>
            </span>
          )}
        </div>
        {messages.length > 0 && (
          <button
            onClick={resetConversation}
            className="flex items-center gap-1.5 px-2.5 py-1.5 text-xs text-stone-500 hover:text-stone-700 border border-stone-300 bg-white hover:bg-stone-50 cursor-pointer"
          >
            <RotateCcw size={12} />
            New chat
          </button>
        )}
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.length === 0 && (
          <div className="flex items-center justify-center h-full text-stone-400 text-sm">
            Send a message to start testing your agent
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
                  ? "bg-stone-800 text-white rounded-lg rounded-br-sm"
                  : "bg-stone-100 text-stone-900 rounded-lg rounded-bl-sm"
              }`}
            >
              {msg.role === "assistant" ? (
                <>
                  {msg.reasoning && (
                    <div className="text-xs text-stone-500 italic mb-2 pb-2 border-b border-stone-200">
                      {msg.reasoning}
                    </div>
                  )}
                  {msg.steps && msg.steps.length > 0 && (
                    <div className="mb-2 space-y-1">
                      {msg.steps.map((step) => (
                        <div
                          key={step.id}
                          className="flex items-center gap-1.5 text-xs text-stone-500"
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
                    <div className="prose prose-sm prose-stone max-w-none [&>*:first-child]:mt-0 [&>*:last-child]:mb-0">
                      <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
                    </div>
                  ) : msg.isStreaming ? (
                    <span className="inline-block w-2 h-4 bg-stone-400 animate-pulse" />
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
      <div className="border-t border-stone-300 p-3 shrink-0">
        <div className="flex items-end gap-2">
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Send a message..."
            rows={1}
            className="flex-1 resize-none border border-stone-300 px-3 py-2 text-sm bg-white focus:outline-none focus:border-stone-500 placeholder:text-stone-400"
            style={{ maxHeight: "120px" }}
            disabled={isLoading}
          />
          <button
            onClick={sendMessage}
            disabled={isLoading || !input.trim()}
            className="px-3 py-2 bg-stone-800 text-white border-none cursor-pointer hover:bg-stone-700 disabled:opacity-40 disabled:cursor-not-allowed flex items-center"
          >
            {isLoading ? <Loader2 size={16} className="animate-spin" /> : <Send size={16} />}
          </button>
        </div>
      </div>
    </div>
  );
}
