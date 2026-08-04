export interface UserResourceListParams {
  q?: string;
  limit?: number;
}

/** Canonical structural value used by visible-resource TanStack keys. */
export function userResourceListParamsKey(
  params: UserResourceListParams = {},
): UserResourceListParams {
  const q = params.q?.trim();
  return {
    ...(q ? { q } : {}),
    ...(params.limit != null ? { limit: params.limit } : {}),
  };
}
