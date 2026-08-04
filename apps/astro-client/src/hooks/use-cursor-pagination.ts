import { useCallback, useEffect, useState } from "react";

interface FetchNextPageResult<Page> {
  data?: { pages: Page[] };
}

export function useCursorPagination<Page>({
  pages,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  resetKey,
}: {
  pages: Page[];
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => Promise<FetchNextPageResult<Page>>;
  resetKey: string;
}) {
  const [requestedPage, setRequestedPage] = useState(1);

  useEffect(() => {
    setRequestedPage(1);
  }, [resetKey]);

  const loadedPageCount = pages.length;
  const currentPage = loadedPageCount === 0
    ? 1
    : Math.min(requestedPage, loadedPageCount);
  const totalPages = loadedPageCount + (hasNextPage ? 1 : 0);

  const onPageChange = useCallback(async (nextPage: number) => {
    if (nextPage < 1 || isFetchingNextPage) return;

    if (nextPage <= loadedPageCount) {
      setRequestedPage(nextPage);
      window.scrollTo({ top: 0, behavior: "smooth" });
      return;
    }

    if (nextPage !== loadedPageCount + 1 || !hasNextPage) return;
    const result = await fetchNextPage();
    if ((result.data?.pages.length ?? 0) >= nextPage) {
      setRequestedPage(nextPage);
      window.scrollTo({ top: 0, behavior: "smooth" });
    }
  }, [fetchNextPage, hasNextPage, isFetchingNextPage, loadedPageCount]);

  return {
    currentPage,
    page: pages[currentPage - 1],
    totalPages,
    onPageChange,
  };
}
