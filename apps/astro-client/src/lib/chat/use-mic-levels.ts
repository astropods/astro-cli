import { useEffect, useRef, useState } from "react";

// Number of bars in the waveform history. Newest sample is the last element
// (rendered on the right); the array scrolls left as samples age.
const DEFAULT_BARS = 56;

// Speech RMS sits roughly in 0.02–0.3; scale so normal talking fills most of
// the bar height without constantly clipping at 1.
const LEVEL_GAIN = 3.4;

// How often a new bar is committed to the history. Larger = slower scroll. We
// still read the analyser every frame (for a smooth peak per sample) but only
// push a bar this often.
const SAMPLE_INTERVAL_MS = 110;

type WindowWithWebkitAudio = Window &
  typeof globalThis & { webkitAudioContext?: typeof AudioContext };

/**
 * Drives an audio-reactive waveform from the live microphone.
 *
 * While `active`, opens a dedicated `getUserMedia` stream and reads real
 * amplitude (RMS over the time-domain buffer) on every animation frame,
 * appending each sample to a fixed-length ring buffer of bar heights (0–1).
 * This is a second mic consumer alongside the Web Speech recognition session —
 * browsers allow concurrent captures of the same device, and we need our own
 * stream because the Speech API never exposes audio.
 *
 * Returns all-zero bars when idle, when the mic is denied, or during SSR, so
 * callers can render unconditionally. The stream, AudioContext, and rAF loop
 * are torn down as soon as `active` flips false or the component unmounts.
 */
export function useMicLevels(active: boolean, bars = DEFAULT_BARS): number[] {
  const [levels, setLevels] = useState<number[]>(() =>
    new Array(bars).fill(0),
  );
  const rafRef = useRef<number | null>(null);

  useEffect(() => {
    if (!active) return;
    if (typeof navigator === "undefined" || !navigator.mediaDevices) return;

    let cancelled = false;
    let stream: MediaStream | null = null;
    let ctx: AudioContext | null = null;
    let analyser: AnalyserNode | null = null;
    let source: MediaStreamAudioSourceNode | null = null;
    const history = new Array(bars).fill(0);

    const start = async () => {
      let micStream: MediaStream;
      try {
        micStream = await navigator.mediaDevices.getUserMedia({ audio: true });
      } catch {
        // Mic denied/unavailable — leave the bars flat. Recognition surfaces its
        // own error separately; the waveform just stays quiet.
        return;
      }
      if (cancelled) {
        micStream.getTracks().forEach((t) => t.stop());
        return;
      }
      stream = micStream;

      const AudioCtx =
        window.AudioContext ?? (window as WindowWithWebkitAudio).webkitAudioContext;
      if (!AudioCtx) return;
      ctx = new AudioCtx();
      // Autoplay policies can hand back a suspended context.
      if (ctx.state === "suspended") await ctx.resume().catch(() => {});

      analyser = ctx.createAnalyser();
      analyser.fftSize = 1024;
      analyser.smoothingTimeConstant = 0.6;
      source = ctx.createMediaStreamSource(stream);
      source.connect(analyser);

      const buffer = new Uint8Array(analyser.fftSize);
      let lastSampleAt = 0;
      let peak = 0;
      const tick = (now: number) => {
        if (cancelled || !analyser) return;
        analyser.getByteTimeDomainData(buffer);
        let sumSquares = 0;
        for (let i = 0; i < buffer.length; i++) {
          const centered = (buffer[i] - 128) / 128;
          sumSquares += centered * centered;
        }
        const rms = Math.sqrt(sumSquares / buffer.length);
        peak = Math.max(peak, Math.min(1, rms * LEVEL_GAIN));

        // Commit one bar per interval so the waveform scrolls at a readable
        // pace; the bar shows the loudest moment since the last commit.
        if (now - lastSampleAt >= SAMPLE_INTERVAL_MS) {
          lastSampleAt = now;
          history.push(peak);
          history.shift();
          setLevels(history.slice());
          peak = 0;
        }
        rafRef.current = requestAnimationFrame(tick);
      };
      rafRef.current = requestAnimationFrame(tick);
    };

    void start();

    return () => {
      cancelled = true;
      if (rafRef.current != null) cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
      source?.disconnect();
      analyser?.disconnect();
      stream?.getTracks().forEach((t) => t.stop());
      void ctx?.close().catch(() => {});
      setLevels(new Array(bars).fill(0));
    };
  }, [active, bars]);

  return levels;
}
