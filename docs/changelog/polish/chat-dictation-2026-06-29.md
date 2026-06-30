## Summary

Chat dictation (voice → text) churned mid-session: the words the browser was
still recognizing flickered, rewrote, and appeared to delete as you spoke, only
settling into the correct text when a phrase finalized. This makes the textbox
fill with confirmed words only, while a muted live caption shows what's still
being heard.

## Design

The Web Speech API streams two kinds of results: volatile *interim* hypotheses
(a single in-progress phrase the engine continually rewrites) and stable *final*
results (emitted once it's confident, at a pause). assistant-ui's composer binds
the input to whatever the dictation adapter reports, so interim hypotheses made
the textarea rewrite itself wholesale until the final landed.

The fix keeps using assistant-ui's composer dictation (start/stop, active state,
mic button, final → input commit) but swaps in a thin custom `DictationAdapter`
— the library's supported extension point — that wraps the built-in
`WebSpeechDictationAdapter`:

- **final** results are forwarded to the composer, so only confirmed words are
  appended to the input (clean, no churn);
- **interim** hypotheses are diverted to a small store (`useDictationInterim`)
  and rendered as a muted "still listening" caption above the composer input,
  so there's live feedback without the textbox rewriting.

Net effect: the input accumulates correct text phrase-by-phrase, and the live
caption shows the in-progress guess. No server, proto, or runtime changes.

## Migration

No action required.
