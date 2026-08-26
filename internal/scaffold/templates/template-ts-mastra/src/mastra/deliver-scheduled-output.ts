import type { ProcessOutputResultArgs } from '@mastra/core/processors';
import { postToConversation } from '../messaging';

/**
 * Request-context key carrying the conversation a scheduled run should be delivered
 * to. Set by `start_schedule` through `ifIdle.streamOptions.requestContext`, which is
 * the documented way to pass context into a schedule-woken run.
 */
export const SCHEDULE_DELIVERY_KEY = 'scheduleDeliveryConversationId';

/**
 * Deliver the output of a schedule-woken run back to the user.
 *
 * `schedules.onFinish` cannot do this. For a threaded schedule the worker calls
 * `agent.sendSignal()` and fires the hook as soon as the signal is *accepted*, with
 * no result attached — the run has not produced text yet. (Only the threadless
 * `agent.generate()` branch populates `result`.) So delivery has to happen where the
 * text exists: at the end of the run itself.
 *
 * A no-op for ordinary turns, because only schedule-woken runs carry the key. That
 * matters — user-initiated turns are already streamed to the client by the messaging
 * bridge, and pushing here would duplicate them.
 */
export const deliverScheduledOutput = {
  id: 'deliver-scheduled-output',

  processOutputResult: ({ result, messages, requestContext }: ProcessOutputResultArgs) => {
    const conversationId = requestContext?.get(SCHEDULE_DELIVERY_KEY);
    const text = result.text?.trim();

    if (typeof conversationId === 'string' && conversationId && text) {
      postToConversation(conversationId, text);
    }

    return messages;
  },
};
