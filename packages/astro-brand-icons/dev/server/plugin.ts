import type { Plugin } from "vite";
import {
  handleGetIcons,
  handleGetSvg,
  handleProcess,
  handleSaveIcon,
} from "./icons-api";
import { handleSource } from "./source-agent";

export function brandIconsApi(): Plugin {
  return {
    name: "brand-icons-api",
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        try {
          const url = req.url ?? "";
          const method = req.method ?? "GET";

          if (method === "GET" && url === "/api/icons") {
            return handleGetIcons(req, res);
          }

          const svgMatch = url.match(/^\/svg\/(light|dark)\/([a-z0-9-]+)\.svg(?:\?|$)/);
          if (method === "GET" && svgMatch) {
            return handleGetSvg(req, res, svgMatch[1] as "light" | "dark", svgMatch[2]!);
          }

          if (method === "POST" && url === "/api/source") {
            return handleSource(req, res);
          }

          if (method === "POST" && url === "/api/icons/save") {
            return handleSaveIcon(req, res);
          }

          if (method === "POST" && url === "/api/process") {
            return handleProcess(req, res);
          }

          next();
        } catch (e) {
          res.statusCode = 500;
          res.setHeader("Content-Type", "application/json");
          res.end(JSON.stringify({ error: String(e) }));
        }
      });
    },
  };
}
