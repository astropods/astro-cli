import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { screen, cleanup, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { Outlet } from "react-router";
import { server } from "@/test/msw/server";
import { mockTemplate, wrapTemplateResponse } from "@/test/msw/handlers";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import type { AgentDeployment } from "@/lib/api";
import type { AuthContextType } from "@/lib/auth-context";
import { DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH } from "@/components/deploy/constants";
import AgentConfigure from "./AgentConfigure";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeDeployment(overrides?: Partial<AgentDeployment>): AgentDeployment {
  return {
    id: "dep-1",
    name: "code-reviewer",
    display_name: "Code Reviewer",
    build_id: "b2c3d4e5f6a7",
    namespace: "astro-ns",
    status: "Running",
    replicas: 1,
    created_at: "2025-04-01T00:00:00Z",
    components: ["agent"],
    avatar_colors: { accent: "#2dd4bf", base: "#0f766e", vibrant: "#2dd4bf", vibrant_light: "#5eead4", accent_light: "#99f6e4", background: "#042f2e", foreground: "#f0fdfa", glow: "#2dd4bf" },
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  // Account variables endpoint (used by useDeployForm for vault references)
  server.use(
    http.get("/api/v1/accounts/:account/variables", () =>
      HttpResponse.json({ variables: [] }),
    ),
    http.post("/api/v1/agents/:account/:name/deployment-template", async ({ request }) => {
      const body = (await request.json().catch(() => ({}))) as Parameters<typeof wrapTemplateResponse>[1];
      const tmpl = {
        ...mockTemplate,
        interfaces: { ...mockTemplate.interfaces, adapters: ["web"] },
      };
      return HttpResponse.json(wrapTemplateResponse(tmpl, body));
    }),
  );
});

afterEach(cleanup);
afterEach(() => server.resetHandlers());

// ---------------------------------------------------------------------------
// Rendering helper
// ---------------------------------------------------------------------------

function renderConfigure(
  deployment?: AgentDeployment,
  searchParams?: string,
  opts?: { account?: string; auth?: AuthContextType },
) {
  const dep = deployment ?? makeDeployment();
  const user = userEvent.setup();
  const account = opts?.account ?? "testuser";
  const path = `/${account}/agents/${dep.id}/configure${searchParams ? `?${searchParams}` : ""}`;

  const result = renderRoute(
    [
      {
        path: "/:account/agents/:deploymentId",
        Component: () => (
          <Outlet
            context={{
              deployment: dep,
              account,
              deploymentId: dep.id,
            }}
          />
        ),
        children: [
          { path: "configure", Component: AgentConfigure },
          {
            path: "deployments",
            Component: () => <div data-testid="deployments-page">Deployments</div>,
          },
        ],
      },
    ],
    { initialEntries: [path], auth: opts?.auth ?? mockAuthContext },
  );

  return { ...result, user };
}

/** Wait for the template-driven form to fully load. */
async function waitForForm() {
  await waitFor(() => {
    expect(screen.getByText("Messaging interface")).toBeInTheDocument();
  });
}

// ===========================================================================
// Tests
// ===========================================================================

describe("page loads and shows the configuration form", () => {
  it("shows the form with required and optional sections and prefilled fields", async () => {
    renderConfigure();
    await waitForForm();

    // All four section headings exist
    expect(screen.getByText("General")).toBeInTheDocument();
    expect(screen.getByText("Messaging interface")).toBeInTheDocument();
    expect(screen.getByText("Configuration")).toBeInTheDocument();
    expect(screen.getByText("Optional credentials")).toBeInTheDocument();

    // Required variable from the mock template is rendered as a labelled input
    expect(screen.getByLabelText("OpenAI API Key")).toBeInTheDocument();
  });

  it("prefills the name field from the template's display_name", async () => {
    // Override the handler to return a display_name that's distinct from
    // slugToTitle(agent_name) — otherwise we can't tell whether the form is
    // reading from the template or just slugifying the agent name as a fallback.
    server.use(
      http.post("/api/v1/agents/:account/:name/deployment-template", async ({ request }) => {
        const body = (await request.json().catch(() => ({}))) as Record<string, unknown>;
        const tmpl = {
          ...mockTemplate,
          target: { ...mockTemplate.target, display_name: "Reviewer Bot" },
        };
        return HttpResponse.json(
          wrapTemplateResponse(tmpl, body as Parameters<typeof wrapTemplateResponse>[1]),
        );
      }),
    );

    renderConfigure();
    await waitForForm();
    expect(await screen.findByDisplayValue("Reviewer Bot")).toBeInTheDocument();
  });

  it("shows the template error message when the template fails to load", async () => {
    server.use(
      http.post("/api/v1/agents/:account/:name/deployment-template", () =>
        HttpResponse.json(
          { error: "internal_error", error_description: "Template unavailable" },
          { status: 500 },
        ),
      ),
    );
    renderConfigure();
    await waitFor(() => {
      expect(screen.getByText("Template unavailable")).toBeInTheDocument();
    });
  });
});

describe("select dropdown value preservation", () => {
  // Regression: Radix Select wiped the seeded value on the "" → real-value
  // transition. The trigger fell back to the placeholder while the hidden
  // form-bubble <option> still matched — so naive `findByText` would pass.
  // This test scopes to the visible trigger to catch the actual symptom.
  it("dropdown trigger shows the selected value", async () => {
    server.use(
      http.post("/api/v1/agents/:account/:name/deployment-template", async ({ request }) => {
        const body = (await request.json().catch(() => ({}))) as Record<string, unknown>;
        const tmpl = {
          ...mockTemplate,
          variables: {
            ...mockTemplate.variables,
            CLAUDE_MODEL: {
              value: "claude-opus-4-6",
              default: "claude-opus-4-6",
              targets: ["agent"],
              datatype: "string",
              "display-as": "select",
              options: ["claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"],
              description: "Claude model used for PR analysis.",
            },
          },
        };
        return HttpResponse.json(
          wrapTemplateResponse(tmpl, body as Parameters<typeof wrapTemplateResponse>[1]),
        );
      }),
    );

    renderConfigure();
    await waitForForm();

    const triggers = screen.getAllByRole("combobox");
    const claudeModelTrigger = triggers.find((t) => t.getAttribute("id") === "CLAUDE_MODEL");
    expect(claudeModelTrigger).toBeDefined();
    await waitFor(() => {
      expect(claudeModelTrigger).toHaveTextContent("claude-opus-4-6");
      expect(claudeModelTrigger).not.toHaveTextContent("Select an option");
    });
  });
});

describe("user edits the agent name", () => {
  it("changing the name shows the footer with Save button", async () => {
    const { user } = renderConfigure();
    await waitForForm();

    const nameInput = screen.getByDisplayValue("Code Reviewer");
    await user.clear(nameInput);
    await user.type(nameInput, "My New Name");

    expect(await screen.findByText(/save to update the agent name/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save/i })).toBeInTheDocument();
  });

  it("saving a name-only change renames the deployment and clears the dirty footer", async () => {
    const renameHandler = vi.fn();
    server.use(
      http.patch("/api/v1/deployments/:id", async ({ params, request }) => {
        const body = (await request.json()) as { display_name: string };
        renameHandler({ id: params.id, displayName: body.display_name });
        return HttpResponse.json({ display_name: body.display_name });
      }),
    );
    const { user } = renderConfigure();
    await waitForForm();

    const nameInput = screen.getByDisplayValue("Code Reviewer");
    await user.clear(nameInput);
    await user.type(nameInput, "Renamed Agent");
    await user.click(screen.getByRole("button", { name: /save/i }));

    // Mutation hits the right deployment with the right name
    await waitFor(() =>
      expect(renameHandler).toHaveBeenCalledWith({
        id: "dep-1",
        displayName: "Renamed Agent",
      }),
    );

    // After success, the footer disappears (the new name becomes the initial state).
    // The dirty flag uses useDeferredValue so the update can lag under slow CI CPUs.
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /^save$/i })).not.toBeInTheDocument();
    }, { timeout: 3000 });
    expect(screen.getByDisplayValue("Renamed Agent")).toBeInTheDocument();
  });

  it("does not send rename requests for over-length names", async () => {
    const renameHandler = vi.fn();
    server.use(
      http.patch("/api/v1/deployments/:id", async ({ request }) => {
        renameHandler(await request.json());
        return HttpResponse.json({ display_name: "Should Not Save" });
      }),
    );
    const { user } = renderConfigure();
    await waitForForm();

    const nameInput = screen.getByDisplayValue("Code Reviewer");
    await user.clear(nameInput);
    await user.type(nameInput, "a".repeat(DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH + 1));

    expect(await screen.findByText(`Name must be ${DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH} characters or fewer`)).toBeInTheDocument();
    const saveButton = screen.getByRole("button", { name: /save/i });
    expect(saveButton).not.toBeDisabled();
    await user.click(saveButton);

    expect(renameHandler).not.toHaveBeenCalled();
  });
});

describe("user edits configuration variables", () => {
  it("changing a variable shows the footer with Redeploy button", async () => {
    const { user } = renderConfigure();
    await waitForForm();

    await user.type(screen.getByLabelText("OpenAI API Key"), "sk-new-key");

    expect(await screen.findByText(/redeploy to apply/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /redeploy/i })).toBeInTheDocument();
  });

  it("clearing a changed variable back to initial keeps the Redeploy button but drops the change message", async () => {
    const { user } = renderConfigure();
    await waitForForm();

    const input = screen.getByLabelText("OpenAI API Key");
    await user.type(input, "sk-temp");
    expect(await screen.findByText(/redeploy to apply/i)).toBeInTheDocument();

    await user.clear(input);
    // The footer stays mounted with a persistent Redeploy button; only the
    // pending-change message reverts to the clean-state copy.
    await waitFor(() => {
      expect(screen.queryByText(/redeploy to apply/i)).not.toBeInTheDocument();
    });
    expect(screen.getByText(/redeploy the current configuration/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /redeploy/i })).toBeInTheDocument();
  });
});

describe("user submits a deployment", () => {
  it("successful deploy posts to /api/v1/deploy and navigates to the deployments tab", async () => {
    const deployHandler = vi.fn();
    server.use(
      http.post("/api/v1/deploy", async ({ request }) => {
        deployHandler(await request.json());
        return HttpResponse.json({
          status: "deployed",
          name: "code-reviewer",
          build_id: "b2c3d4e5f6a7",
          k8s_namespace: "astro-ns",
          deployed_at: new Date().toISOString(),
          resources: [],
        });
      }),
    );
    const { user } = renderConfigure();
    await waitForForm();

    await user.type(screen.getByLabelText("OpenAI API Key"), "sk-test-key");
    await user.click(screen.getByRole("button", { name: /redeploy/i }));

    expect(await screen.findByTestId("deployments-page")).toBeInTheDocument();
    expect(deployHandler).toHaveBeenCalledTimes(1);
  });

  it("deploy error keeps the user on the form and surfaces the error", async () => {
    const deployHandler = vi.fn();
    server.use(
      http.post("/api/v1/deploy", () => {
        deployHandler();
        return HttpResponse.json(
          { error: "deploy_failed", error_description: "Insufficient resources" },
          { status: 422 },
        );
      }),
    );
    const { user } = renderConfigure();
    await waitForForm();

    await user.type(screen.getByLabelText("OpenAI API Key"), "sk-test-key");
    await user.click(screen.getByRole("button", { name: /redeploy/i }));

    await waitFor(() => {
      expect(screen.getByText("Insufficient resources")).toBeInTheDocument();
    });
    // Did NOT navigate away
    expect(screen.queryByTestId("deployments-page")).not.toBeInTheDocument();
    expect(deployHandler).toHaveBeenCalledTimes(1);
  });

  it("redeploys a protected custom interface without messaging adapters", async () => {
    const deployHandler = vi.fn();
    server.use(
      http.post("/api/v1/agents/:account/:name/deployment-template", async ({ request }) => {
        const body = (await request.json().catch(() => ({}))) as Parameters<typeof wrapTemplateResponse>[1];
        const tmpl = {
          ...mockTemplate,
          agent: {
            ...mockTemplate.agent,
            endpoints: { http: { port: 8080, expose: { enabled: true } } },
          },
          interfaces: {
            ...mockTemplate.interfaces,
            adapters: [],
            auth: { custom: { public: false, grants: [] } },
          },
          variables: {},
        };
        return HttpResponse.json(
          wrapTemplateResponse(tmpl, {
            ...body,
            interfaces: {
              adapters: [],
              auth: { custom: { public: false, grants: [] } },
            },
          }),
        );
      }),
      http.post("/api/v1/deploy", () => {
        deployHandler();
        return HttpResponse.json({ status: "deployed" });
      }),
    );

    const { user } = renderConfigure();
    await waitForForm();

    await user.click(screen.getByRole("button", { name: /redeploy/i }));

    expect(await screen.findByTestId("deployments-page")).toBeInTheDocument();
    expect(deployHandler).toHaveBeenCalledTimes(1);
  });

  it("clicking Redeploy with a required field empty does not call the deploy endpoint", async () => {
    const deployHandler = vi.fn();
    server.use(
      http.post("/api/v1/deploy", () => {
        deployHandler();
        return HttpResponse.json({ status: "deployed" });
      }),
    );

    // Rollback mode keeps the Redeploy button visible regardless of dirty state,
    // so we can click it while the required OpenAI API Key is empty.
    const { user } = renderConfigure(undefined, "revision=2&build=abc12345");
    await waitForForm();

    // Required field is empty by default in mockTemplate.
    expect(screen.getByLabelText("OpenAI API Key")).toHaveValue("");

    await user.click(screen.getByRole("button", { name: /redeploy/i }));

    // Validation blocks the request — no navigation, no deploy call.
    expect(screen.queryByTestId("deployments-page")).not.toBeInTheDocument();
    expect(deployHandler).not.toHaveBeenCalled();
  });
});

describe("user discards changes", () => {
  it("clicking Discard reverts the form and hides the footer", async () => {
    const { user } = renderConfigure();
    await waitForForm();

    const nameInput = screen.getByDisplayValue("Code Reviewer");
    await user.clear(nameInput);
    await user.type(nameInput, "Changed Name");
    expect(screen.getByDisplayValue("Changed Name")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: /discard/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /discard/i }));

    await waitFor(() => {
      expect(screen.getByDisplayValue("Code Reviewer")).toBeInTheDocument();
    });
    // Footer exits via animation — wait for Discard to leave the DOM.
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /discard/i })).not.toBeInTheDocument();
    }, { timeout: 3000 });
  });
});

describe("rollback mode", () => {
  it("shows rollback banner with revision number", async () => {
    renderConfigure(undefined, "revision=2&build=abc12345");
    await waitForForm();

    expect(screen.getByText("Rollback")).toBeInTheDocument();
    expect(screen.getByText("Config #2")).toBeInTheDocument();
    expect(screen.getByText("abc12345")).toBeInTheDocument();
  });

  it("footer shows rollback context", async () => {
    renderConfigure(undefined, "revision=2&build=abc12345");
    await waitForForm();

    expect(screen.getByText(/rollback to config #2/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /redeploy/i })).toBeInTheDocument();
  });

  // Note: clearing override params via the X button calls setSearchParams
  // which is not fully supported by createRoutesStub. The component logic
  // is correct; the routing stub limitation prevents testing the full flow.
});

describe("upgrade mode", () => {
  it("shows upgrade banner with build comparison", async () => {
    renderConfigure(
      makeDeployment({ build_id: "oldbuild1" }),
      "build=newbuild2",
    );
    await waitForForm();

    expect(screen.getByText("Update")).toBeInTheDocument();
    // Shows old → new build hash comparison
    expect(screen.getByText(/oldbuild/)).toBeInTheDocument();
    expect(screen.getByText("newbuild")).toBeInTheDocument();
  });

  it("footer shows upgrade context", async () => {
    renderConfigure(undefined, "build=newbuild2");
    await waitForForm();

    expect(screen.getByText(/update to new build/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /redeploy/i })).toBeInTheDocument();
  });
});

// Regression: redeploying an org deployment was building target.account from
// personalAccount.name (because useDeployForm seeds _targetAccount from "" and
// the seeding effect never calls setTargetAccount). This makes the server reject
// any private blueprint redeploy with "source agent not found" because
// canDeploySourceAgent treats it as a cross-account private deploy. Pin the
// configure-page contract: the deploy payload must always carry the URL
// account as both source and target, no matter which other accounts the user
// belongs to.
describe("redeploy of org-owned deployment", () => {
  function multiAccountAuth() {
    return {
      ...mockAuthContext,
      accounts: [
        { id: "acct-personal", name: "mattcolozzo", type: "personal" as const },
        { id: "acct-org", name: "astropods", type: "organization" as const },
      ],
    };
  }

  it("uses the URL account (org) as target.account, not the user's personal account", async () => {
    const deployHandler = vi.fn();
    server.use(
      // The configure page is for /astropods/agents/dep-1, so the
      // deployment-template POST hits the org account path.
      http.post("/api/v1/agents/astropods/:name/deployment-template", async ({ request }) => {
        const body = (await request.json().catch(() => ({}))) as Record<string, unknown>;
        // Server-resolved template: source.account is the publishing org;
        // target.account would be set by mergeDeploymentPrefill to the
        // deployment's owning account (also the org for same-account redeploy).
        const tmpl = {
          ...mockTemplate,
          source: { ...mockTemplate.source, account: "astropods" },
          target: { ...mockTemplate.target, account: "astropods", deployment_id: "dep-1" },
          interfaces: { ...mockTemplate.interfaces, adapters: ["web"] },
        };
        return HttpResponse.json(
          wrapTemplateResponse(tmpl, body as Parameters<typeof wrapTemplateResponse>[1]),
        );
      }),
      http.post("/api/v1/deploy", async ({ request }) => {
        const payload = (await request.json()) as Record<string, unknown>;
        deployHandler(payload);
        return HttpResponse.json({
          status: "deployed",
          deployment_id: "dep-1",
          name: "code-reviewer",
          build_id: "b2c3d4e5f6a7",
          k8s_namespace: "astro-ns",
          deployed_at: new Date().toISOString(),
          resources: [],
        });
      }),
    );

    const { user } = renderConfigure(undefined, undefined, {
      account: "astropods",
      auth: multiAccountAuth(),
    });
    await waitForForm();

    // Make a redeploy-required edit so the Redeploy button enables.
    await user.type(screen.getByLabelText("OpenAI API Key"), "sk-redeploy-key");
    await user.click(screen.getByRole("button", { name: /redeploy/i }));

    await waitFor(() => expect(deployHandler).toHaveBeenCalledTimes(1));

    const payload = deployHandler.mock.calls[0][0] as {
      source?: { account?: string };
      target?: { account?: string };
    };
    expect(payload.source?.account).toBe("astropods");
    expect(payload.target?.account).toBe("astropods");
  });
});
