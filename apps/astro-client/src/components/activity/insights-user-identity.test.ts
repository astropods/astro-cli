import { describe, expect, it } from "vitest";
import {
  classifyUserID,
  countSlackRowsMissingDetails,
  identityRefFromUserID,
  insightsUserIdentityKey,
  slackIdentityDisplay,
} from "./insights-user-identity";

describe("classifyUserID", () => {
  it("classifies WorkOS-prefixed ids as astro", () => {
    expect(classifyUserID("user_01HXX_bob")).toBe("astro");
  });
  it("classifies bare Slack ids as slack", () => {
    expect(classifyUserID("U07BOBBOB1")).toBe("slack");
  });
  it("classifies opaque ids as unknown", () => {
    expect(classifyUserID("anon-session-7f3")).toBe("unknown");
  });
  it("classifies empty string as unknown", () => {
    expect(classifyUserID("")).toBe("unknown");
  });
});

describe("insightsUserIdentityKey", () => {
  it("returns user_id for astro users", () => {
    expect(insightsUserIdentityKey({
      user_id: "user_01HXX_bob",
      user_details: { kind: "astro" },
    })).toBe("user_01HXX_bob");
  });
  it("returns slack:<team>:<uid> for slack users with team_id", () => {
    expect(insightsUserIdentityKey({
      user_id: "U07BOBBOB1",
      user_details: { kind: "slack", team_id: "T07POSTMAN" },
    })).toBe("slack:T07POSTMAN:U07BOBBOB1");
  });
  it("falls back to user_id for slack users without team_id", () => {
    expect(insightsUserIdentityKey({
      user_id: "U07GHOSTLY",
      user_details: { kind: "slack" },
    })).toBe("U07GHOSTLY");
  });
});

describe("countSlackRowsMissingDetails", () => {
  it("counts unique Slack rows missing team or profile details", () => {
    expect(countSlackRowsMissingDetails([
      { user_id: "U07UNKNOWN", user_details: { kind: "slack" } },
      { user_id: "U07UNKNOWN", user_details: { kind: "slack" } },
      { user_id: "U08NOPROF1", user_details: { kind: "slack", team_id: "T08TEAM" } },
      { user_id: "U09READY01", user_details: { kind: "slack", team_id: "T09TEAM", display_name: "Ready User" } },
      { user_id: "user_01HXX", user_details: { kind: "astro" } },
    ])).toBe(2);
  });

  it("does not count enriched Slack rows even when avatar is absent", () => {
    expect(countSlackRowsMissingDetails([
      { user_id: "U09READY01", user_details: { kind: "slack", team_id: "T09TEAM", display_name: "Ready User" } },
      { user_id: "U09HANDLE1", user_details: { kind: "slack", team_id: "T09TEAM", username: "handle.only" } },
    ])).toBe(0);
  });
});

describe("slackIdentityDisplay", () => {
  it("prefers username, then display name, then raw Slack user fallback", () => {
    expect(slackIdentityDisplay({
      user_id: "U09READY01",
      user_details: { kind: "slack", display_name: "Ready User", username: "ready" },
    }).primary).toBe("ready");

    expect(slackIdentityDisplay({
      user_id: "U09DISPLAY",
      user_details: { kind: "slack", display_name: "Display Only" },
    }).primary).toBe("Display Only");

    expect(slackIdentityDisplay({
      user_id: "U09RAWONLY",
      user_details: { kind: "slack" },
    }).primary).toBe("Slack user - U09RAWONLY");
  });

  it("builds a slack:// deep link only when team_id is present", () => {
    expect(slackIdentityDisplay({
      user_id: "U09READY01",
      user_details: { kind: "slack", team_id: "T09TEAM" },
    }).deepLink).toBe("slack://user?team=T09TEAM&id=U09READY01");

    expect(slackIdentityDisplay({
      user_id: "U09RAWONLY",
      user_details: { kind: "slack" },
    }).deepLink).toBeUndefined();
  });
});

describe("identityRefFromUserID", () => {
  it("classifies the user_id and returns a placeholder UserIdentity", () => {
    expect(identityRefFromUserID("user_01HXX")).toEqual({
      user_id: "user_01HXX",
      user_details: { kind: "astro" },
    });
    expect(identityRefFromUserID("U07SLACK01")).toEqual({
      user_id: "U07SLACK01",
      user_details: { kind: "slack" },
    });
    expect(identityRefFromUserID("anon-7f3")).toEqual({
      user_id: "anon-7f3",
      user_details: { kind: "unknown" },
    });
  });
});
