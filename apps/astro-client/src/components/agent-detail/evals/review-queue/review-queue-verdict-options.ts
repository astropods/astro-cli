import type { LucideIcon } from "lucide-react";
import { Check, Minus, X } from "lucide-react";
import type { DatasetJudgmentVerdict } from "@/lib/api";

export const REVIEW_QUEUE_VERDICT_OPTIONS: Array<{
  verdict: DatasetJudgmentVerdict;
  label: string;
  shortcut: string;
  Icon: LucideIcon;
  iconClassName: string;
}> = [
  {
    verdict: "good",
    label: "Good",
    shortcut: "G",
    Icon: Check,
    iconClassName: "text-success",
  },
  {
    verdict: "bad",
    label: "Bad",
    shortcut: "B",
    Icon: X,
    iconClassName: "text-destructive",
  },
  {
    verdict: "unknown",
    label: "Not sure",
    shortcut: "S",
    Icon: Minus,
    iconClassName: "text-muted-foreground",
  },
];
