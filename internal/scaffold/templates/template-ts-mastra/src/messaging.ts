import type { Agent } from '@mastra/core/agent';
import type { IMastraLogger } from '@mastra/core/logger';
import { MessagingBridge } from '@astropods/adapter-core';
import { MastraAdapter } from '@astropods/adapter-mastra';

/**
 * Messaging wiring, replacing `serve()` from `@astropods/adapter-mastra`.
 *
 * `serve()` constructs a MessagingBridge and drops the reference, leaving no way to
 * send outside an inbound turn — which makes scheduled runs invisible. We construct
 * the bridge ourselves and keep it.
 *
 * Exactly ONE bidirectional stream per process: the messaging service picks an
 * arbitrary entry from its stream map when routing inbound platform messages
 * (`HandleIncomingMessage`), so a second stream would swallow a random share of the
 * user's chat messages. That rules out opening a separate `MessagingClient`.
 */

/**
 * The bridge's public API is start/stop/sendRenderable, and renderables are
 * interactive forms (`RenderKind` is `UNSPECIFIED | FORM`) that cannot carry a plain
 * notification — so reaching its stream is the only way to push text. Isolated here
 * and probed at runtime so a future adapter-core reshaping the field degrades to a
 * warning instead of a crash.
 */
type ContentSender = {
  sendContentChunk: (
    conversationId: string,
    chunk: { type: 'START' | 'DELTA' | 'END' | 'REPLACE'; content: string },
  ) => void;
};

let bridge: MessagingBridge | null = null;
let logger: IMastraLogger | null = null;

function contentSender(): ContentSender | null {
  const stream = (bridge as unknown as { stream?: unknown } | null)?.stream;
  if (
    stream &&
    typeof (stream as ContentSender).sendContentChunk === 'function'
  ) {
    return stream as ContentSender;
  }
  return null;
}

/** Connect the agent to the messaging sidecar. Replaces `serve(agent)`. */
export async function startMessaging(agent: Agent, log: IMastraLogger): Promise<void> {
  logger = log;
  bridge = new MessagingBridge(new MastraAdapter(agent));
  await bridge.start();
}

/**
 * Deliver an unsolicited message into a conversation. Returns false when no stream is
 * available, so callers can log rather than assume delivery.
 *
 * Note the chunk shape is web-specific and will not post on Slack, whose adapter
 * posts its accumulated DELTA buffer and skips an empty one. A single shape works on
 * both surfaces only once the web adapter stops reading the message body from END
 * alone; at that point move the text to a DELTA and send an empty END.
 */
export function postToConversation(conversationId: string, text: string): boolean {
  const sender = contentSender();
  if (!sender) {
    logger?.warn('No messaging stream; proactive message not delivered', {
      conversationId,
    });
    return false;
  }

  // START opens the message; END carries the whole body. The text has to be on END
  // because the web adapter persists `payload.Content.Content` from END alone — an
  // empty END stores a blank message. Sending it as a DELTA *and* on END would double
  // it, since the adapter's persist buffer appends every chunk after a START reset.
  sender.sendContentChunk(conversationId, { type: 'START', content: '' });
  sender.sendContentChunk(conversationId, { type: 'END', content: text });
  logger?.debug('Delivered a proactive message', { conversationId, chars: text.length });
  return true;
}
