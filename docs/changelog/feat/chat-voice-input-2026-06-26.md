## Summary

Chat had no voice input. This adds browser-native dictation to the deployment chat composer: click the mic, speak, and your words are transcribed into the input as text and sent to the agent like any other message.

## Design

Dictation uses assistant-ui's first-party `DictationAdapter`. The deployment chat runtime registers the built-in `WebSpeechDictationAdapter` (the browser's Web Speech API) when supported, and the composer renders a mic button that toggles `startDictation()` / `stopDictation()` on the composer runtime. Transcription happens entirely in the browser; the agent receives a normal text message over the existing chat path — no audio leaves the client to our backend, and there is no client-side speech model.

This deliberately avoids a heavier audio-streaming pipeline. Speech-to-text runs in the browser, so there is:
- no VAD/ONNX model or WASM to vendor or load (no `@ricky0123/vad-web`, no `onnxruntime-web`, no `public/vad/` assets, no CDN),
- no audio WebSocket and no WebSocket passthrough in the messaging proxy — chat stays request/response + SSE as before.

The mic button only appears where the Web Speech API is available (Chrome/Edge/Safari); other browsers simply see no mic. Because dictation produces text, it works for any chat-eligible agent and is independent of the agent's server-side capabilities.

## Migration

No action required. The mic button appears automatically in supported browsers. Note: with the Web Speech API, the browser may send captured audio to its vendor's speech service for transcription (e.g. Chrome → Google); this is browser behavior, not an Astro backend dependency.
