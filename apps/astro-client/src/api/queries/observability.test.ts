import { describe, expect, it } from "vitest";
import { observabilityKeys } from "./keys";

describe("observabilityKeys.insights", () => {
  const params = { q: "acme" };

  // The SSR loader primes the cache without knowing the experiment state
  // (localStorage isn't readable server-side), so it always primes a v1 key.
  // v1 keys therefore have to stay byte-identical to what they were before the
  // experiment existed, or the primed entry silently stops being a cache hit.
  it("leaves v1 keys unchanged", () => {
    expect(observabilityKeys.insights("acme", params, "v1")).toEqual([
      "observability",
      "insights",
      "acme",
      params,
    ]);
    expect(observabilityKeys.insights("acme", params)).toEqual(
      observabilityKeys.insights("acme", params, "v1"),
    );
    expect(observabilityKeys.insights("acme")).toEqual(["observability", "insights", "acme"]);
  });

  // Without the version in the key, toggling the experiment would re-render the
  // other read path's cached response — the two would look identical because
  // they're wire-compatible, which is exactly what makes the bug hard to spot.
  it("distinguishes v2 from v1", () => {
    const v1 = observabilityKeys.insights("acme", params, "v1");
    const v2 = observabilityKeys.insights("acme", params, "v2");
    expect(v2).not.toEqual(v1);
    expect(v2).toEqual([...v1, "v2"]);
  });

  it("distinguishes v2 from v1 without params too", () => {
    expect(observabilityKeys.insights("acme", undefined, "v2")).toEqual([
      "observability",
      "insights",
      "acme",
      "v2",
    ]);
  });

  it("still separates accounts and params within a version", () => {
    expect(observabilityKeys.insights("acme", params, "v2")).not.toEqual(
      observabilityKeys.insights("other", params, "v2"),
    );
    expect(observabilityKeys.insights("acme", { q: "a" }, "v2")).not.toEqual(
      observabilityKeys.insights("acme", { q: "b" }, "v2"),
    );
  });
});
