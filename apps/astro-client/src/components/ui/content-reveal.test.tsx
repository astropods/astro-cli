import { render, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  ContentReveal,
  ResultSetReveal,
  SettledContentReveal,
} from "./content-reveal";
import { shouldRevealContent } from "./content-reveal-motion";

function Harness({ selection }: { selection: string }) {
  return (
    <ContentReveal key={selection}>
      <span>Filtered results</span>
    </ContentReveal>
  );
}

function ResultSetHarness({
  itemCount,
  settled = true,
  transitionKey,
}: {
  itemCount: number;
  settled?: boolean;
  transitionKey?: string;
}) {
  return (
    <ResultSetReveal
      itemCount={itemCount}
      settled={settled}
      transitionKey={transitionKey}
    >
      <span>{itemCount} filtered results</span>
    </ResultSetReveal>
  );
}

function SettledHarness({
  account,
  settled,
  children,
}: {
  account: string;
  settled: boolean;
  children: string;
}) {
  return (
    <SettledContentReveal transitionKey={account} settled={settled}>
      {children}
    </SettledContentReveal>
  );
}

describe("ContentReveal", () => {
  it("uses the agent-detail entrance motion", () => {
    const { container } = render(<Harness selection="all" />);
    const reveal = container.querySelector("[data-slot='content-reveal']");

    expect(reveal).toHaveStyle({
      opacity: "0",
      transform: "translateY(12px)",
    });
  });

  it("remounts when its selection key changes", () => {
    const { container, rerender } = render(<Harness selection="all" />);
    const firstReveal = container.querySelector("[data-slot='content-reveal']");

    rerender(<Harness selection="team-a" />);

    const nextReveal = container.querySelector("[data-slot='content-reveal']");
    expect(nextReveal).not.toBe(firstReveal);
  });
});

describe("ResultSetReveal", () => {
  it("keeps decreasing result sets static", async () => {
    const { container, rerender } = render(<ResultSetHarness itemCount={2} />);
    const reveal = container.querySelector("[data-slot='result-set-reveal']");

    await waitFor(() => expect(reveal).toHaveStyle({ opacity: "1" }));
    rerender(<ResultSetHarness itemCount={0} />);

    expect(container.querySelector("[data-slot='result-set-reveal']")).toBe(
      reveal,
    );
    expect(reveal).toHaveStyle({ opacity: "1" });
  });

  it("reveals settled initial content and non-empty account transitions", () => {
    expect(
      shouldRevealContent(null, { itemCount: 0, settled: false }),
    ).toBe(false);
    expect(
      shouldRevealContent(null, { itemCount: 2, settled: true }),
    ).toBe(true);
    expect(
      shouldRevealContent(
        { itemCount: 0, settled: false, transitionKey: "" },
        { itemCount: 2, settled: true, transitionKey: "" },
      ),
    ).toBe(true);
    expect(
      shouldRevealContent(
        { itemCount: 0, settled: true, transitionKey: "" },
        { itemCount: 2, settled: true, transitionKey: "" },
      ),
    ).toBe(false);
    expect(
      shouldRevealContent(
        { itemCount: 2, settled: true, transitionKey: "" },
        { itemCount: 0, settled: true, transitionKey: "team-a" },
      ),
    ).toBe(false);
    expect(
      shouldRevealContent(
        { itemCount: 2, settled: true, transitionKey: "" },
        { itemCount: 1, settled: true, transitionKey: "team-a" },
      ),
    ).toBe(true);
    expect(
      shouldRevealContent(
        { itemCount: 2, settled: true, transitionKey: "team-a" },
        { itemCount: 2, settled: true, transitionKey: "team-a" },
      ),
    ).toBe(false);
    expect(
      shouldRevealContent(
        { itemCount: 2, settled: true, transitionKey: "team-a" },
        { itemCount: 3, settled: true, transitionKey: "team-a" },
      ),
    ).toBe(false);
    expect(
      shouldRevealContent(
        { itemCount: 1, settled: true, transitionKey: "team-a" },
        { itemCount: 3, settled: true, transitionKey: "" },
      ),
    ).toBe(true);
  });
});

describe("SettledContentReveal", () => {
  it("holds placeholder content steady and reveals the settled account once", () => {
    const { container, rerender } = render(
      <SettledHarness account="account-a" settled>
        Account A
      </SettledHarness>,
    );
    const accountAReveal = container.querySelector(
      "[data-slot='settled-content-reveal']",
    );

    rerender(
      <SettledHarness account="account-b" settled={false}>
        Account A
      </SettledHarness>,
    );
    expect(
      container.querySelector("[data-slot='settled-content-reveal']"),
    ).toBe(accountAReveal);

    rerender(
      <SettledHarness account="account-b" settled>
        Account B
      </SettledHarness>,
    );
    expect(
      container.querySelector("[data-slot='settled-content-reveal']"),
    ).not.toBe(accountAReveal);
  });

  it("keeps an initial loading state static until data settles", () => {
    const { container, rerender } = render(
      <SettledHarness account="account-a" settled={false}>
        Loading
      </SettledHarness>,
    );
    expect(
      container.querySelector("[data-slot='settled-content-pending']"),
    ).toBeInTheDocument();

    rerender(
      <SettledHarness account="account-a" settled>
        Account A
      </SettledHarness>,
    );
    expect(
      container.querySelector("[data-slot='settled-content-reveal']"),
    ).toBeInTheDocument();
  });
});
