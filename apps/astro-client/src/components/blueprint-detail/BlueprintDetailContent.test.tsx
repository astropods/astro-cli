import { describe, it, expect } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { BlueprintDetailContent } from "./BlueprintDetailContent";
import { getLinkedInShareHref, getXShareHref } from "@/lib/share-utils";
import { DISCORD_INVITE_URL } from "@/lib/constants";
import { afterEach } from "vitest";

afterEach(cleanup);

const baseProps = {
  account: "acme",
  name: "signal-watcher",
  categories: ["MONITORING"],
};

describe("BlueprintDetailContent", () => {
  it("renders full markdown content as provided", () => {
    const readme = [
      "# API Changelog Writer",
      "",
      "Small starter intro block that should be shown.",
      "",
      "## Quick start",
      "",
      "Run this command.",
    ].join("\n");

    renderWithProviders(
      <BlueprintDetailContent
        {...baseProps}
        readme={readme}
      />,
    );

    expect(screen.getByRole("heading", { name: /api changelog writer/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /quick start/i })).toBeInTheDocument();
    expect(screen.getByText(/should be shown/i)).toBeInTheDocument();
  });

  it("keeps content unchanged when only one heading exists", () => {
    const readme = [
      "# Quick start",
      "",
      "Only one section should remain visible.",
    ].join("\n");

    renderWithProviders(
      <BlueprintDetailContent
        {...baseProps}
        readme={readme}
      />,
    );

    expect(screen.getByRole("heading", { name: /quick start/i })).toBeInTheDocument();
    expect(screen.getByText(/Only one section should remain visible/i)).toBeInTheDocument();
  });

  it("keeps content unchanged when markdown has no headings", () => {
    const readme = "This readme has no markdown headings at all.";

    renderWithProviders(
      <BlueprintDetailContent
        {...baseProps}
        readme={readme}
      />,
    );

    expect(screen.getByText(readme)).toBeInTheDocument();
  });

  it("links draft setup support to Discord", () => {
    renderWithProviders(
      <BlueprintDetailContent
        {...baseProps}
        isDraft
      />,
    );

    const discordLink = screen.getByRole("link", { name: /join discord/i });
    expect(discordLink).toHaveAttribute("href", DISCORD_INVITE_URL);
    expect(screen.queryByRole("link", { name: /join slack/i })).not.toBeInTheDocument();
  });
});

describe("getLinkedInShareHref", () => {
  it("encodes the url, title, and summary into the shareArticle query", () => {
    const href = getLinkedInShareHref("https://astropods.com/acme/my-agent", "my-agent");
    const parsed = new URL(href);
    expect(parsed.origin + parsed.pathname).toBe("https://www.linkedin.com/shareArticle");
    expect(parsed.searchParams.get("mini")).toBe("true");
    expect(parsed.searchParams.get("url")).toBe("https://astropods.com/acme/my-agent");
    expect(parsed.searchParams.get("title")).toBe("Check out my-agent on Astro AI:\n\nhttps://astropods.com/acme/my-agent");
    expect(parsed.searchParams.get("summary")).toBe("Check out my-agent on Astro AI:\n\nhttps://astropods.com/acme/my-agent");
  });

  it("handles an empty url", () => {
    const href = getLinkedInShareHref("", "my-agent");
    const parsed = new URL(href);
    expect(parsed.searchParams.get("url")).toBe("");
  });
});

describe("getXShareHref", () => {
  it("encodes the prefilled tweet text with url on the second line", () => {
    const href = getXShareHref("https://astropods.com/acme/my-agent", "my-agent");
    const parsed = new URL(href);
    expect(parsed.origin + parsed.pathname).toBe("https://x.com/intent/tweet");
    expect(parsed.searchParams.get("text")).toBe(
      "Check out my-agent on Astro AI:\n\nhttps://astropods.com/acme/my-agent",
    );
  });

  it("handles an empty url", () => {
    const href = getXShareHref("", "my-agent");
    const parsed = new URL(href);
    expect(parsed.searchParams.get("text")).toBe("Check out my-agent on Astro AI:\n\n");
  });
});
