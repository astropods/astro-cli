import { useLayoutEffect, useRef } from "react";
import { useThreadViewport } from "@assistant-ui/react";
import { useDeploymentChatViewport } from "@/components/chat/deployment-chat-streaming-context";

/** After history load, keep correcting async markdown layout briefly. */
const LOAD_SETTLE_MS = 4000;

/**
 * Pins the thread to the latest message after history loads. Streamdown and
 * top-turn anchoring can leave the viewport mid-message on large browsers.
 * Live streaming scroll is handled by assistant-ui (`autoScroll` / top anchor).
 */
export function DeploymentChatHistoryScroll() {
  const { conversationId, historyLoading, isStreaming, streamingMessageId } =
    useDeploymentChatViewport();
  const scrollToBottom = useThreadViewport((s) => s.scrollToBottom);
  const viewportEl = useThreadViewport((s) => s.element.viewport);
  const generationRef = useRef(0);

  // Conversation switches reuse the same DOM viewport in assistant-ui unless the
  // runtime remounts; always reset scroll position immediately.
  useLayoutEffect(() => {
    if (!conversationId) return;
    scrollToBottom({ behavior: "instant" });
  }, [conversationId, scrollToBottom]);

  useLayoutEffect(() => {
    const shouldPin =
      !!conversationId && !historyLoading && !isStreaming;
    if (!shouldPin) return;

    const generation = ++generationRef.current;

    const pin = () => {
      if (generation !== generationRef.current) return;
      scrollToBottom({ behavior: "instant" });
    };

    pin();
    const raf1 = requestAnimationFrame(pin);
    const raf2 = requestAnimationFrame(() => requestAnimationFrame(pin));

    const el = viewportEl;
    if (!el) {
      return () => {
        cancelAnimationFrame(raf1);
        cancelAnimationFrame(raf2);
      };
    }

    let lastHeight = el.scrollHeight;
    const observer = new ResizeObserver(() => {
      if (generation !== generationRef.current) return;
      const nextHeight = el.scrollHeight;
      if (nextHeight === lastHeight) return;
      lastHeight = nextHeight;
      pin();
    });
    observer.observe(el);

    const stop = window.setTimeout(() => observer.disconnect(), LOAD_SETTLE_MS);

    return () => {
      generationRef.current += 1;
      cancelAnimationFrame(raf1);
      cancelAnimationFrame(raf2);
      observer.disconnect();
      window.clearTimeout(stop);
    };
  }, [
    conversationId,
    historyLoading,
    isStreaming,
    streamingMessageId,
    scrollToBottom,
    viewportEl,
  ]);

  return null;
}
