import { describe, expect, it } from "vitest";
import { countSlackRowsMissingDetails } from "./insights-user-identity";

describe("countSlackRowsMissingDetails", () => {
  it("counts unique Slack rows missing team or profile details", () => {
    expect(countSlackRowsMissingDetails([
      { user_id: "U07UNKNOWN" },
      { user_id: "U07UNKNOWN" },
      { user_id: "U08NOPROF1", slack_team_id: "T08TEAM" },
      { user_id: "U09READY01", slack_team_id: "T09TEAM", slack_display_name: "Ready User" },
      { user_id: "user_01HXX" },
    ])).toBe(2);
  });

  it("does not count enriched Slack rows even when avatar is absent", () => {
    expect(countSlackRowsMissingDetails([
      { user_id: "U09READY01", slack_team_id: "T09TEAM", slack_display_name: "Ready User" },
    ])).toBe(0);
  });
});
