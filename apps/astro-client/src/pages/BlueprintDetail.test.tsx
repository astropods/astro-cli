/**
 * Unit tests for BlueprintDetail loader and meta.
 *
 * Tests the server-side ogImage logic in isolation — no rendering required.
 * createServerApi is mocked so the loader never makes real HTTP calls.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { Blueprint } from "@/lib/api";

vi.mock("@/lib/api.server");

import { loader, meta } from "./BlueprintDetail";
import { createServerApi } from "@/lib/api.server";

// ─── Fixtures ─────────────────────────────────────────────────────────────────

const PUBLIC_BLUEPRINT: Blueprint = {
  account:    "acme",
  name:       "my-agent",
  visibility: "public",
  registry:   "reg.example.com",
  versions: [
    {
      build_id:     "abc123",
      spec:         {},
      agent_card:   { description: "A test agent" },
      published_at: "2025-01-01T00:00:00Z",
    },
  ],
};

// ─── Helpers ──────────────────────────────────────────────────────────────────

function makeApi(blueprint: Blueprint | null, throwBlueprint = false) {
  return {
    getBlueprint: throwBlueprint
      ? vi.fn().mockRejectedValue(new Error("not found"))
      : vi.fn().mockResolvedValue(blueprint),
    listBlueprints:  vi.fn().mockResolvedValue({ agents: [], count: 0 }),
    getAccount:      vi.fn().mockResolvedValue(null),
    getGitHubStatus: vi.fn().mockResolvedValue(null),
  };
}

function makeArgs(
  account  = "acme",
  agentSlug = "my-agent",
  origin   = "https://astro.example.com",
) {
  return {
    params:  { account, agentSlug },
    request: new Request(`${origin}/${account}/${agentSlug}`),
  };
}

// ─── loader — ogImage logic ───────────────────────────────────────────────────

describe("loader", () => {
  beforeEach(() => {
    vi.mocked(createServerApi).mockReset();
  });

  it("sets ogImage to the badge URL for a public blueprint with versions", async () => {
    vi.mocked(createServerApi).mockReturnValue(makeApi(PUBLIC_BLUEPRINT) as never);

    const data = await loader(makeArgs() as never);

    expect(data.ogImage).toBe(
      "https://astro.example.com/badge/agents/acme/my-agent.png",
    );
  });

  it("sets ogImage to null for a private blueprint", async () => {
    const privateBp: Blueprint = { ...PUBLIC_BLUEPRINT, visibility: "private" };
    vi.mocked(createServerApi).mockReturnValue(makeApi(privateBp) as never);

    const data = await loader(makeArgs() as never);

    expect(data.ogImage).toBeNull();
  });

  it("sets ogImage to null when the blueprint has no published versions", async () => {
    const draftBp: Blueprint = { ...PUBLIC_BLUEPRINT, versions: [] };
    vi.mocked(createServerApi).mockReturnValue(makeApi(draftBp) as never);

    const data = await loader(makeArgs() as never);

    expect(data.ogImage).toBeNull();
  });

  it("sets ogImage to null when the blueprint cannot be fetched", async () => {
    vi.mocked(createServerApi).mockReturnValue(makeApi(null, true) as never);

    const data = await loader(makeArgs() as never);

    expect(data.ogImage).toBeNull();
  });
});

// ─── meta — OG tag generation ─────────────────────────────────────────────────

describe("meta", () => {
  const ORIGIN   = "https://astro.example.com";
  const OG_IMAGE = `${ORIGIN}/badge/agents/acme/my-agent.png`;

  function makeData(overrides: {
    blueprint?: Blueprint | null;
    ogImage?:   string | null;
  } = {}) {
    return {
      blueprint:       "blueprint" in overrides ? overrides.blueprint : PUBLIC_BLUEPRINT,
      canonicalUrl:    `${ORIGIN}/acme/my-agent`,
      ogImage:         "ogImage"   in overrides ? overrides.ogImage   : OG_IMAGE,
      isPublic:        true,
      blueprintsData:  { agents: [], count: 0 },
      accountData:     null,
      accountsMap:     {},
      githubStatus:    null,
    };
  }

  it("includes og:image, og:image:width, og:image:height, and twitter:card=summary_large_image when ogImage is set", () => {
    const tags = meta({ data: makeData() } as never);

    expect(tags.some((t: Record<string, unknown>) => t.property === "og:image" && t.content === OG_IMAGE)).toBe(true);
    expect(tags.some((t: Record<string, unknown>) => t.property === "og:image:width"  && t.content === "1200")).toBe(true);
    expect(tags.some((t: Record<string, unknown>) => t.property === "og:image:height" && t.content === "628")).toBe(true);
    expect(tags.some((t: Record<string, unknown>) => t.name === "twitter:card" && t.content === "summary_large_image")).toBe(true);
    expect(tags.some((t: Record<string, unknown>) => t.name === "twitter:image" && t.content === OG_IMAGE)).toBe(true);
  });

  it("omits og:image tags and uses twitter:card=summary when ogImage is null", () => {
    const tags = meta({ data: makeData({ ogImage: null }) } as never);

    expect(tags.some((t: Record<string, unknown>) => t.property === "og:image")).toBe(false);
    expect(tags.some((t: Record<string, unknown>) => t.name === "twitter:card" && t.content === "summary")).toBe(true);
  });

  it("returns a fallback title when the blueprint is null", () => {
    const tags = meta({ data: makeData({ blueprint: null, ogImage: null }) } as never);

    expect(tags.some((t: Record<string, unknown>) => "title" in t && t.title === "Agent Details | Astro")).toBe(true);
    expect(tags.some((t: Record<string, unknown>) => t.property === "og:image")).toBe(false);
  });
});
