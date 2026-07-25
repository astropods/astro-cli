import type { InsightsIdentityRef } from "@/lib/api";

type IdentitySignal = Pick<InsightsIdentityRef, "kind" | "user_details">;

// Insights lists agents and Slack bots alongside real people, so usage metrics
// blur human engagement with automated traffic. These classify an identity so
// the UI can flag the non-human ones.
export function isNonHumanIdentity(identity: IdentitySignal): boolean {
  return (
    identity.kind === "agent" ||
    identity.kind === "system" ||
    identity.user_details?.is_bot === true
  );
}

// Short badge label for a non-human identity, or null for a human (no badge).
// "system" rows carry their own marker already, so they get no badge here.
// is_bot only comes from the Slack directory, so the bot is always a Slack bot.
export function nonHumanLabel(identity: IdentitySignal): string | null {
  if (identity.kind === "agent") return "Agent";
  if (identity.user_details?.is_bot) return "Slack bot";
  return null;
}
