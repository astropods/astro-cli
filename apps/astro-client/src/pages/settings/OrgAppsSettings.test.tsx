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
  scopes: ["audiences:manage"],
  secrets: [{ id: "sec_1", hint: "wxyz", last_used_at: "2026-08-20T00:00:00Z" }],
  created_at: "2026-08-01T00:00:00Z",
};

function renderPage() {
  renderRoute([{ path: "/settings/org/:orgSlug/apps", Component: OrgAppsSettings }], {
    initialEntries: ["/settings/org/test-org/apps"],
  });
}

afterEach(cleanup);

describe("OrgAppsSettings", () => {
  it("lists OAuth apps with their client ID and secret hints", async () => {
    server.use(
      http.get("/api/v1/accounts/test-org/apps", () =>
        HttpResponse.json({ apps: [app], available_scopes: SCOPES }),
      ),
    );
    renderPage();

    expect(await screen.findByText("lumos-connector")).toBeInTheDocument();
    expect(screen.getByText("client_abc123")).toBeInTheDocument();
    expect(screen.getByText("…wxyz")).toBeInTheDocument();
  });

  it("reveals the plaintext secret inline once after creating an OAuth app", async () => {
    let created: unknown;
    server.use(
      http.get("/api/v1/accounts/test-org/apps", () =>
        HttpResponse.json({ apps: [], available_scopes: SCOPES }),
      ),
      http.post("/api/v1/accounts/test-org/apps", async ({ request }) => {
        created = await request.json();
        return HttpResponse.json(
          { app, secret: { id: "sec_1", hint: "wxyz", value: "sk_live_plaintext" } },
          { status: 201 },
        );
      }),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Create your first OAuth app/ }));
    await user.type(screen.getByLabelText("Name"), "lumos-connector");
    await user.click(screen.getByRole("button", { name: "Create OAuth app" }));

    await waitFor(() => expect(created).toEqual({ name: "lumos-connector" }));
    expect(await screen.findByText("sk_live_plaintext")).toBeInTheDocument();
    expect(screen.getByText(/only time it is shown/)).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: /Copy/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "I saved it" }));
    await waitFor(() => expect(screen.queryByText("sk_live_plaintext")).not.toBeInTheDocument());
  });

  it("marks scope selection as coming soon rather than offering one", async () => {
    server.use(
      http.get("/api/v1/accounts/test-org/apps", () =>
        HttpResponse.json({ apps: [], available_scopes: SCOPES }),
      ),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Create your first OAuth app/ }));
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByText("audiences:manage")).not.toBeInTheDocument();
    expect(screen.getByText(/coming soon/)).toBeInTheDocument();

    await user.type(screen.getByLabelText("Name"), "ci");
    expect(screen.getByRole("button", { name: "Create OAuth app" })).toBeEnabled();
  });

  it("hides revoke on a lone secret so an app is never left without one", async () => {
    server.use(
      http.get("/api/v1/accounts/test-org/apps", () =>
        HttpResponse.json({ apps: [app], available_scopes: SCOPES }),
      ),
    );
    renderPage();

    await screen.findByText("lumos-connector");
    expect(screen.queryByRole("button", { name: /Revoke secret/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add a secret" })).toBeInTheDocument();
  });

  it("offers revoke once a replacement secret exists", async () => {
    server.use(
      http.get("/api/v1/accounts/test-org/apps", () =>
        HttpResponse.json({
          apps: [{ ...app, secrets: [...app.secrets, { id: "sec_2", hint: "abcd" }] }],
          available_scopes: SCOPES,
        }),
      ),
    );
    renderPage();

    await screen.findByText("lumos-connector");
    expect(await screen.findByRole("button", { name: "Revoke secret ending wxyz" })).toBeInTheDocument();
  });
});
