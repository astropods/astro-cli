import { reactRouter } from "@react-router/dev/vite";
import { defineConfig, loadEnv } from "vite";
import tailwindcss from "@tailwindcss/vite";
import { astroThemeColors } from "astro-theme/plugin";
import fs from "fs";
import path from "path";
import type { Plugin } from "vite";
import type { IncomingMessage, ServerResponse } from "http";

// Local development domain (must match what's in /etc/hosts)
const LOCAL_DOMAIN = "local.astropods.ai";

// Check for local HTTPS certificates
function getHttpsConfig() {
  const certDir = path.resolve(__dirname, ".certs");
  const certPath = path.join(certDir, `${LOCAL_DOMAIN}.pem`);
  const keyPath = path.join(certDir, `${LOCAL_DOMAIN}-key.pem`);

  if (fs.existsSync(certPath) && fs.existsSync(keyPath)) {
    return {
      cert: fs.readFileSync(certPath),
      key: fs.readFileSync(keyPath),
    };
  }
  return undefined;
}

// Vite plugin: intercept observability API requests server-side when VITE_MOCK_API=true.
// More reliable than MSW (no service worker activation race conditions).
function mockObservabilityPlugin(): Plugin {
  return {
    name: "mock-observability",
    configureServer(server) {
      server.middlewares.use((req: IncomingMessage, res: ServerResponse, next: () => void) => {
        const url = req.url ?? "";
        const match = url.match(/^\/api\/v1\/deployments\/([^/?]+)\/observability\/(metrics|summary|traces)/);
        if (!match) return next();

        const endpoint = match[2];
        const now = Date.now();

        let body: unknown;

        if (endpoint === "metrics") {
          const bucketCount = 24;
          const intervalMinutes = 60;
          const buckets = Array.from({ length: bucketCount }, (_, i) => {
            const base = 40 + Math.sin(i / 3) * 20;
            const spike = i === Math.floor(bucketCount * 0.6) ? 80 : 0;
            const traceCount = Math.max(0, Math.round(base + spike + Math.random() * 10));
            return {
              timestamp: new Date(now - (bucketCount - i) * intervalMinutes * 60 * 1000).toISOString(),
              trace_count: traceCount,
              avg_latency_ms: Math.round(320 + Math.sin(i / 4) * 80 + Math.random() * 40),
              p95_latency_ms: Math.round(720 + Math.sin(i / 4) * 150 + Math.random() * 80),
              input_tokens: traceCount * Math.round(180 + Math.random() * 60),
              output_tokens: traceCount * Math.round(420 + Math.random() * 100),
              error_count: Math.random() < 0.15 ? Math.floor(Math.random() * 3) + 1 : 0,
            };
          });
          body = {
            buckets,
            time_range: { start: buckets[0].timestamp, end: buckets[bucketCount - 1].timestamp },
            interval_minutes: intervalMinutes,
          };
        } else if (endpoint === "summary") {
          body = {
            total_traces: 936,
            time_range: { start: new Date(now - 24 * 60 * 60 * 1000).toISOString(), end: new Date(now).toISOString() },
            metrics: { avg_latency_ms: 347, p95_latency_ms: 812, total_tokens: 626400, error_rate: 0.032, traces_per_hour: 39 },
          };
        } else if (endpoint === "traces") {
          const qs = new URL(url, "http://localhost").searchParams;
          const limit = parseInt(qs.get("limit") ?? "20", 10);
          const offset = parseInt(qs.get("offset") ?? "0", 10);
          const NAMES = ["process_user_query", "fetch_context", "generate_response", "tool_call:search", "tool_call:read_file", "summarize_document", "classify_intent", "route_request"];
          const STATUSES = ["success", "success", "success", "success", "error", "timeout"] as const;
          const INPUTS = [
            "Analyze the latest quarterly report and summarize key risks.\n\n**Context:** We're presenting to the board on Friday. Focus on financial risks and any operational issues flagged by auditors. Keep it under 300 words.",
            "What were the top 5 support tickets last week?\n\n- Include ticket volume and resolution status\n- Flag anything still open or escalated\n- Compare to the previous week if possible",
            "Draft a follow-up email for the Acme Corp deal.\n\n**Deal context:**\n- Stage: Proposal sent 6 days ago\n- Contact: Marcus Chen, CTO\n- Key asks: custom SLA, migration support, enterprise pricing\n- Last touch: intro call on Thursday",
            "Review this pull request for security issues.\n\n```\nPR #4821 — feat: add user search endpoint\nFiles changed: 12\nAdditions: +847  Deletions: -23\n```\n\nFocus on: input validation, auth checks, and any data exposure risks.",
            "Generate a content brief for \"AI in healthcare\" targeting CTOs.\n\n**Goals:**\n1. Drive demo signups from health system leadership\n2. Address common objections (HIPAA, hallucination risk)\n3. Differentiate from generic AI tools",
          ];
          const OUTPUTS = [
            "## Q3 Risk Summary\n\nRevenue growth decelerated to **8% YoY** (down from 14% in Q2). Three key risk areas identified:\n\n1. **Customer churn** — NRR dropped to 104%, lowest since Q1 2024\n2. **Burn rate** — runway now at 14 months assuming flat ARR\n3. **Market concentration** — top 3 customers represent 41% of ARR\n\n> Recommend immediate review of enterprise retention playbook before Q4 QBRs.",
            "### Top 5 Support Tickets (Last 7 Days)\n\n| # | Issue | Volume | Status |\n|---|-------|--------|--------|\n| 1 | Login failures after SSO update | 142 | Resolved |\n| 2 | API rate limit errors on `/v2/ingest` | 87 | In progress |\n| 3 | Webhook delivery delays >30s | 54 | Monitoring |\n| 4 | CSV export encoding bug (UTF-8) | 31 | Fixed in 2.4.1 |\n| 5 | Mobile app crash on iOS 17.4 | 28 | Escalated |\n\nTotal tickets opened: **342** — up 18% WoW.",
            "Subject: Following up on Acme Corp proposal\n\nHi Marcus,\n\nWanted to circle back on the proposal we discussed last Thursday. A few things worth highlighting:\n\n- **Custom SLA** — we can offer 99.95% uptime with a 4-hour response window\n- **Migration support** — our team can handle the data migration at no extra cost\n- **Pricing** — happy to revisit the enterprise tier given your projected seat count\n\nLet me know if a 30-min call this week works.\n\nBest, Alex",
            "## Security Review — PR #4821\n\n### Critical\n- **SQL injection risk** on `/api/search` (line 142) — user input passed directly to query builder\n\n### High\n- Missing `Authorization` check on `DELETE /api/v1/users/:id`\n\n### Medium\n- `console.log` statements leak internal stack traces (lines 88, 203, 319)\n\n```ts\n// vulnerable\nconst results = db.query(`SELECT * FROM items WHERE name = '${req.query.q}'`);\n\n// fix\nconst results = db.query('SELECT * FROM items WHERE name = $1', [req.query.q]);\n```",
            "# Content Brief: AI in Healthcare\n\n**Primary keyword:** AI in healthcare\n**Secondary:** clinical AI, hospital automation\n**Target audience:** CTOs at health systems\n\n## Recommended Structure\n\n1. **Hook** — diagnostic error rate statistic\n2. **Problem** — manual process friction\n3. **Solution framing** — AI as infrastructure\n4. **3 use cases** — prior auth, clinical documentation, supply chain\n5. **Objections** — HIPAA, hallucination risk\n6. **CTA** — product demo or ROI calculator\n\n**Suggested word count:** 1,400–1,800",
          ];
          const total = 148;
          const traces = Array.from({ length: Math.min(limit, total - offset) }, (_, i) => {
            const idx = offset + i;
            const status = STATUSES[idx % STATUSES.length];
            return {
              trace_id: `tr-${String(idx + 1).padStart(4, "0")}`,
              name: NAMES[idx % NAMES.length],
              status,
              latency_ms: status === "timeout" ? 30000 : status === "error" ? Math.round(150 + Math.random() * 200) : Math.round(200 + Math.random() * 800),
              total_tokens: status === "error" ? undefined : Math.round(400 + Math.random() * 600),
              input: INPUTS[idx % INPUTS.length],
              output: status === "error" ? "Error: upstream tool returned 500" : status === "timeout" ? "Error: execution timed out after 30s" : OUTPUTS[idx % OUTPUTS.length],
              timestamp: new Date(now - (idx * 4 + Math.random() * 2) * 60 * 1000).toISOString(),
            };
          });
          body = { traces, total, limit, offset };
        }

        res.setHeader("Content-Type", "application/json");
        res.setHeader("Access-Control-Allow-Origin", "*");
        res.end(JSON.stringify(body));
      });
    },
  };
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const apiTarget = env.VITE_API_URL || "http://localhost:8080";
  const httpsConfig = getHttpsConfig();

  // Use local domain when HTTPS is configured (for same-site cookie sharing)
  const useLocalDomain = !!httpsConfig;

  return {
    plugins: [
      astroThemeColors(),
      tailwindcss(),
      !process.env.STORYBOOK && reactRouter(),
      env.VITE_MOCK_API === "true" && mockObservabilityPlugin(),
    ].filter(Boolean),
    // Workspace package `astro-trading-card` imports `uqr`; SSR must bundle them so
    // Node resolves from the Vite graph (avoids "Cannot find module 'uqr'" in dev).
    optimizeDeps: {
      include: ["uqr", "astro-trading-card"],
    },
    ssr: {
      noExternal: ["astro-trading-card", "uqr"],
    },
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      // When certs are present, bind to the local domain for same-site cookies
      host: useLocalDomain ? LOCAL_DOMAIN : "localhost",
      https: httpsConfig,
      proxy: {
        // Proxy API requests to the backend
        "/api": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
        // Proxy CLI binary download to the backend (Dev page links and curl use /download/*)
        "/download": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
        // Proxy CLI install script to the backend
        "/install": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
        // Proxy auth endpoints to the backend
        "/auth": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
      },
    },
  };
});
