/**
 * Client-side dev script.
 * Handles avatar picker, color extraction, and card rendering entirely in JS.
 */

// Accept HMR updates — triggers full page reload on any source change
if (import.meta.hot) {
  import.meta.hot.accept(() => {
    location.reload();
  });
}
import { extractPalette, pickCardColors } from "./src/mmcq";
import { generateCard } from "./src/index";
import { downloadSvg, downloadPng } from "./src/browser";
import { generateIdentity } from "identity-gen";
import type { CardAvatar, CardColors, CardData } from "./src/types";

// --- Sample data ---

const sampleData: Omit<CardData, "avatar" | "colors"> = {
  name: "research-assistant",
  displayName: "Research Assistant",
  account: "acme-labs",
  description: "An AI research assistant that helps you find, summarize, and synthesize academic papers.",
  tags: ["research", "summarization", "RAG"],
  heartCount: 128,
  stats: [
    { label: "Deployed by", account: { fullName: "Jane Smith", handle: "jsmith", avatarUrl: null } },
    { label: "Version", value: "1.4.2" },
    { label: "Hearts", value: "128" },
    { label: "Deploys", value: "1,247" },
  ],
  integrations: [
    { name: "Slack", icon: `<path d="M5.042 15.165a2.528 2.528 0 0 1-2.52 2.523A2.528 2.528 0 0 1 0 15.165a2.527 2.527 0 0 1 2.522-2.52h2.52v2.52zm1.271 0a2.527 2.527 0 0 1 2.521-2.52 2.527 2.527 0 0 1 2.521 2.52v6.313A2.528 2.528 0 0 1 8.834 24a2.528 2.528 0 0 1-2.521-2.522v-6.313zM8.834 5.042a2.528 2.528 0 0 1-2.521-2.52A2.528 2.528 0 0 1 8.834 0a2.528 2.528 0 0 1 2.521 2.522v2.52H8.834zm0 1.271a2.528 2.528 0 0 1 2.521 2.521 2.528 2.528 0 0 1-2.521 2.521H2.522A2.528 2.528 0 0 1 0 8.834a2.528 2.528 0 0 1 2.522-2.521h6.312zM18.956 8.834a2.528 2.528 0 0 1 2.522-2.521A2.528 2.528 0 0 1 24 8.834a2.528 2.528 0 0 1-2.522 2.521h-2.522V8.834zm-1.27 0a2.528 2.528 0 0 1-2.523 2.521 2.527 2.527 0 0 1-2.52-2.521V2.522A2.527 2.527 0 0 1 15.163 0a2.528 2.528 0 0 1 2.523 2.522v6.312zM15.163 18.956a2.528 2.528 0 0 1 2.523 2.522A2.528 2.528 0 0 1 15.163 24a2.527 2.527 0 0 1-2.52-2.522v-2.522h2.52zm0-1.27a2.527 2.527 0 0 1-2.52-2.523 2.527 2.527 0 0 1 2.52-2.52h6.315A2.528 2.528 0 0 1 24 15.163a2.528 2.528 0 0 1-2.522 2.523h-6.315z" fill="currentColor"/>` },
    { name: "GitHub", icon: `<path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z" fill="currentColor"/>` },
    { name: "Linear", icon: `<path d="M1.16 17.37l5.93 5.93a11.86 11.86 0 0 1-5.93-5.93zM.5 13.5l10 10a11.93 11.93 0 0 0 3.86-1.25L1.75 9.64A11.93 11.93 0 0 0 .5 13.5zm2.1-6.07l13.97 13.97a11.8 11.8 0 0 0 2.56-2.56L5.16 4.87a11.8 11.8 0 0 0-2.56 2.56zM9.64 1.75L22.25 14.36a11.93 11.93 0 0 0 1.25-3.86l-10-10a11.93 11.93 0 0 0-3.86 1.25zM17.37 1.16l5.47 5.47A12.01 12.01 0 0 0 17.37 1.16z" fill="currentColor"/>` },
    { name: "Notion" },
    { name: "Google Drive" },
  ],
  barcodeId: "AGT-7f3a9b2e-01",
};

interface AvatarSample {
  label: string;
  avatar: CardAvatar | undefined;
  thumb: string;
  source: string | null;
}

function identityAvatar(seed: string, label: string): AvatarSample {
  const svg = generateIdentity({ seed, size: 128 });
  const inner = svg.replace(/<svg[^>]*>/, "").replace(/<\/svg>/, "");
  const thumbSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 128 128">${inner}</svg>`;
  const full = `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">${inner}</svg>`;
  return {
    label,
    avatar: { svg: inner },
    thumb: thumbSvg,
    source: "data:image/svg+xml;charset=utf-8," + encodeURIComponent(full),
  };
}

const avatars: AvatarSample[] = [
  identityAvatar("astro/zenith", "Zenith"),
  identityAvatar("nova/pulse", "Pulse"),
  identityAvatar("arc/bloom", "Bloom"),
  {
    label: "Photo: robot",
    avatar: { url: "https://images.unsplash.com/photo-1485827404703-89b55fcc595e?w=256&h=256&fit=crop" },
    thumb: `<img src="https://images.unsplash.com/photo-1485827404703-89b55fcc595e?w=256&h=256&fit=crop" width="48" height="48" style="object-fit:cover;border-radius:8px"/>`,
    source: "https://images.unsplash.com/photo-1485827404703-89b55fcc595e?w=256&h=256&fit=crop",
  },
  {
    label: "Photo: abstract",
    avatar: { url: "https://images.unsplash.com/photo-1634017839464-5c339ebe3cb4?w=256&h=256&fit=crop" },
    thumb: `<img src="https://images.unsplash.com/photo-1634017839464-5c339ebe3cb4?w=256&h=256&fit=crop" width="48" height="48" style="object-fit:cover;border-radius:8px"/>`,
    source: "https://images.unsplash.com/photo-1634017839464-5c339ebe3cb4?w=256&h=256&fit=crop",
  },
  {
    label: "OpenClaw",
    avatar: { url: "/assets/openclaw.png" },
    thumb: `<img src="/assets/openclaw.png" width="48" height="48" style="object-fit:cover;border-radius:8px"/>`,
    source: "/assets/openclaw.png",
  },
  {
    label: "No avatar",
    avatar: undefined,
    thumb: `<div style="width:48px;height:48px;background:#333;border-radius:8px;display:flex;align-items:center;justify-content:center;font-size:10px;opacity:0.5">none</div>`,
    source: null,
  },
];

// --- Color extraction ---

const DEFAULT_COLORS: CardColors = {
  background: "#1a1a2e",
  foreground: "#eaeaee",
  accent: "#6366f1",
  accentLight: "#a5b4fc",
};

async function extractColors(source: string | null): Promise<CardColors> {
  if (!source) return DEFAULT_COLORS;

  const img = new Image();
  img.crossOrigin = "anonymous";
  img.src = source;
  await new Promise((resolve, reject) => { img.onload = resolve; img.onerror = reject; });

  const size = 64;
  const canvas = document.createElement("canvas");
  canvas.width = size; canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  ctx.drawImage(img, 0, 0, size, size);
  const { data } = ctx.getImageData(0, 0, size, size);

  const palette = extractPalette(data, 8, 1);
  return pickCardColors(palette) ?? DEFAULT_COLORS;
}

// --- UI ---

let selectedIdx = 0;

function buildPicker() {
  const row = document.getElementById("avatar-row")!;
  row.innerHTML = avatars.map((a, i) =>
    `<button class="avatar-pick${i === 0 ? " active" : ""}" data-idx="${i}" title="${a.label}">${a.thumb}<span>${a.label}</span></button>`
  ).join("");

  row.addEventListener("click", (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLElement>(".avatar-pick");
    if (!btn) return;
    const idx = parseInt(btn.dataset.idx!, 10);
    render(idx);
  });
}

let lastSvg = "";

async function render(idx: number) {
  selectedIdx = idx;

  document.querySelectorAll(".avatar-pick").forEach((el, i) => {
    el.classList.toggle("active", i === idx);
  });

  const sample = avatars[idx];
  const colors = await extractColors(sample.source);
  const data: CardData = { ...sampleData, avatar: sample.avatar, colors, displayName: sample.label };
  lastSvg = generateCard(data, { variant: "standard" });

  const slot = document.getElementById("card-slot")!;
  slot.innerHTML = `<div style="perspective:600px;display:inline-block"><div class="holo-card"><div style="border-radius:16px;overflow:hidden">${lastSvg}</div><div class="holo-card__shine"></div><div class="holo-card__glare"></div></div></div>`;
  setupHolo(slot.querySelector<HTMLElement>(".holo-card")!);
}

// --- Holographic hover ---

function clamp(v: number, min = 0, max = 100) { return Math.min(max, Math.max(min, v)); }

function setupHolo(el: HTMLElement) {
  el.addEventListener("pointermove", (e: PointerEvent) => {
    const rect = el.getBoundingClientRect();
    const px = clamp(((e.clientX - rect.left) / rect.width) * 100);
    const py = clamp(((e.clientY - rect.top) / rect.height) * 100);
    const cx = px - 50;
    const cy = py - 50;
    const dist = Math.sqrt(cx * cx + cy * cy) / 50;
    const s = el.style;
    s.setProperty("--px", `${px}%`);
    s.setProperty("--py", `${py}%`);
    s.setProperty("--fl", String(px / 100));
    s.setProperty("--ft", String(py / 100));
    s.setProperty("--fc", String(clamp(dist, 0, 1)));
    s.setProperty("--o", "1");
    s.setProperty("--rx", `${-(cx / 4)}deg`);
    s.setProperty("--ry", `${cy / 4}deg`);
  });
  el.addEventListener("pointerleave", () => {
    el.style.setProperty("--o", "0");
    el.style.setProperty("--rx", "0deg");
    el.style.setProperty("--ry", "0deg");
  });
}

// --- Download buttons ---
function setupDownloads() {
  document.getElementById("dl-svg")?.addEventListener("click", async () => {
    if (lastSvg) await downloadSvg(lastSvg, { name: sampleData.name, id: sampleData.barcodeId ?? "unknown" });
  });
  document.getElementById("dl-png")?.addEventListener("click", async () => {
    if (lastSvg) await downloadPng(lastSvg, { name: sampleData.name, id: sampleData.barcodeId ?? "unknown" });
  });
}

buildPicker();
render(0);
setupDownloads();
