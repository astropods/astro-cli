/** Query params for GET /api/v1/agents/:account (blueprints list). */
export interface BlueprintListParams {
  q?: string;
  tag?: string;
  visibility?: 'public' | 'private';
  sort?: 'name' | 'newest';
  limit?: number;
  offset?: number;
}

export const BLUEPRINT_LIST_DEFAULT_PAGE_SIZE = 50;
export const BLUEPRINT_LIST_MAX_LIMIT = 100;

function appendParam(params: URLSearchParams, key: string, value: string | undefined) {
  const trimmed = value?.trim();
  if (trimmed) {
    params.set(key, trimmed);
  }
}

/** Serializes blueprint list params (no leading `?`). */
export function buildBlueprintListQuery(params?: BlueprintListParams): string {
  if (!params) {
    return '';
  }
  const search = new URLSearchParams();
  appendParam(search, 'q', params.q);
  appendParam(search, 'tag', params.tag);
  if (params.visibility) {
    search.set('visibility', params.visibility);
  }
  if (params.sort) {
    search.set('sort', params.sort);
  }
  if (params.limit != null) {
    search.set('limit', String(params.limit));
  }
  if (params.offset != null) {
    search.set('offset', String(params.offset));
  }
  return search.toString();
}

/** Stable params object for TanStack Query keys. */
export function blueprintListParamsKey(params?: BlueprintListParams): BlueprintListParams {
  if (!params) {
    return {};
  }
  const key: BlueprintListParams = {};
  if (params.q?.trim()) {
    key.q = params.q.trim();
  }
  if (params.tag?.trim()) {
    key.tag = params.tag.trim();
  }
  if (params.visibility) {
    key.visibility = params.visibility;
  }
  if (params.sort) {
    key.sort = params.sort;
  }
  if (params.limit != null) {
    key.limit = params.limit;
  }
  if (params.offset != null) {
    key.offset = params.offset;
  }
  return key;
}

/** True when any filter (not pagination) is active. */
export function hasBlueprintListFilters(params?: BlueprintListParams): boolean {
  const key = blueprintListParamsKey(params);
  return !!(key.q || key.tag || key.visibility || key.sort);
}
