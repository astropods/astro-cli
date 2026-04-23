import { ApiClient } from "./api";

export function createServerApi(request: Request): ApiClient {
  const apiUrl = process.env.API_URL || "http://localhost:8080";
  const cookie = request.headers.get("cookie") || "";
  return new ApiClient(apiUrl, apiUrl, { cookie });
}

export async function getPersonalAccount(request: Request) {
  try {
    const api = createServerApi(request);
    const auth = await api.getCurrentUser();
    const account = auth.accounts?.find((a) => a.type === "personal");
    return account ? { api, accountName: account.name } : null;
  } catch {
    return null;
  }
}
