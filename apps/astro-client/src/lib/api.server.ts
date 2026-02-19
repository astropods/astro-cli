import { ApiClient } from "./api";

export function createServerApi(request: Request): ApiClient {
  const apiUrl = process.env.API_URL || "http://localhost:8080";
  const cookie = request.headers.get("cookie") || "";
  return new ApiClient(apiUrl, apiUrl, { cookie });
}
