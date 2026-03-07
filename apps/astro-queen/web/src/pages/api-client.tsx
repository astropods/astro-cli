import { useState, useMemo, useCallback } from "react";
import {
  useAstroOpenAPISpec,
  startDeviceAuth,
  pollDeviceAuth,
} from "@/api/admin";
import { astroProxyFetch } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { Play, ChevronRight, Search, Lock, Key } from "lucide-react";

// --- Types ---

interface OpenAPIParam {
  name: string;
  in: "path" | "query" | "header";
  required?: boolean;
  description?: string;
  schema?: { type?: string };
}

interface OpenAPIOperation {
  summary?: string;
  description?: string;
  tags?: string[];
  parameters?: OpenAPIParam[];
  requestBody?: {
    required?: boolean;
    content?: {
      "application/json"?: {
        schema?: unknown;
      };
    };
  };
  security?: Array<Record<string, string[]>>;
  deprecated?: boolean;
  responses?: Record<
    string,
    { description?: string; content?: Record<string, { schema?: unknown }> }
  >;
}

interface Endpoint {
  method: string;
  path: string;
  op: OpenAPIOperation;
  tag: string;
}

interface ApiResponse {
  status: number;
  headers: Record<string, string>;
  body: string;
  durationMs: number;
}

// --- Helpers ---

const METHOD_COLORS: Record<string, string> = {
  GET: "bg-emerald-500/20 text-emerald-400 border-emerald-500/30",
  POST: "bg-amber-500/20 text-amber-400 border-amber-500/30",
  PUT: "bg-blue-500/20 text-blue-400 border-blue-500/30",
  DELETE: "bg-red-500/20 text-red-400 border-red-500/30",
  PATCH: "bg-purple-500/20 text-purple-400 border-purple-500/30",
};

function MethodBadge({
  method,
  size = "sm",
}: {
  method: string;
  size?: "xs" | "sm";
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center justify-center rounded border font-mono font-bold uppercase",
        METHOD_COLORS[method] ?? "bg-gray-500/20 text-gray-400 border-gray-500/30",
        size === "xs"
          ? "px-1 py-0 text-[8px] min-w-[32px]"
          : "px-1.5 py-0.5 text-[10px] min-w-[42px]"
      )}
    >
      {method}
    </span>
  );
}

function parseEndpoints(spec: Record<string, unknown>): Endpoint[] {
  const paths = spec.paths as Record<
    string,
    Record<string, OpenAPIOperation>
  > | undefined;
  if (!paths) return [];

  const endpoints: Endpoint[] = [];
  for (const [path, methods] of Object.entries(paths)) {
    for (const [method, op] of Object.entries(methods)) {
      if (["get", "post", "put", "delete", "patch", "head"].includes(method)) {
        endpoints.push({
          method: method.toUpperCase(),
          path,
          op,
          tag: op.tags?.[0] ?? "Other",
        });
      }
    }
  }
  return endpoints;
}

function groupByTag(endpoints: Endpoint[]): Map<string, Endpoint[]> {
  const grouped = new Map<string, Endpoint[]>();
  for (const ep of endpoints) {
    const existing = grouped.get(ep.tag);
    if (existing) existing.push(ep);
    else grouped.set(ep.tag, [ep]);
  }
  return grouped;
}

function extractPathParams(path: string): string[] {
  return [...path.matchAll(/\{(\w+)\}/g)].map((m) => m[1]);
}

function buildUrl(
  path: string,
  pathParams: Record<string, string>,
  queryParams: Record<string, string>
): string {
  let url = path;
  for (const [key, value] of Object.entries(pathParams)) {
    url = url.replace(`{${key}}`, encodeURIComponent(value));
  }
  const qs = Object.entries(queryParams)
    .filter(([, v]) => v)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join("&");
  if (qs) url += `?${qs}`;
  return url;
}

function formatJson(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function statusColor(status: number): string {
  if (status < 300) return "text-emerald-400";
  if (status < 400) return "text-amber-400";
  return "text-red-400";
}

// --- Components ---

function EndpointSidebar({
  grouped,
  selected,
  onSelect,
  filter,
  onFilterChange,
}: {
  grouped: Map<string, Endpoint[]>;
  selected: Endpoint | null;
  onSelect: (ep: Endpoint) => void;
  filter: string;
  onFilterChange: (v: string) => void;
}) {
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const toggleTag = (tag: string) =>
    setCollapsed((s) => ({ ...s, [tag]: !s[tag] }));

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="shrink-0 relative px-2 pb-2">
        <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground" />
        <Input
          value={filter}
          onChange={(e) => onFilterChange(e.target.value)}
          placeholder="Filter endpoints..."
          className="h-7 pl-7 text-[11px]"
        />
      </div>
      <div className="flex-1 overflow-y-auto px-1">
        {[...grouped.entries()].map(([tag, endpoints]) => (
          <div key={tag} className="mb-1">
            <button
              onClick={() => toggleTag(tag)}
              className="flex w-full items-center gap-1 px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground hover:text-foreground"
            >
              <ChevronRight
                className={cn(
                  "size-2.5 transition-transform",
                  !collapsed[tag] && "rotate-90"
                )}
              />
              {tag}
              <span className="ml-auto text-[9px] font-normal opacity-60">
                {endpoints.length}
              </span>
            </button>
            {!collapsed[tag] && (
              <div className="ml-2 space-y-px">
                {endpoints.map((ep) => {
                  const isSelected =
                    selected?.method === ep.method &&
                    selected?.path === ep.path;
                  return (
                    <button
                      key={`${ep.method}-${ep.path}`}
                      onClick={() => onSelect(ep)}
                      className={cn(
                        "flex w-full items-center gap-1.5 rounded px-1.5 py-[3px] text-[11px] transition-colors text-left",
                        isSelected
                          ? "bg-pollen/80 text-honey-dark"
                          : "text-muted-foreground hover:bg-glass-light hover:text-foreground",
                        ep.op.deprecated && "line-through opacity-60"
                      )}
                    >
                      <MethodBadge method={ep.method} size="xs" />
                      <span className="truncate flex-1">{ep.path}</span>
                      {ep.op.security && ep.op.security.length > 0 && (
                        <Lock className="size-2.5 shrink-0 opacity-40" />
                      )}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

function RequestPanel({
  endpoint,
  onResponse,
  sharedToken,
}: {
  endpoint: Endpoint;
  onResponse: (r: ApiResponse) => void;
  sharedToken: string;
}) {
  const pathParamNames = extractPathParams(endpoint.path);
  const queryParamDefs =
    endpoint.op.parameters?.filter((p) => p.in === "query") ?? [];
  const hasBody =
    !!endpoint.op.requestBody &&
    endpoint.method !== "GET" &&
    endpoint.method !== "HEAD";
  const requiresAuth =
    endpoint.op.security && endpoint.op.security.length > 0;

  const [pathParams, setPathParams] = useState<Record<string, string>>({});
  const [queryParams, setQueryParams] = useState<Record<string, string>>({});
  const [bodyText, setBodyText] = useState("{}");
  const [bearerToken, setBearerToken] = useState(sharedToken);
  const [sending, setSending] = useState(false);

  const handleSend = useCallback(async () => {
    setSending(true);
    const url = buildUrl(endpoint.path, pathParams, queryParams);
    const headers: Record<string, string> = {};
    if (bearerToken) {
      headers["Authorization"] = `Bearer ${bearerToken}`;
    }
    const start = performance.now();
    try {
      const resp = await astroProxyFetch(
        endpoint.method,
        url,
        headers,
        hasBody ? bodyText : undefined
      );
      onResponse({ ...resp, durationMs: performance.now() - start });
    } catch (err) {
      onResponse({
        status: 0,
        headers: {},
        body: String(err),
        durationMs: performance.now() - start,
      });
    } finally {
      setSending(false);
    }
  }, [endpoint, pathParams, queryParams, bodyText, bearerToken, hasBody, onResponse]);

  return (
    <div className="space-y-3">
      {/* URL bar */}
      <div className="flex items-center gap-2">
        <MethodBadge method={endpoint.method} />
        <code className="flex-1 text-[11px] text-muted-foreground truncate">
          {buildUrl(endpoint.path, pathParams, queryParams)}
        </code>
        <Button
          size="sm"
          onClick={handleSend}
          disabled={sending}
          className="h-7 gap-1 text-[11px]"
        >
          <Play className="size-3" />
          {sending ? "Sending..." : "Send"}
        </Button>
      </div>

      {endpoint.op.summary && (
        <p className="text-[11px] text-muted-foreground">
          {endpoint.op.summary}
        </p>
      )}

      {/* Auth */}
      {requiresAuth && !sharedToken && (
        <div>
          <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider flex items-center gap-1">
            <Lock className="size-2.5" /> Bearer Token
          </label>
          <Input
            value={bearerToken}
            onChange={(e) => setBearerToken(e.target.value)}
            placeholder="Paste your JWT token or use Login above"
            className="mt-1 h-7 font-mono text-[11px]"
            type="password"
          />
        </div>
      )}
      {requiresAuth && sharedToken && (
        <p className="text-[10px] text-muted-foreground flex items-center gap-1">
          <Lock className="size-2.5" /> Using token from login
        </p>
      )}

      {/* Path params */}
      {pathParamNames.length > 0 && (
        <div>
          <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
            Path Parameters
          </label>
          <div className="mt-1 space-y-1">
            {pathParamNames.map((name) => {
              const paramDef = endpoint.op.parameters?.find(
                (p) => p.name === name && p.in === "path"
              );
              return (
                <div key={name} className="flex items-center gap-2">
                  <code className="text-[11px] text-muted-foreground min-w-[80px]">
                    {name}
                    {paramDef?.required !== false && (
                      <span className="text-red-400">*</span>
                    )}
                  </code>
                  <Input
                    value={pathParams[name] ?? ""}
                    onChange={(e) =>
                      setPathParams((s) => ({ ...s, [name]: e.target.value }))
                    }
                    placeholder={paramDef?.description ?? name}
                    className="h-7 flex-1 font-mono text-[11px]"
                  />
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Query params */}
      {queryParamDefs.length > 0 && (
        <div>
          <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
            Query Parameters
          </label>
          <div className="mt-1 space-y-1">
            {queryParamDefs.map((p) => (
              <div key={p.name} className="flex items-center gap-2">
                <code className="text-[11px] text-muted-foreground min-w-[80px]">
                  {p.name}
                  {p.required && <span className="text-red-400">*</span>}
                </code>
                <Input
                  value={queryParams[p.name] ?? ""}
                  onChange={(e) =>
                    setQueryParams((s) => ({
                      ...s,
                      [p.name]: e.target.value,
                    }))
                  }
                  placeholder={p.description ?? p.name}
                  className="h-7 flex-1 font-mono text-[11px]"
                />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Request body */}
      {hasBody && (
        <div>
          <label className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
            Request Body
          </label>
          <textarea
            value={bodyText}
            onChange={(e) => setBodyText(e.target.value)}
            className="mt-1 w-full rounded-md border border-input bg-transparent p-2 font-mono text-[11px] text-foreground outline-none focus:border-ring min-h-[120px] resize-y"
            spellCheck={false}
          />
        </div>
      )}
    </div>
  );
}

function ResponsePanel({ response }: { response: ApiResponse }) {
  const [tab, setTab] = useState<"body" | "headers">("body");
  const formatted = useMemo(() => formatJson(response.body), [response.body]);

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-3 mb-2">
        <span
          className={cn("text-sm font-bold", statusColor(response.status))}
        >
          {response.status || "Error"}
        </span>
        <span className="text-[10px] text-muted-foreground">
          {response.durationMs.toFixed(0)}ms
        </span>
        <div className="flex gap-1 ml-auto">
          {(["body", "headers"] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={cn(
                "px-2 py-0.5 rounded text-[10px] font-medium capitalize transition-colors",
                tab === t
                  ? "bg-pollen/80 text-honey-dark"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              {t}
            </button>
          ))}
        </div>
      </div>
      <div className="flex-1 overflow-auto rounded-md border border-glass-border-honey glass p-2">
        {tab === "body" ? (
          <pre className="text-[11px] font-mono whitespace-pre-wrap break-all">
            {formatted}
          </pre>
        ) : (
          <div className="space-y-0.5">
            {Object.entries(response.headers).map(([k, v]) => (
              <div key={k} className="text-[11px] font-mono">
                <span className="text-muted-foreground">{k}:</span> {v}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// --- Main Page ---

export function ApiClientPage() {
  const { data: spec, isLoading, error } = useAstroOpenAPISpec();
  const [selected, setSelected] = useState<Endpoint | null>(null);
  const [filter, setFilter] = useState("");
  const [response, setResponse] = useState<ApiResponse | null>(null);
  const [bearerToken, setBearerToken] = useState("");
  const [loginState, setLoginState] = useState<
    | { status: "idle" }
    | { status: "waiting"; userCode: string }
    | { status: "error"; message: string }
  >({ status: "idle" });

  const endpoints = useMemo(
    () => (spec ? parseEndpoints(spec) : []),
    [spec]
  );

  const filtered = useMemo(() => {
    if (!filter) return endpoints;
    const lower = filter.toLowerCase();
    return endpoints.filter(
      (ep) =>
        ep.path.toLowerCase().includes(lower) ||
        ep.method.toLowerCase().includes(lower) ||
        ep.tag.toLowerCase().includes(lower) ||
        ep.op.summary?.toLowerCase().includes(lower)
    );
  }, [endpoints, filter]);

  const grouped = useMemo(() => groupByTag(filtered), [filtered]);

  const handleLogin = useCallback(async () => {
    try {
      setLoginState({ status: "waiting", userCode: "..." });
      const auth = await startDeviceAuth();
      setLoginState({ status: "waiting", userCode: auth.user_code });

      // Open browser to verification URL
      const a = document.createElement("a");
      a.href = auth.verification_uri_complete;
      a.target = "_blank";
      a.rel = "noopener noreferrer";
      a.click();

      // Poll for completion
      const interval = (auth.interval || 5) * 1000;
      const deadline = Date.now() + (auth.expires_in || 300) * 1000;

      while (Date.now() < deadline) {
        await new Promise((r) => setTimeout(r, interval));
        const result = await pollDeviceAuth(auth.device_code);
        if (result.status === "complete" && result.access_token) {
          setBearerToken(result.access_token);
          setLoginState({ status: "idle" });
          return;
        }
        if (
          result.status === "access_denied" ||
          result.status === "expired_token"
        ) {
          setLoginState({
            status: "error",
            message: result.error || result.status,
          });
          return;
        }
        // authorization_pending / slow_down → keep polling
      }
      setLoginState({ status: "error", message: "Login timed out" });
    } catch (err) {
      setLoginState({ status: "error", message: String(err) });
    }
  }, []);

  const handleSelect = useCallback((ep: Endpoint) => {
    setSelected(ep);
    setResponse(null);
  }, []);

  if (isLoading) {
    return (
      <div>
        <h2 className="mb-4 text-xl font-semibold">API Client</h2>
        <div className="space-y-2">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="h-8 w-full" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <h2 className="mb-4 text-xl font-semibold">API Client</h2>
        <p className="text-destructive">
          Failed to load OpenAPI spec: {error.message}
        </p>
      </div>
    );
  }

  return (
    <div className="flex h-[calc(100vh-3rem)] gap-3">
      {/* Left: Endpoint list */}
      <div className="w-96 shrink-0 flex flex-col glass rounded-lg py-2 overflow-hidden">
        <div className="shrink-0 px-2 pb-1 space-y-1">
          <div className="flex items-center justify-between">
            <h2 className="text-xs font-semibold">API Client</h2>
            <span className="text-[9px] text-muted-foreground">
              {endpoints.length} endpoints
            </span>
          </div>
          <Button
            size="sm"
            variant={bearerToken ? "outline" : "default"}
            onClick={handleLogin}
            disabled={loginState.status === "waiting"}
            className="w-full h-6 gap-1 text-[10px]"
          >
            <Key className="size-2.5" />
            {loginState.status === "waiting"
              ? `Enter code: ${loginState.userCode}`
              : bearerToken
                ? "Logged in"
                : "Login"}
          </Button>
          {loginState.status === "error" && (
            <p className="text-[9px] text-destructive truncate" title={loginState.message}>
              {loginState.message}
            </p>
          )}
        </div>
        <EndpointSidebar
          grouped={grouped}
          selected={selected}
          onSelect={handleSelect}
          filter={filter}
          onFilterChange={setFilter}
        />
      </div>

      {/* Right: Request + Response */}
      <div className="flex-1 flex flex-col gap-3 min-w-0">
        {selected ? (
          <>
            <div className="glass rounded-lg p-3">
              <RequestPanel
                key={`${selected.method}-${selected.path}`}
                endpoint={selected}
                onResponse={setResponse}
                sharedToken={bearerToken}
              />
            </div>
            {response && (
              <div className="glass rounded-lg p-3 flex-1 min-h-0">
                <ResponsePanel response={response} />
              </div>
            )}
          </>
        ) : (
          <div className="flex flex-1 items-center justify-center text-muted-foreground text-sm">
            Select an endpoint from the sidebar to get started
          </div>
        )}
      </div>
    </div>
  );
}
