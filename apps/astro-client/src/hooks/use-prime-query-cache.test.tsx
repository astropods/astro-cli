import { describe, it, expect, vi, afterEach } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { usePrimeQueryCache } from "./use-prime-query-cache";

afterEach(cleanup);

// The whole point of usePrimeQueryCache is that setQueryData runs
// synchronously during render — BEFORE the page's useQuery hooks read the
// cache — so the first paint has real data and no client-side fetch fires.
describe("usePrimeQueryCache", () => {
  it("primes the cache synchronously so a sibling useQuery returns data on first render", () => {
    const qc = new QueryClient({
      // staleTime large enough that the primed value is treated as fresh and
      // useQuery skips the background fetch — same shape as the real app's
      // ACTIVITY_QUERY_OPTS / placeholderData configs.
      defaultOptions: { queries: { retry: false, staleTime: 60_000 } },
    });
    const queryFn = vi.fn().mockResolvedValue({ from: "fetch" });

    function Page() {
      // Prime cache via the hook…
      usePrimeQueryCache({ name: "primed" }, (client, data) => {
        client.setQueryData(["foo"], { from: data.name });
      });
      // …and read it in the same render. If the prime didn't happen
      // synchronously this useQuery would call queryFn and return undefined
      // for the first paint.
      const { data, isLoading } = useQuery({ queryKey: ["foo"], queryFn });
      return <span data-testid="state">{isLoading ? "loading" : data?.from ?? "<none>"}</span>;
    }

    render(
      <QueryClientProvider client={qc}>
        <Page />
      </QueryClientProvider>,
    );

    // Data from the cache is shown immediately, with no fetch.
    expect(screen.getByTestId("state")).toHaveTextContent("primed");
    expect(queryFn).not.toHaveBeenCalled();
  });

  it("re-runs the setup callback when loaderData changes (covers loader revalidation on org switch)", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const setup = vi.fn((client: QueryClient, data: { v: number }) => {
      client.setQueryData(["foo"], data.v);
    });

    function Page({ data }: { data: { v: number } }) {
      usePrimeQueryCache(data, setup);
      const { data: cached } = useQuery({ queryKey: ["foo"], queryFn: () => -1 });
      return <span data-testid="cached">{String(cached)}</span>;
    }

    const { rerender } = render(
      <QueryClientProvider client={qc}>
        <Page data={{ v: 1 }} />
      </QueryClientProvider>,
    );
    expect(screen.getByTestId("cached")).toHaveTextContent("1");
    expect(setup).toHaveBeenCalledTimes(1);

    rerender(
      <QueryClientProvider client={qc}>
        <Page data={{ v: 2 }} />
      </QueryClientProvider>,
    );
    expect(screen.getByTestId("cached")).toHaveTextContent("2");
    expect(setup).toHaveBeenCalledTimes(2);
  });

  it("does NOT re-run the setup callback when loaderData reference is unchanged", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const setup = vi.fn((client: QueryClient, data: { v: number }) => {
      client.setQueryData(["foo"], data.v);
    });

    const stable = { v: 1 };
    function Page({ unrelated }: { unrelated: number }) {
      usePrimeQueryCache(stable, setup);
      return <span data-testid="unrelated">{unrelated}</span>;
    }

    const { rerender } = render(
      <QueryClientProvider client={qc}>
        <Page unrelated={0} />
      </QueryClientProvider>,
    );
    expect(setup).toHaveBeenCalledTimes(1);

    // Re-render with a different prop, but the same `stable` reference.
    // useMemo's dep array should skip re-running setup.
    rerender(
      <QueryClientProvider client={qc}>
        <Page unrelated={1} />
      </QueryClientProvider>,
    );
    expect(setup).toHaveBeenCalledTimes(1);
  });
});
