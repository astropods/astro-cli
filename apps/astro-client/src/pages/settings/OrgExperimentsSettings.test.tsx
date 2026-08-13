import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { renderRoute } from "@/test/test-utils";
import OrgExperimentsSettings from "./OrgExperimentsSettings";

afterEach(cleanup);

describe("OrgExperimentsSettings", () => {
  it("loads and updates the server-owned fine-grained access switch", async () => {
    let updated: unknown;
    server.use(
      http.get("/api/v1/accounts/test-org/experiments/fine-grained-access", () =>
        HttpResponse.json({ experiment: "fine_grained_access", enabled: false }),
      ),
      http.put("/api/v1/accounts/test-org/experiments/fine-grained-access", async ({ request }) => {
        updated = await request.json();
        return HttpResponse.json({ experiment: "fine_grained_access", enabled: true });
      }),
    );

    renderRoute(
      [{ path: "/settings/org/:orgSlug/experiments", Component: OrgExperimentsSettings }],
      { initialEntries: ["/settings/org/test-org/experiments"] },
    );

    const toggle = await screen.findByRole("switch", { name: "Fine-grained access" });
    await waitFor(() => expect(toggle).not.toBeDisabled());
    expect(toggle).not.toBeChecked();

    await userEvent.setup().click(toggle);
    await waitFor(() => expect(updated).toEqual({ enabled: true }));
    await waitFor(() => expect(toggle).toBeChecked());
  });
});
