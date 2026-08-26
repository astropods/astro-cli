import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { renderRoute } from "@/test/test-utils";
import AppsSettings from "./AppsSettings";
import OrgAppsSettings from "./OrgAppsSettings";

const SCOPES = [
  { slug: "members:read", name: "Read members", description: "Read the people in this organization" },
  { slug: "audiences:manage", name: "Manage audiences", description: "Add and remove members", resource_type: "audience" },
  { slug: "audiences:read", name: "Read audiences", description: "Read audiences and membership", resource_type: "audience" },
];

const app = {
  id: "app-1",
  name: "ci-pipeline",
  client_id: "client_abc123",
  scopes: ["audiences:read"],
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
  return [
    http.get("/api/v1/accounts/test-org/apps", () => HttpResponse.json({ apps })),
    http.get("/api/v1/accounts/test-org/app-scopes", () => HttpResponse.json({ scopes: SCOPES })),
  ];
}

function renderPage() {
  renderRoute([{ path: "/settings/org/:orgSlug/apps", Component: OrgAppsSettings }], {
    initialEntries: ["/settings/org/test-org/apps"],
  });
}

afterEach(cleanup);

describe("OrgAppsSettings", () => {
  it("keeps secrets out of the collapsed row and never opens a dialog", async () => {
    server.use(...listing([app]));
    renderPage();

    expect(await screen.findByText("ci-pipeline")).toBeInTheDocument();
    expect(screen.getByText("client_abc123")).toBeInTheDocument();
    expect(screen.getByText("1 secret")).toBeInTheDocument();
    expect(screen.queryByText(/wxyz/)).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("manages secrets only in the expanded row", async () => {
    server.use(...listing([app]));
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByText("ci-pipeline"));

    expect(await screen.findByText(/wxyz/)).toBeInTheDocument();
    expect(screen.getByText(/Added/)).toBeInTheDocument();
    expect(screen.getByText(/Last used/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Add a secret/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Delete app/ })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await user.click(screen.getByText("ci-pipeline"));
    await waitFor(() => expect(screen.queryByText(/wxyz/)).not.toBeInTheDocument());
  });

  it("creates an app inline and reveals the secret as a row in its list", async () => {
    let created: unknown;
    const stored: unknown[] = [];
    server.use(
      http.get("/api/v1/accounts/test-org/apps", () => HttpResponse.json({ apps: stored })),
      http.get("/api/v1/accounts/test-org/app-scopes", () => HttpResponse.json({ scopes: SCOPES })),
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

    await user.type(screen.getByLabelText("Name"), "ci-pipeline");

    await user.click(await screen.findByRole("button", { name: "Select scopes" }));
    await user.type(await screen.findByPlaceholderText("Search scopes…"), "audien");
    await waitFor(() => expect(screen.queryByText("members:read")).not.toBeInTheDocument());
    await user.click(screen.getByText("audiences:manage"));
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "Create OAuth app" }));

    await waitFor(() =>
      expect(created).toEqual({ name: "ci-pipeline", scopes: ["audiences:manage"] }),
    );
    expect(await screen.findByText("sk_live_plaintext")).toBeInTheDocument();
    expect(screen.getByText("New secret")).toBeInTheDocument();
    expect(screen.getByText(/not shown again/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "I saved it" }));
    await waitFor(() => expect(screen.queryByText("sk_live_plaintext")).not.toBeInTheDocument());
  });

  it("groups scopes by resource", async () => {
    server.use(...listing([]));
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Create your first OAuth app/ }));
    await user.click(await screen.findByRole("button", { name: "Select scopes" }));

    // WorkOS gives audience permissions a resource type; members:read has none,
    // so its slug prefix stands in.
    expect(await screen.findByText("audience")).toBeInTheDocument();
    expect(screen.getByText("members")).toBeInTheDocument();
    expect(screen.getByText("audiences:manage")).toBeInTheDocument();
    expect(screen.getByText("audiences:read")).toBeInTheDocument();
  });

  it("shows selected scopes as removable chips", async () => {
    server.use(...listing([]));
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Create your first OAuth app/ }));
    await user.click(await screen.findByRole("button", { name: "Select scopes" }));
    await user.click(await screen.findByText("audiences:manage"));
    await user.click(await screen.findByText("members:read"));
    await user.keyboard("{Escape}");

    expect(screen.getByRole("button", { name: "Select scopes" })).toHaveTextContent("2 scopes selected");
    await user.click(screen.getByRole("button", { name: "Remove audiences:manage" }));

    expect(screen.getByRole("button", { name: "Select scopes" })).toHaveTextContent("1 scope selected");
    expect(screen.queryByRole("button", { name: "Remove audiences:manage" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove members:read" })).toBeInTheDocument();
  });

  it("explains an environment with no scopes configured", async () => {
    server.use(
      http.get("/api/v1/accounts/test-org/apps", () => HttpResponse.json({ apps: [] })),
      http.get("/api/v1/accounts/test-org/app-scopes", () => HttpResponse.json({ scopes: [] })),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Create your first OAuth app/ }));

    expect(await screen.findByText(/No scopes are configured/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Select scopes" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create OAuth app" })).toBeInTheDocument();
  });

  it("changes an app's scopes from the expanded row", async () => {
    let patched: unknown;
    server.use(
      ...listing([app]),
      http.patch("/api/v1/accounts/test-org/apps/app-1", async ({ request }) => {
        patched = await request.json();
        return HttpResponse.json({ ...app, scopes: ["audiences:manage"] });
      }),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByText("ci-pipeline"));
    expect(await screen.findByText("audiences:read")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Change scopes/ }));
    await user.click(await screen.findByRole("button", { name: "Select scopes" }));
    await user.click(await screen.findByText("audiences:manage"));
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Remove audiences:read" }));
    await user.click(screen.getByRole("button", { name: "Save scopes" }));

    await waitFor(() => expect(patched).toEqual({ scopes: ["audiences:manage"] }));
  });

  it("disables revoke on a lone secret and explains why", async () => {
    let revoked = false;
    server.use(
      ...listing([app]),
      http.delete("/api/v1/accounts/test-org/apps/app-1/secrets/sec_1", () => {
        revoked = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByText("ci-pipeline"));
    const revoke = screen.getByRole("button", { name: "Revoke secret ending wxyz" });
    expect(revoke).toHaveAttribute("aria-disabled", "true");

    await user.click(revoke);
    expect(revoked).toBe(false);

    expect(revoke.getAttribute("title")).toMatch(/before revoking this one/);
  });

  it("offers revoke once a replacement exists", async () => {
    server.use(...listing([{ ...app, secrets: [...app.secrets, { id: "sec_2", hint: "abcd" }] }]));
    renderPage();

    await userEvent.setup().click(await screen.findByText("ci-pipeline"));

    expect(await screen.findByRole("button", { name: "Revoke secret ending wxyz" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Revoke secret ending abcd" })).toBeInTheDocument();
  });

  it("confirms deletion inline rather than in a dialog", async () => {
    let deleted = false;
    server.use(
      ...listing([app]),
      http.delete("/api/v1/accounts/test-org/apps/app-1", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPage();
    const user = userEvent.setup();

    await user.click(await screen.findByText("ci-pipeline"));
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

describe("AppsSettings", () => {
  it("lists the signed-in user's personal apps", async () => {
    server.use(
      http.get("/api/v1/accounts/testuser/apps", () => HttpResponse.json({ apps: [app] })),
    );
    renderRoute([{ path: "/settings/apps", Component: AppsSettings }], {
      initialEntries: ["/settings/apps"],
    });

    expect(await screen.findByText("ci-pipeline")).toBeInTheDocument();
  });
});
