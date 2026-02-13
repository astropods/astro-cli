import { watch } from "fs";
import { generateIdentity } from "./src/index";

const count = 100;
const size = 128;

let version = 0;
const clients = new Set<ReadableStreamDefaultController>();

function generateBatch(prefix: string): { seed: string; svg: string }[] {
  return Array.from({ length: count }, (_, i) => {
    const seed = `${prefix}-${i}`;
    return { seed, svg: generateIdentity({ seed, size }) };
  });
}

function buildHtml(): string {
  return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Identity Generator Preview</title>
  <style>
    body { margin: 0; padding: 24px; background: #111; font-family: system-ui, sans-serif; }
    .toolbar { margin-bottom: 16px; }
    .toolbar button {
      background: #333; color: #eee; border: 1px solid #555; border-radius: 6px;
      padding: 8px 20px; font-size: 14px; cursor: pointer; font-family: inherit;
    }
    .toolbar button:hover { background: #444; }
    .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(${size + 16}px, 1fr)); gap: 16px; }
    .cell { display: flex; flex-direction: column; align-items: center; gap: 4px; }
    .cell img { border-radius: 8px; }
    .cell span { font-size: 11px; color: #888; }
  </style>
</head>
<body>
  <div class="toolbar">
    <button onclick="generate()">Generate</button>
  </div>
  <div class="grid" id="grid"></div>
  <script>
    async function generate() {
      const res = await fetch("/__generate");
      const items = await res.json();
      const grid = document.getElementById("grid");
      grid.innerHTML = items.map(item =>
        '<div class="cell">' +
          '<img src="data:image/svg+xml,' + encodeURIComponent(item.svg) + '" width="${size}" height="${size}" />' +
          '<span>' + item.seed + '</span>' +
        '</div>'
      ).join("");
    }
    generate();
    const es = new EventSource("/__reload");
    es.onmessage = () => { generate(); };
  </script>
</body>
</html>`;
}

const server = Bun.serve({
  port: 3456,
  fetch(req) {
    const url = new URL(req.url);
    if (url.pathname === "/__generate") {
      const prefix = url.searchParams.get("p") || Math.random().toString(36).slice(2);
      return Response.json(generateBatch(prefix));
    }
    if (url.pathname === "/__reload") {
      const stream = new ReadableStream({
        start(controller) {
          clients.add(controller);
          req.signal.addEventListener("abort", () => clients.delete(controller));
        },
      });
      return new Response(stream, {
        headers: {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          Connection: "keep-alive",
        },
      });
    }
    return new Response(buildHtml(), { headers: { "Content-Type": "text/html" } });
  },
});

// Watch src/ for changes and notify connected clients
watch(import.meta.dir + "/src", { recursive: true }, () => {
  version++;
  for (const client of clients) {
    try {
      client.enqueue(new TextEncoder().encode(`data: ${version}\n\n`));
    } catch {
      clients.delete(client);
    }
  }
});

console.log(`Preview: http://localhost:${server.port} (watching for changes)`);
