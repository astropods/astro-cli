import { createContext, useContext } from "react";
import { api, ApiClient } from "./api";

const ApiClientContext = createContext<ApiClient>(api);

export const ApiClientProvider = ApiClientContext.Provider;

export function useApiClient(): ApiClient {
  return useContext(ApiClientContext);
}
