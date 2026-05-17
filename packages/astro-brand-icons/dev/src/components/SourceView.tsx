import { useEffect, useRef, useState } from "react";
import { chatTurn } from "../api";
import type { ChatMessage, SourceCandidate } from "../types";
import { Editor } from "./Editor";

interface Selection {
  messageIdx: number;
  candidateIdx: number;
}

export function SourceView({
  onLibraryChanged,
}: {
  onLibraryChanged: () => void;
}) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Selection | null>(null);
  const [savedNotice, setSavedNotice] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const scrollerRef = useRef<HTMLDivElement | null>(null);

  // Auto-scroll on new messages.
  useEffect(() => {
    const el = scrollerRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages, loading, savedNotice]);

  async function send() {
    const text = draft.trim();
    if (!text || loading) return;
    abortRef.current?.abort();
    const ac = new AbortController();
    abortRef.current = ac;

    setMessages((m) => [...m, { role: "user", text }]);
    setDraft("");
    setLoading(true);
    setError(null);
    setSavedNotice(null);

    try {
      const res = await chatTurn({
        prompt: text,
        sessionId: sessionId ?? undefined,
        signal: ac.signal,
      });
      setSessionId(res.sessionId);
      setMessages((m) => [
        ...m,
        {
          role: "assistant",
          text: res.turn.text,
          candidates: res.turn.candidates,
        },
      ]);
    } catch (e) {
      if ((e as Error).name !== "AbortError") setError(String(e));
    } finally {
      if (abortRef.current === ac) setLoading(false);
    }
  }

  function cancel() {
    abortRef.current?.abort();
    setLoading(false);
  }

  function reset() {
    abortRef.current?.abort();
    setMessages([]);
    setSessionId(null);
    setSelected(null);
    setError(null);
    setLoading(false);
    setDraft("");
    setSavedNotice(null);
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      send();
    }
  }

  const selectedCandidate =
    selected != null
      ? messages[selected.messageIdx]?.candidates?.[selected.candidateIdx]
      : undefined;

  return (
    <div className="flex flex-col h-full max-w-5xl mx-auto w-full">
      <div ref={scrollerRef} className="flex-1 overflow-y-auto px-6 py-6">
        {messages.length === 0 && !loading && (
          <EmptyState onPick={setDraft} />
        )}

        <div className="space-y-6">
          {messages.map((m, i) => (
            <MessageBubble
              key={i}
              message={m}
              messageIdx={i}
              selected={selected}
              onSelect={(candidateIdx) => setSelected({ messageIdx: i, candidateIdx })}
            />
          ))}

          {loading && (
            <div className="text-sm text-white/60">
              <Spinner /> Claude is thinking…
            </div>
          )}
          {error && (
            <div className="text-sm text-red-300/80 font-mono whitespace-pre-wrap">
              {error}
            </div>
          )}
        </div>

        {selectedCandidate && selected && (
          <div className="mt-8">
            <Editor
              key={`${selected.messageIdx}-${selected.candidateIdx}`}
              initialId={selectedCandidate.id}
              initialLightSvg={selectedCandidate.lightSvg}
              initialDarkSvg={selectedCandidate.darkSvg}
              onSaved={(savedId) => {
                setSelected(null);
                setSavedNotice(savedId);
                onLibraryChanged();
              }}
            />
          </div>
        )}

        {savedNotice && (
          <div className="mt-6 flex items-center gap-2 rounded-md border border-emerald-400/20 bg-emerald-400/[0.05] px-3 py-2 text-xs">
            <span className="font-mono text-emerald-300">✓ Saved</span>
            <span className="font-mono text-white/80">{savedNotice}</span>
            <span className="text-white/40">
              — light + dark written to{" "}
              <span className="font-mono">sources/</span> and regenerated in{" "}
              <span className="font-mono">assets/integrations/</span>.
            </span>
            <button
              onClick={() => setSavedNotice(null)}
              className="ml-auto text-white/40 hover:text-white"
              aria-label="Dismiss"
            >
              ×
            </button>
          </div>
        )}
      </div>

      <div className="border-t border-white/10 bg-[#0b0c0f] px-6 py-4">
        <div className="flex items-end gap-2">
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKeyDown}
            rows={2}
            placeholder={
              messages.length === 0
                ? "Describe what you need: “the Vercel logo, monochrome”, “Cursor.so square mark”, “Stripe S only”…"
                : "Follow up: ask for variants, edits, replacements…"
            }
            className="flex-1 bg-white/5 border border-white/10 rounded-md px-3 py-2 text-sm placeholder:text-white/30 focus:outline-none focus:border-white/30 resize-y font-mono min-h-[44px]"
            disabled={loading}
          />
          <div className="flex flex-col gap-1.5">
            <button
              type="button"
              onClick={send}
              disabled={!draft.trim() || loading}
              className="px-4 py-2 rounded-md bg-white text-black text-sm font-medium hover:bg-white/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              {loading ? "…" : "Send"}
            </button>
            {loading ? (
              <button
                type="button"
                onClick={cancel}
                className="px-3 py-1 rounded-md border border-white/15 text-white/70 text-xs hover:bg-white/5"
              >
                Cancel
              </button>
            ) : messages.length > 0 ? (
              <button
                type="button"
                onClick={reset}
                className="px-3 py-1 rounded-md border border-white/15 text-white/50 text-xs hover:bg-white/5"
              >
                New chat
              </button>
            ) : null}
          </div>
        </div>
        <div className="text-[10px] text-white/30 mt-1.5 flex items-center gap-3">
          <span>⌘↵ to send</span>
          {sessionId && (
            <span className="font-mono truncate">
              session: {sessionId.slice(0, 8)}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

function MessageBubble({
  message,
  messageIdx,
  selected,
  onSelect,
}: {
  message: ChatMessage;
  messageIdx: number;
  selected: Selection | null;
  onSelect: (candidateIdx: number) => void;
}) {
  const isUser = message.role === "user";
  return (
    <div className={isUser ? "flex justify-end" : "flex justify-start"}>
      <div className={isUser ? "max-w-[80%]" : "max-w-full w-full"}>
        <div className="text-[10px] uppercase tracking-wider text-white/30 mb-1.5">
          {isUser ? "You" : "Agent"}
        </div>
        <div
          className={
            isUser
              ? "rounded-lg bg-white/10 px-3 py-2 text-sm whitespace-pre-wrap"
              : "rounded-lg bg-white/[0.03] border border-white/10 px-4 py-3 text-sm whitespace-pre-wrap"
          }
        >
          {message.text || (isUser ? "" : "(no text)")}
        </div>

        {!isUser && message.candidates && message.candidates.length > 0 && (
          <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(200px,1fr))] mt-3">
            {message.candidates.map((c, i) => (
              <CandidateCard
                key={i}
                candidate={c}
                selected={
                  selected?.messageIdx === messageIdx &&
                  selected?.candidateIdx === i
                }
                onSelect={() => onSelect(i)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function CandidateCard({
  candidate,
  selected,
  onSelect,
}: {
  candidate: SourceCandidate;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={
        "text-left rounded-lg border overflow-hidden transition-colors " +
        (selected
          ? "border-white/40 ring-1 ring-white/40"
          : "border-white/10 hover:border-white/25")
      }
    >
      <div className="grid grid-cols-2">
        <div className="aspect-square bg-white flex items-center justify-center p-4">
          <SvgPreview svg={candidate.lightSvg} />
        </div>
        <div className="aspect-square bg-[#0b0c0f] flex items-center justify-center p-4">
          <SvgPreview svg={candidate.darkSvg} />
        </div>
      </div>
      <div className="px-3 py-2 border-t border-white/10 text-xs space-y-0.5">
        <div className="flex items-baseline gap-2">
          <span className="font-mono text-white/90 truncate">{candidate.id}</span>
          {candidate.brand && candidate.brand.toLowerCase() !== candidate.id && (
            <span className="text-white/50 truncate">{candidate.brand}</span>
          )}
        </div>
        <div className="text-white/60 truncate">{candidate.label}</div>
        {candidate.sourceUrl && (
          <a
            href={candidate.sourceUrl}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
            className="text-white/40 hover:text-white/70 font-mono text-[10px] truncate block"
          >
            {hostname(candidate.sourceUrl)}
          </a>
        )}
      </div>
    </button>
  );
}

function SvgPreview({ svg }: { svg: string }) {
  return (
    <div
      className="w-full h-full [&>svg]:w-full [&>svg]:h-full [&>svg]:max-w-12 [&>svg]:max-h-12 flex items-center justify-center"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}

function EmptyState({ onPick }: { onPick: (s: string) => void }) {
  const suggestions = [
    "Find the Vercel logo, monochrome preferred",
    "Source the Cursor.so icon as a square mark",
    "The Stripe S, no wordmark",
    "Linear's mark in its brand purple",
  ];
  return (
    <div className="text-center py-12">
      <div className="text-sm text-white/50 mb-4">
        Start a conversation. Describe an icon, then refine — the agent can
        re-source or edit the SVG inline.
      </div>
      <div className="flex flex-wrap justify-center gap-2">
        {suggestions.map((s) => (
          <button
            key={s}
            onClick={() => onPick(s)}
            className="text-xs px-3 py-1.5 rounded-full border border-white/10 text-white/60 hover:text-white hover:border-white/25"
          >
            {s}
          </button>
        ))}
      </div>
    </div>
  );
}

function Spinner() {
  return (
    <span className="inline-block w-3 h-3 rounded-full border-2 border-white/30 border-t-white animate-spin align-[-2px] mr-1" />
  );
}

function hostname(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}
