# Session-aware eval judge predictions

## Summary

Eval judge predictions now consider the recent conversation that preceded a target trace. This prevents contextual follow-up answers from being marked incorrect simply because the judge evaluated the turn in isolation.

Trace and signal content is preserved for normal-sized values, with a high defensive ceiling preventing a single oversized field from exhausting the model context or creating unbounded token cost. Oversized judge explanations are visibly shortened instead of failing the prediction.

## Design

The prediction worker loads the three traces immediately preceding the target from the same deployment, user, and Langfuse session. The query is bounded by the target timestamp and ordered newest-first for efficient retrieval; the target and any same-time or future traces are excluded before the context is restored to chronological order.

Previous trace inputs and outputs are sent as a separate `previous_turns` collection. The judge uses these turns only to interpret references and follow-up questions, while scoring only the target output. The later user message remains separate reaction evidence and is never treated as information available to the agent at response time.

The target, prior-turn, and next-user values share one high per-field rune ceiling. Values below it retain their native JSON structure; oversized serialized values preserve their beginning and end around an omission marker. Session context is additionally bounded by turn count.

Explanation guidance asks the model to aim for 120–180 characters and never exceed 220. If the model exceeds the database's 240-character ceiling, Astro reserves the final three characters for an ellipsis so the stored explanation remains bounded and visibly incomplete without failing the prediction. The provider-portable response schema does not rely on unsupported string-length constraints, and the judge version remains unchanged.

## Migration

No migration or configuration changes are required. Existing predictions remain valid; newly generated predictions receive the additional session context.
