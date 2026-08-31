import { http, HttpResponse } from "msw";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ManageLimitsDialog } from "@/components/settings/ManageLimitsDialog";
import { buildSpendResponse } from "@/api/queries/billing.fixtures";
import type { BillingSpend } from "@/lib/api";

function spendHandlers(partial: Partial<BillingSpend> = {}) {
  return [
    http.get("/api/v1/accounts/:account/billing/spend", () =>
      HttpResponse.json(
        buildSpendResponse({
          currency: "USD",
          current_spend: 812.4,
          usage_spend: 812.4,
          limit: { amount: 100_000, in_alarm: false },
          ...partial,
        }),
      ),
    ),
    http.put("/api/v1/accounts/:account/billing/spend/thresholds", () =>
      HttpResponse.json({ available: true }),
    ),
    http.post("/api/v1/accounts/:account/quota-increase", () =>
      HttpResponse.json({ id: "req-1", status: "pending" }, { status: 201 }),
    ),
  ];
}

const meta = {
  title: "Settings/ManageLimitsDialog",
  component: ManageLimitsDialog,
  args: { account: "acme", open: true, onOpenChange: () => {} },
} satisfies Meta<typeof ManageLimitsDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The account's saved thresholds, both inside the self-serve ceiling. */
export const Default: Story = {
  parameters: { msw: { handlers: spendHandlers() } },
};

/** Type a limit above $1,000 to see the field offer the request route. */
export const AboveTheSelfServeCeiling: Story = {
  parameters: { msw: { handlers: spendHandlers() } },
};

/** An approved request raised this account's ceiling to $5,000. */
export const WithAGrantedCeiling: Story = {
  parameters: { msw: { handlers: spendHandlers({ spend_ceiling: 500_000 }) } },
};
