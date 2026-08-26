import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { renderRoute } from "@/test/test-utils";
import OrgAppsSettings from "./OrgAppsSettings";

const SCOPES = ["members:read", "audiences:read", "audiences:manage", "slack_identities:manage"];

const app = {
  id: "app-1",
  name: "lumos-connector",
  client_id: "client_abc123",
  scopes: [],
  secrets: [
    {
      id: "sec_1",
      hint: "wxyz",
      created_at: "2026-08-01T00:00:00Z",
      last_used_at: "2026-08-20T00:00:00Z",
    },
  ],
  created_at: "2026-08-01T00:00:00Z",
};

function listing(apps: unknown[]) {
  return http.get("/api/v1/accounts/test-org/apps", () =>
    HttpResponse.json({ apps, available_scopes: SCOPES }),
  );
}

function renderPage() {
  renderRoute([{ path: "/settings/org/:orgSlug/apps", Component: OrgAppsSettings }], {
    initialEntries: ["/settings/org/test-org/apps"],
  });
}

afterEach(cleanup);

describe("OrgAppsSettings", () => {
  it("keeps secrets out of the collapsed row and never opens a dialog", async () => {
    server.use(listing([app]));
    renderPage();

    expect(await screen.findByText("lumos-connector")).toBeInTheDocument();
    expect(screen.getByText("client_abc123")).toBeInTheDocument();
    expect(screen.getByText("1 secret")).toBeInTheDocument();
    expect(screen.queryByText(/wxyz/)).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("manages secrets only in the expanded row", async () => {
    server.use(listing([app]));
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByText("lumos-connector"));

    expect(await screen.findByText(/wxyz/)).toBeInTheDocument();
    expect(screen.getByText(/Added/)).toBeInTheDocument();
    expect(screen.getByText(/Last used/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Add a secret/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Delete app/ })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await user.click(screen.getByText("lumos-connector"));
    await waitFor(() => expect(screen.queryByText(/wxyz/)).not.toBeInTheDocument());
  });

  it("creates an app inline and reveals the secret as a row in its list", async () => {
    let created: unknown;
    const stored: unknown[] = [];
    server.use(
      http.get("/api/v1/accounts/test-org/apps", () =>
        HttpResponse.json({ apps: stored, available_scopes: SCOPES }),
      ),
      http.post("/api/v1/accounts/test-org/apps", async ({ request }) => {
        created = await request.json();
        stored.push(app);
        return HttpResponse.json(
          { app, secret: { id: "sec_new", hint: "wxyz", value: "sk_live_plaintext" } },
          { status: 201 },
        );
      }),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Create your first OAuth app/ }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByText(/coming soon/)).toBeInTheDocument();

    await user.type(screen.getByLabelText("Name"), "lumos-connector");
    await user.click(screen.getByRole("button", { name: "Create OAuth app" }));

    await waitFor(() => expect(created).toEqual({ name: "lumos-connector" }));
    expect(await screen.findByText("sk_live_plaintext")).toBeInTheDocument();
    expect(screen.getByText("New secret")).toBeInTheDocument();
    expect(screen.getByText(/not shown again/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "I saved it" }));
    await waitFor(() => expect(screen.queryByText("sk_live_plaintext")).not.toBeInTheDocument());
  });

  it("disables revoke on a lone secret and explains why", async () => {
    let revoked = false;
    server.use(
      listing([app]),
      http.delete("/api/v1/accounts/test-org/apps/app-1/secrets/sec_1", () => {
        revoked = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByText("lumos-connector"));
    const revoke = screen.getByRole("button", { name: "Revoke secret ending wxyz" });
    expect(revoke).toHaveAttribute("aria-disabled", "true");

    await user.click(revoke);
    expect(revoked).toBe(false);

    expect(revoke.getAttribute("title")).toMatch(/before revoking this one/);
  });

  it("offers revoke once a replacement exists", async () => {
    server.use(listing([{ ...app, secrets: [...app.secrets, { id: "sec_2", hint: "abcd" }] }]));
    renderPage();

    await userEvent.setup().click(await screen.findByText("lumos-connector"));

    expect(await screen.findByRole("button", { name: "Revoke secret ending wxyz" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Revoke secret ending abcd" })).toBeInTheDocument();
  });

  it("confirms deletion inline rather than in a dialog", async () => {
    let deleted = false;
    server.use(
      listing([app]),
      http.delete("/api/v1/accounts/test-org/apps/app-1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByText("lumos-connector"));
    await user.click(screen.getByRole("button", { name: /Delete app/ }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByText(/Every secret is revoked/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText(/Every secret is revoked/)).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Delete app/ }));
    await user.click(screen.getByRole("button", { name: "Delete app" }));
    await waitFor(() => expect(deleted).toBe(true));
  });
});
