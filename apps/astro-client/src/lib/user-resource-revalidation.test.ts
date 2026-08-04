import { describe, expect, it } from "vitest";
import { shouldRevalidateUserResourceList } from "./user-resource-revalidation";

function args(
  current: string,
  next: string,
  defaultShouldRevalidate = true,
  formMethod?: string,
) {
  return {
    currentUrl: new URL(current),
    nextUrl: new URL(next),
    defaultShouldRevalidate,
    formMethod,
  };
}

describe("shouldRevalidateUserResourceList", () => {
  it("skips the route loader when list query parameters change", () => {
    expect(shouldRevalidateUserResourceList(
      args("http://astro/agents", "http://astro/agents?account=team"),
    )).toBe(false);
    expect(shouldRevalidateUserResourceList(
      args("http://astro/agents?account=team", "http://astro/agents?scope=all"),
    )).toBe(false);
  });

  it("preserves programmatic revalidation of the current URL", () => {
    expect(shouldRevalidateUserResourceList(
      args("http://astro/agents?account=team", "http://astro/agents?account=team"),
    )).toBe(true);
  });

  it("defers cross-page navigation to React Router", () => {
    expect(shouldRevalidateUserResourceList(
      args("http://astro/blueprints", "http://astro/agents", true),
    )).toBe(true);
    expect(shouldRevalidateUserResourceList(
      args("http://astro/blueprints", "http://astro/agents", false),
    )).toBe(false);
  });

  it("does not suppress mutation-driven revalidation", () => {
    expect(shouldRevalidateUserResourceList(
      args("http://astro/agents", "http://astro/agents?account=team", true, "POST"),
    )).toBe(true);
  });
});
