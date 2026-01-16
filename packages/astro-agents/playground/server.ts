import { exampleGithubAgent } from "../src/agents";
import type { AstroAgent } from "../src/agent";

// Registry of available agents
const AGENTS: Record<string, AstroAgent> = {
  "example-github": exampleGithubAgent,
};

const server = Bun.serve({
  port: 3001,
  idleTimeout: 0, // Disable idle timeout
  async fetch(req) {
    const url = new URL(req.url);

    // CORS headers
    const corsHeaders = {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type",
    };

    // Handle CORS preflight
    if (req.method === "OPTIONS") {
      return new Response(null, { headers: corsHeaders });
    }

    // List available agents
    if (url.pathname === "/api/agents" && req.method === "GET") {
      const agentList = Object.entries(AGENTS).map(([id, agent]) => ({
        id,
        // Access private _meta field
        title: (agent as unknown as { _meta: { title: string } })._meta.title,
        description: (agent as unknown as { _meta: { description: string } })
          ._meta.description,
      }));

      return new Response(JSON.stringify(agentList), {
        headers: { ...corsHeaders, "Content-Type": "application/json" },
      });
    }

    // Stream chat with an agent
    if (url.pathname === "/api/chat" && req.method === "POST") {
      const body = await req.json();
      const { agentId, prompt, threadId, userId, model } = body;

      const agent = AGENTS[agentId];
      if (!agent) {
        return new Response(JSON.stringify({ error: "Agent not found" }), {
          status: 404,
          headers: { ...corsHeaders, "Content-Type": "application/json" },
        });
      }

      // Create a streaming response
      const stream = new ReadableStream({
        async start(controller) {
          const encoder = new TextEncoder();

          const sendEvent = (type: string, data: unknown) => {
            controller.enqueue(
              encoder.encode(`data: ${JSON.stringify({ type, data })}\n\n`)
            );
          };

          try {
            await agent.stream({
              prompt,
              threadId,
              userId,
              model,
              onChunk: (chunk) => sendEvent("chunk", { text: chunk }),
              onStepStart: (step) => sendEvent("step-start", step),
              onStepEnd: (step) => sendEvent("step-end", step),
              onReasoningStart: () => sendEvent("reasoning-start", {}),
              onReasoningChunk: (chunk) =>
                sendEvent("reasoning-chunk", { text: chunk }),
              onReasoningEnd: () => sendEvent("reasoning-end", {}),
              onFinish: (result) => sendEvent("finish", { result }),
              onError: (error) =>
                sendEvent("error", { message: error.message }),
            });
          } catch (error) {
            sendEvent("error", {
              message: error instanceof Error ? error.message : "Unknown error",
            });
          }

          controller.close();
        },
      });

      return new Response(stream, {
        headers: {
          ...corsHeaders,
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          Connection: "keep-alive",
        },
      });
    }

    return new Response("Not found", { status: 404, headers: corsHeaders });
  },
});

console.log(`🚀 Astro Agents API running at http://localhost:${server.port}`);
