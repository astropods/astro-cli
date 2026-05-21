import { useEffect } from "react";
import { useSearchParams } from "react-router";

export function useCleanupOAuthParams(keys: readonly string[]) {
  const [searchParams, setSearchParams] = useSearchParams();
  const hasAny = keys.some((k) => searchParams.has(k));
  useEffect(() => {
    if (!hasAny) return;
    setSearchParams(
      (p) => {
        for (const k of keys) p.delete(k);
        return p;
      },
      { replace: true },
    );
  }, [hasAny, keys, setSearchParams]);
}
