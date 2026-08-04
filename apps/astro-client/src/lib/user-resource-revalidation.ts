interface UserResourceRevalidationArgs {
  currentUrl: URL;
  nextUrl: URL;
  defaultShouldRevalidate: boolean;
  formMethod?: string;
}

/**
 * User-resource list loaders prime the first SSR query only. Once mounted,
 * TanStack Query owns scope and search changes, so waiting for the route loader
 * would duplicate the request and delay controlled filter state from updating.
 */
export function shouldRevalidateUserResourceList({
  currentUrl,
  nextUrl,
  defaultShouldRevalidate,
  formMethod,
}: UserResourceRevalidationArgs): boolean {
  const isMutation = !!formMethod && formMethod.toLowerCase() !== "get";
  if (
    !isMutation &&
    currentUrl.pathname === nextUrl.pathname &&
    currentUrl.search !== nextUrl.search
  ) {
    return false;
  }
  return defaultShouldRevalidate;
}
