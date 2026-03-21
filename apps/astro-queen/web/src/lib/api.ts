class APIError extends Error {
  constructor(
    public status: number,
    public body: string
  ) {
    super(`${status}: ${body}`);
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  const opts: RequestInit = {
    method,
    headers: { "Content-Type": "application/json" },
  };
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new APIError(res.status, text);
  }
  if (res.status === 204 || res.headers.get("content-length") === "0") {
    return undefined as T;
  }
  return res.json();
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  put: <T>(path: string, body?: unknown) => request<T>("PUT", path, body),
  patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
  del: (path: string) => request<void>("DELETE", path),
};

// Raw fetch through the astro proxy — returns full response details.
export async function astroProxyFetch(
  method: string,
  path: string,
  headers?: Record<string, string>,
  body?: string
): Promise<{ status: number; headers: Record<string, string>; body: string }> {
  const proxyPath = `/api/astro${path}`;
  const opts: RequestInit = {
    method,
    headers: { "Content-Type": "application/json", ...headers },
  };
  if (body && method !== "GET" && method !== "HEAD") {
    opts.body = body;
  }
  const res = await fetch(proxyPath, opts);
  const text = await res.text();
  const respHeaders: Record<string, string> = {};
  res.headers.forEach((v, k) => {
    respHeaders[k] = v;
  });
  return { status: res.status, headers: respHeaders, body: text };
}

export { APIError };
