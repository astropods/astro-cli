import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { mockTemplate, wrapTemplateResponse } from '@/test/msw/handlers';
import { mockAuthContext, renderRoute } from '@/test/test-utils';
import DeployBlueprint from './DeployBlueprint';
import type { Blueprint, DeploymentSpec } from '@/lib/api';
import type { AuthContextType } from '@/lib/auth-context';

afterEach(cleanup);

const ROUTE_PATH = '/:account/:agentSlug/install';
const ACCOUNT = 'testuser';
const AGENT = 'code-reviewer';

function renderInstall({ account = ACCOUNT, agent = AGENT, auth = undefined }: {
  account?: string;
  agent?: string;
  auth?: AuthContextType | null;
} = {}) {
  return renderRoute(
    [
      {
        path: ROUTE_PATH,
        Component: DeployBlueprint,
      },
    ],
    { initialEntries: [`/${account}/${agent}/install`], auth },
  );
}

function renderInstallWithAgentsRoute() {
  return renderRoute(
    [
      {
        path: ROUTE_PATH,
        Component: DeployBlueprint,
      },
      {
        path: '/agents',
        Component: () => <div>Dashboard Page</div>,
      },
    ],
    { initialEntries: [`/${ACCOUNT}/${AGENT}/install`] },
  );
}

/** Wait for the install form to be fully loaded. */
async function waitForForm() {
  await waitFor(() => {
    expect(screen.getByRole('heading', { level: 1, name: /Deploy/ })).toBeInTheDocument();
  });
  // Also wait for template-driven sections to appear
  await waitFor(() => {
    expect(screen.getByText('Messaging interface')).toBeInTheDocument();
  });
}

// ── Rendering & Data Loading ────────────────────────────────────────

describe('DeployBlueprint page', () => {
  describe('rendering & data loading', () => {
    it('renders the install form with agent name', async () => {
      renderInstall();
      await waitForForm();

      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Deploy');
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('code-reviewer');
    });

    it('shows agent not found when agent does not exist', async () => {
      renderInstall({ agent: 'no-such-agent' });

      await waitFor(() => {
        expect(screen.getByText('Agent not found')).toBeInTheDocument();
      });
      expect(screen.getByRole('link', { name: /blueprints/i })).toHaveAttribute('href', '/blueprints');
    });

    it('renders back link to agent detail page', async () => {
      renderInstall();
      await waitForForm();

      const links = screen.getAllByRole('link');
      const hrefs = links.map((l) => l.getAttribute('href'));
      expect(hrefs).toContain(`/${ACCOUNT}/${AGENT}`);
    });

    it('does not offer personal deploy scope for private org blueprints', async () => {
      const privateOrgBlueprint: Blueprint = {
        name: 'private-agent',
        account: 'source-org',
        registry: 'registry.example.com',
        visibility: 'private',
        versions: [
          {
            build_id: 'private-build-1',
            spec: {},
            published_at: '2026-04-25T12:00:00Z',
          },
        ],
      };
      const capturedRequests: unknown[] = [];
      server.use(
        http.get('/api/v1/agents/:account/:name', ({ params }) => {
          if (params.account === 'source-org' && params.name === 'private-agent') {
            return HttpResponse.json(privateOrgBlueprint);
          }
          return HttpResponse.json({ error: 'not_found' }, { status: 404 });
        }),
        http.post('/api/v1/agents/:account/:name/deployment-template', async ({ params, request }) => {
          if (params.account !== 'source-org' || params.name !== 'private-agent') {
            return HttpResponse.json({ error: 'not_found' }, { status: 404 });
          }
          const body = (await request.json().catch(() => ({}))) as {
            interfaces?: { adapters?: string[] };
            variables?: Record<string, { value?: string; ref?: string }>;
            schedules?: Record<string, string>;
          };
          return HttpResponse.json(wrapTemplateResponse(mockTemplate, body));
        }),
        http.post('/api/v1/deploy', async ({ request }) => {
          capturedRequests.push(await request.json());
          return HttpResponse.json({
            status: 'deployed',
            name: 'private-agent',
            build_id: 'private-build-1',
            k8s_namespace: 'source-org-private-agent',
            deployed_at: new Date().toISOString(),
            resources: [],
          });
        }),
      );

      const user = userEvent.setup();
      renderInstall({
        account: 'source-org',
        agent: 'private-agent',
        auth: {
          ...mockAuthContext,
          accounts: [
            { id: 'acct-personal', name: ACCOUNT, type: 'personal' },
            { id: 'acct-org', name: 'source-org', type: 'organization' },
          ],
        },
      });
      await waitForForm();

      expect(screen.queryByText('Deploy to')).not.toBeInTheDocument();

      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(capturedRequests).toHaveLength(1);
      });

      const payload = capturedRequests[0] as DeploymentSpec;
      expect(payload.target.account).toBe('source-org');
    });

    // Regression: deploying a public blueprint owned by a different account
    // was looking up vault variables under the BLUEPRINT owner's account
    // because the seeding effect overwrote the user's personal-account
    // default with the blueprint's URL/source account. The variables call,
    // the picker contents, and the deploy target must all resolve to the
    // deploying user's account, not the blueprint owner.
    it('routes vault lookup and deploy target to the deploying user, not the blueprint owner', async () => {
      const publicBlueprint: Blueprint = {
        name: 'shared-agent',
        account: 'acme',
        registry: 'registry.example.com',
        visibility: 'public',
        versions: [
          { build_id: 'shared-build-1', spec: {}, published_at: '2026-04-25T12:00:00Z' },
        ],
      };
      const variablesCalls: string[] = [];
      const capturedDeploys: DeploymentSpec[] = [];

      server.use(
        http.get('/api/v1/agents/:account/:name', ({ params }) => {
          if (params.account === 'acme' && params.name === 'shared-agent') {
            return HttpResponse.json(publicBlueprint);
          }
          return HttpResponse.json({ error: 'not_found' }, { status: 404 });
        }),
        http.post('/api/v1/agents/:account/:name/deployment-template', async ({ params, request }) => {
          if (params.account !== 'acme' || params.name !== 'shared-agent') {
            return HttpResponse.json({ error: 'not_found' }, { status: 404 });
          }
          const body = (await request.json().catch(() => ({}))) as Parameters<typeof wrapTemplateResponse>[1];
          const tmpl = {
            ...mockTemplate,
            source: { ...mockTemplate.source, account: 'acme', name: 'shared-agent' },
          };
          return HttpResponse.json(wrapTemplateResponse(tmpl, body));
        }),
        http.get('/api/v1/accounts/:account/variables', ({ params }) => {
          variablesCalls.push(params.account as string);
          return HttpResponse.json({
            variables: params.account === 'mattcolozzo'
              ? [{ name: 'MY_PERSONAL_KEY', description: 'My key', secret: true }]
              : [{ name: 'ACME_OWNER_KEY', description: 'Acme key', secret: true }],
          });
        }),
        http.post('/api/v1/deploy', async ({ request }) => {
          capturedDeploys.push((await request.json()) as DeploymentSpec);
          return HttpResponse.json({
            status: 'deployed',
            deployment_id: 'dep-shared-1',
            name: 'shared-agent',
            build_id: 'shared-build-1',
            k8s_namespace: 'mattcolozzo-shared',
            deployed_at: new Date().toISOString(),
            resources: [],
          });
        }),
      );

      const user = userEvent.setup();
      renderInstall({
        account: 'acme',
        agent: 'shared-agent',
        auth: {
          ...mockAuthContext,
          // User has only their personal account — they are NOT a member of
          // the blueprint owner ("acme"). This is the realistic public-blueprint
          // deploy scenario.
          accounts: [
            { id: 'acct-personal', name: 'mattcolozzo', type: 'personal' },
          ],
        },
      });
      await waitForForm();

      // The variables endpoint must be keyed off the deploying user, never
      // the blueprint's owning account.
      await waitFor(() => {
        expect(variablesCalls.length).toBeGreaterThan(0);
      });
      expect(variablesCalls).toContain('mattcolozzo');
      expect(variablesCalls).not.toContain('acme');

      // The picker should surface the user's variables only.
      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await user.click(firstKeyButton);
      await waitFor(() => {
        expect(screen.getByText('MY_PERSONAL_KEY')).toBeInTheDocument();
      });
      expect(screen.queryByText('ACME_OWNER_KEY')).not.toBeInTheDocument();

      // Close the picker by pressing Escape so the underlying form is interactive.
      await user.keyboard('{Escape}');

      // And the deploy submission must target the user's account.
      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(capturedDeploys).toHaveLength(1);
      });
      expect(capturedDeploys[0].target.account).toBe('mattcolozzo');
      expect(capturedDeploys[0].source?.account).toBe('acme');
    });
  });

  // ── Template Error ──────────────────────────────────────────────────

  describe('template error', () => {
    it('shows error panel when template fails to load', async () => {
      server.use(
        http.post('/api/v1/agents/:account/:name/deployment-template', () =>
          HttpResponse.json({ error: 'internal_error', error_description: 'Template service unavailable' }, { status: 500 }),
        ),
      );

      renderInstall();

      await waitFor(() => {
        expect(screen.getByText('Template service unavailable')).toBeInTheDocument();
      });
    });
  });

  // ── Name Field ──────────────────────────────────────────────────

  describe('name field', () => {
    it('renders with a default derived from the agent slug', async () => {
      renderInstall();
      await waitForForm();

      const nameInput = screen.getByDisplayValue('Code Reviewer');
      expect(nameInput).toBeInTheDocument();
    });

    it('allows the user to change the name', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      const nameInput = screen.getByDisplayValue('Code Reviewer');
      await user.clear(nameInput);
      await user.type(nameInput, 'My Custom Agent');

      expect(screen.getByDisplayValue('My Custom Agent')).toBeInTheDocument();
    });

    it('sends custom name in the deploy payload', async () => {
      const capturedRequests: unknown[] = [];
      server.use(
        http.post('/api/v1/deploy', async ({ request }) => {
          capturedRequests.push(await request.json());
          return HttpResponse.json({
            status: 'deployed',
            name: AGENT,
            build_id: 'a1b2c3d4e5f6',
            k8s_namespace: 'user-abc123',
            deployed_at: new Date().toISOString(),
            resources: [{ kind: 'Deployment', name: AGENT, status: 'created' }],
          });
        }),
      );

      const user = userEvent.setup();
      renderInstallWithAgentsRoute();
      await waitForForm();

      // Change name
      const nameInput = screen.getByDisplayValue('Code Reviewer');
      await user.clear(nameInput);
      await user.type(nameInput, 'My Bot');

      // Fill required credential
      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');

      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(capturedRequests).toHaveLength(1);
      });

      const payload = capturedRequests[0] as Record<string, unknown>;
      expect(payload).toMatchObject({
        target: expect.objectContaining({ display_name: 'My Bot' }),
      });
    });

    it('shows error when name is empty on submit', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Clear the name
      const nameInput = screen.getByDisplayValue('Code Reviewer');
      await user.clear(nameInput);

      // Fill credentials so that's not the blocker
      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');

      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByText('Enter a name for the agent')).toBeInTheDocument();
      });
    });
  });

  // ── Interfaces Picker ─────────────────────────────────────────────

  describe('interfaces picker', () => {
    it('has Web selected by default', async () => {
      renderInstall();
      await waitForForm();

      const webButton = screen.getByRole('button', { name: /chat/i });
      expect(webButton).toHaveAttribute('aria-pressed', 'true');

      const slackButton = screen.getByRole('button', { name: /slack/i });
      expect(slackButton).toHaveAttribute('aria-pressed', 'false');
    });

    it('shows Slack credential fields when Slack is toggled on', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      await user.click(screen.getByRole('button', { name: /slack/i }));

      await waitFor(() => {
        expect(screen.getByLabelText('Slack Bot Token')).toBeInTheDocument();
      });
      expect(screen.getByLabelText('Slack App Token')).toBeInTheDocument();
    });

    it('hides Slack credential fields when Slack is toggled off', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Toggle on
      await user.click(screen.getByRole('button', { name: /slack/i }));
      await waitFor(() => {
        expect(screen.getByLabelText('Slack Bot Token')).toBeInTheDocument();
      });

      // Toggle off
      await user.click(screen.getByRole('button', { name: /slack/i }));
      await waitFor(() => {
        expect(screen.queryByLabelText('Slack Bot Token')).not.toBeInTheDocument();
      });
    });
  });

  // ── Vault Picker in Slack Credentials ────────────────────────────

  describe('vault picker in Slack credentials', () => {
    async function enableSlack(user: ReturnType<typeof userEvent.setup>) {
      await user.click(screen.getByRole('button', { name: /slack/i }));
      await waitFor(() => {
        expect(screen.getByLabelText('Slack Bot Token')).toBeInTheDocument();
      });
    }

    it('shows vault key buttons on Slack token fields when Slack is enabled', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();
      await enableSlack(user);

      // Slack Bot Token + Slack App Token each have a vault key button
      const keyButtons = screen.getAllByTitle('Insert vault reference');
      expect(keyButtons.length).toBeGreaterThanOrEqual(2);
    });

    it('shows vault entries in Slack token picker when account has variables', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/variables', () =>
          HttpResponse.json({
            variables: [
              { name: 'MY_SLACK_BOT_TOKEN', description: 'Slack bot token', secret: true },
              { name: 'MY_SLACK_APP_TOKEN', description: 'Slack app token', secret: true },
            ],
          }),
        ),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();
      await enableSlack(user);

      // Slack fields are rendered before the Configuration section, so index 0 is Slack Bot Token
      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await user.click(firstKeyButton);

      await waitFor(() => {
        expect(screen.getByText('MY_SLACK_BOT_TOKEN')).toBeInTheDocument();
        expect(screen.getByText('MY_SLACK_APP_TOKEN')).toBeInTheDocument();
      });
      expect(screen.queryByText('No variables yet')).not.toBeInTheDocument();
    });

    it('shows empty state in Slack token picker when account has no variables', async () => {
      // Default handler in handlers.ts returns { variables: [] }
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();
      await enableSlack(user);

      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await user.click(firstKeyButton);

      await waitFor(() => {
        expect(screen.getByText('No variables yet')).toBeInTheDocument();
      });
    });

    it('shows load error in vault picker when variables API fails', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/variables', () =>
          HttpResponse.json({ error: 'insufficient permissions for this account' }, { status: 403 }),
        ),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();
      await enableSlack(user);

      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await user.click(firstKeyButton);

      await waitFor(() => {
        expect(screen.getByText('Could not load variables')).toBeInTheDocument();
      });
      expect(screen.queryByText('No variables yet')).not.toBeInTheDocument();
    });

    it('shows ErrorPanel when variables list fails on deploy form', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/variables', () =>
          HttpResponse.json({ error: 'insufficient permissions for this account' }, { status: 403 }),
        ),
      );

      renderInstall();
      await waitForForm();

      await waitFor(() => {
        expect(screen.getByText("Couldn't load your variables")).toBeInTheDocument();
        expect(screen.getByText('insufficient permissions for this account')).toBeInTheDocument();
      });
    });

    it('shows ErrorPanel with switch-org message when JWT org scope mismatches target account', async () => {
      const msg = 'session is not scoped to this organization, use switch-org first';
      server.use(
        http.get('/api/v1/accounts/:account/variables', () =>
          HttpResponse.json({ error: msg }, { status: 403 }),
        ),
      );

      renderInstall();
      await waitForForm();

      await waitFor(() => {
        expect(screen.getByText("Couldn't load your variables")).toBeInTheDocument();
        expect(screen.getByText(msg)).toBeInTheDocument();
      });
    });

    // Regression: InterfacesPicker did not forward the form's targetAccount to
    // the Slack adapter's VariableFields. The VaultPicker therefore created
    // variables against an empty account, and POST /api/v1/accounts//variables
    // 400'd with "account name is required" — even for admins. Verifies the
    // create call lands on the resolved account.
    it('creates new variable against the form account when launched from Slack vault picker', async () => {
      const variableCreates: { account: string; body: unknown }[] = [];
      server.use(
        http.post('/api/v1/accounts/:account/variables', async ({ params, request }) => {
          variableCreates.push({
            account: params.account as string,
            body: await request.json(),
          });
          return HttpResponse.json({ results: [{ name: 'NEW_SLACK_BOT_TOKEN', status: 'created' }] });
        }),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();
      await enableSlack(user);

      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await user.click(firstKeyButton);

      // Empty-state CTA inside the popover (account has no vault entries by default).
      const newVariableButton = await screen.findByRole('button', { name: /new variable/i });
      await user.click(newVariableButton);

      // NewEntryDialog opens with the Key prefilled from the field that launched
      // the picker; clear it and enter a custom name.
      const keyInput = await screen.findByLabelText('Key');
      await user.clear(keyInput);
      await user.type(keyInput, 'NEW_SLACK_BOT_TOKEN');
      await user.type(screen.getByLabelText('Value'), 'xoxb-from-test');
      await user.click(screen.getByRole('button', { name: /^save$/i }));

      await waitFor(() => {
        expect(variableCreates).toHaveLength(1);
      });
      expect(variableCreates[0].account).toBe(ACCOUNT);
      expect(variableCreates[0].body).toMatchObject({
        variables: [
          expect.objectContaining({ name: 'NEW_SLACK_BOT_TOKEN', value: 'xoxb-from-test' }),
        ],
      });
    });

    it('inserts vault reference into Slack token field on selection', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/variables', () =>
          HttpResponse.json({
            variables: [
              { name: 'MY_BOT_TOKEN', description: 'Bot token', secret: true },
            ],
          }),
        ),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();
      await enableSlack(user);

      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await user.click(firstKeyButton);

      await waitFor(() => {
        expect(screen.getByText('MY_BOT_TOKEN')).toBeInTheDocument();
      });
      await user.click(screen.getByText('MY_BOT_TOKEN'));

      await waitFor(() => {
        // Input replaced by vault reference chip
        expect(screen.getByRole('button', { name: 'Clear vault reference' })).toBeInTheDocument();
      });
    });
  });

  // ── Credential Fields ─────────────────────────────────────────────

  describe('credential fields', () => {
    it('renders required credential fields from template', async () => {
      renderInstall();
      await waitForForm();

      expect(screen.getByText('Configuration')).toBeInTheDocument();
      expect(screen.getByLabelText('OpenAI API Key')).toBeInTheDocument();
    });

    it('renders optional credential fields from template', async () => {
      renderInstall();
      await waitForForm();

      expect(screen.getByText('Optional credentials')).toBeInTheDocument();
      expect(screen.getByLabelText('Sentry Dsn')).toBeInTheDocument();
    });

    it('hides sections when template has no credentials', async () => {
      const noVarsTemplate = { ...mockTemplate, variables: {} };
      server.use(
        http.post('/api/v1/agents/:account/:name/deployment-template', async ({ request }) => {
          const body = (await request.json().catch(() => ({}))) as Record<string, unknown>;
          return HttpResponse.json(wrapTemplateResponse(noVarsTemplate, body as { interfaces?: { adapters?: string[] }; variables?: Record<string, { value?: string; ref?: string }> }));
        }),
      );

      renderInstall();
      await waitForForm();

      expect(screen.queryByText('Configuration')).not.toBeInTheDocument();
      expect(screen.queryByText('Optional credentials')).not.toBeInTheDocument();
    });
  });

  // ── Vault Picker in Credential Fields ────────────────────────────

  describe('vault picker in credential fields', () => {
    it('shows vault key buttons on regular credential fields', async () => {
      renderInstall();
      await waitForForm();

      // OPENAI_API_KEY (secret) and SENTRY_DSN (text) both have a key button
      const keyButtons = screen.getAllByTitle('Insert vault reference');
      expect(keyButtons.length).toBeGreaterThanOrEqual(2);
    });

    it('shows vault entries in credential field picker when account has variables', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/variables', () =>
          HttpResponse.json({
            variables: [
              { name: 'OPENAI_KEY', description: 'OpenAI key', secret: true },
              { name: 'SENTRY_TOKEN', description: 'Sentry token', secret: false },
            ],
          }),
        ),
      );

      renderInstall();
      await waitForForm();

      // With Slack disabled, the first key button belongs to OPENAI_API_KEY
      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await userEvent.setup().click(firstKeyButton);

      await waitFor(() => {
        expect(screen.getByText('OPENAI_KEY')).toBeInTheDocument();
        expect(screen.getByText('SENTRY_TOKEN')).toBeInTheDocument();
      });
      expect(screen.queryByText('No variables yet')).not.toBeInTheDocument();
    });

    it('shows empty state in credential field picker when account has no variables', async () => {
      // Default handler in handlers.ts returns { variables: [] }
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await user.click(firstKeyButton);

      await waitFor(() => {
        expect(screen.getByText('No variables yet')).toBeInTheDocument();
      });
    });

    it('inserts vault reference into credential field on selection', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/variables', () =>
          HttpResponse.json({
            variables: [
              { name: 'MY_OPENAI_KEY', description: 'OpenAI key', secret: true },
            ],
          }),
        ),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      const [firstKeyButton] = screen.getAllByTitle('Insert vault reference');
      await user.click(firstKeyButton);

      await waitFor(() => {
        expect(screen.getByText('MY_OPENAI_KEY')).toBeInTheDocument();
      });
      await user.click(screen.getByText('MY_OPENAI_KEY'));

      await waitFor(() => {
        // Input replaced by vault reference chip
        expect(screen.getByRole('button', { name: 'Clear vault reference' })).toBeInTheDocument();
      });
    });
  });

  // ── Submit-Time Validation ─────────────────────────────────────────

  describe('submit-time validation', () => {
    it('launch button is always enabled before submission', async () => {
      renderInstall();
      await waitForForm();

      // Button should be enabled even with empty credentials
      expect(screen.getByRole('button', { name: /deploy/i })).toBeEnabled();
    });

    it('shows inline errors on required credentials when submitting with empty fields', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByLabelText('OpenAI API Key')).toHaveAttribute('aria-invalid', 'true');
      });
      expect(screen.getByText('Required')).toBeInTheDocument();
    });

    it('does not show inline errors before first submit attempt', async () => {
      renderInstall();
      await waitForForm();

      expect(screen.getByLabelText('OpenAI API Key')).not.toHaveAttribute('aria-invalid');
      expect(screen.queryByText('Required')).not.toBeInTheDocument();
    });

    it('clears credential errors when fields are filled after submit', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Submit with empty fields
      await user.click(screen.getByRole('button', { name: /deploy/i }));
      await waitFor(() => {
        expect(screen.getByLabelText('OpenAI API Key')).toHaveAttribute('aria-invalid', 'true');
      });

      // Fill the field
      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');

      await waitFor(() => {
        expect(screen.getByLabelText('OpenAI API Key')).not.toHaveAttribute('aria-invalid');
      });
      expect(screen.queryByText('Required')).not.toBeInTheDocument();
    });

    it('shows messaging error as inline field text when all types are deselected and form is submitted', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Fill credentials so that's not the issue
      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');

      // Deselect Web
      await user.click(screen.getByRole('button', { name: /chat/i }));

      // No error yet (haven't submitted)
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
      expect(screen.getByRole('group', { name: /messaging interface options/i })).not.toHaveAttribute('aria-invalid');

      // Submit
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      const messagingError = await screen.findByText('Select at least one messaging type');
      const messagingOptions = screen.getByRole('group', { name: /messaging interface options/i });
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
      expect(messagingOptions).toHaveAttribute('aria-invalid', 'true');
      expect(messagingOptions).toHaveClass('outline-destructive');
      expect(messagingError).toHaveClass('text-destructive', 'text-xs');
    });

    it('clears messaging error when a type is reselected after submit', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /chat/i }));
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByText('Select at least one messaging type')).toBeInTheDocument();
      });

      // Reselect Web
      await user.click(screen.getByRole('button', { name: /chat/i }));

      await waitFor(() => {
        expect(screen.queryByText('Select at least one messaging type')).not.toBeInTheDocument();
      });
      expect(screen.getByRole('group', { name: /messaging interface options/i })).not.toHaveAttribute('aria-invalid');
    });

    it('shows inline errors on Slack credentials when submitted with empty tokens', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Fill agent credential
      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');

      // Select Slack
      await user.click(screen.getByRole('button', { name: /slack/i }));
      await waitFor(() => {
        expect(screen.getByLabelText('Slack Bot Token')).toBeInTheDocument();
      });

      // Submit without filling Slack tokens
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByLabelText('Slack Bot Token')).toHaveAttribute('aria-invalid', 'true');
        expect(screen.getByLabelText('Slack App Token')).toHaveAttribute('aria-invalid', 'true');
      });
    });

    it('does not deploy when validation fails', async () => {
      const capturedRequests: unknown[] = [];
      server.use(
        http.post('/api/v1/deploy', async ({ request }) => {
          capturedRequests.push(await request.json());
          return HttpResponse.json({ status: 'deployed' });
        }),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Submit with empty required fields
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      // Wait a tick and verify no request was made
      await waitFor(() => {
        expect(screen.getByLabelText('OpenAI API Key')).toHaveAttribute('aria-invalid', 'true');
      });
      expect(capturedRequests).toHaveLength(0);
    });
  });

  // ── Deployment Submission ─────────────────────────────────────────

  describe('deployment submission', () => {
    it('navigates to /agents on successful deploy', async () => {
      const user = userEvent.setup();
      renderInstallWithAgentsRoute();
      await waitForForm();

      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByText('Dashboard Page')).toBeInTheDocument();
      });
    });

    it('sends correct payload with credentials and interfaces', async () => {
      const capturedRequests: unknown[] = [];
      server.use(
        http.post('/api/v1/deploy', async ({ request }) => {
          capturedRequests.push(await request.json());
          return HttpResponse.json({
            status: 'deployed',
            name: AGENT,
            build_id: 'a1b2c3d4e5f6',
            k8s_namespace: 'user-abc123',
            deployed_at: new Date().toISOString(),
            resources: [{ kind: 'Deployment', name: AGENT, status: 'created' }],
          });
        }),
      );

      const user = userEvent.setup();
      renderInstallWithAgentsRoute();
      await waitForForm();

      // Fill required credential
      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');

      // Enable Slack and fill its tokens
      await user.click(screen.getByRole('button', { name: /slack/i }));
      await waitFor(() => {
        expect(screen.getByLabelText('Slack Bot Token')).toBeInTheDocument();
      });
      await user.type(screen.getByLabelText('Slack Bot Token'), 'xoxb-test');
      await user.type(screen.getByLabelText('Slack App Token'), 'xapp-test');

      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(capturedRequests).toHaveLength(1);
      });

      const payload = capturedRequests[0] as Record<string, unknown>;
      expect(payload).toMatchObject({
        spec: 'deployment/v1',
        source: { account: 'testuser', name: AGENT },
        target: expect.objectContaining({ display_name: 'Code Reviewer' }),
        variables: {
          OPENAI_API_KEY: expect.objectContaining({ value: 'sk-test123' }),
        },
      });
    });

    it('matches template contract: deploys with only required adapter vars and omits non-template vars', async () => {
      const capturedRequests: unknown[] = [];

      const slackTemplate = {
        ...mockTemplate,
        variables: {
          OPENAI_API_KEY: {
            default: '',
            targets: ['agent'],
            secret: true,
            optional: false,
            description: 'OpenAI API key',
          },
          SLACK_APP_TOKEN: {
            default: '',
            targets: ['interface.slack'],
            secret: true,
            optional: false,
            description: 'Slack app token',
          },
          SLACK_CONFIG: {
            default: '',
            targets: ['interface.slack'],
            secret: false,
            optional: true,
            description: 'Slack adapter configuration',
          },
        },
      };

      server.use(
        http.post('/api/v1/agents/:account/:name/deployment-template', async ({ request }) => {
          const body = (await request.json().catch(() => ({}))) as Record<string, unknown>;
          return HttpResponse.json(wrapTemplateResponse(slackTemplate, body as { interfaces?: { adapters?: string[] }; variables?: Record<string, { value?: string; ref?: string }> }));
        }),
        http.post('/api/v1/deploy', async ({ request }) => {
          capturedRequests.push(await request.json());
          return HttpResponse.json({
            status: 'deployed',
            name: AGENT,
            build_id: 'a1b2c3d4e5f6',
            k8s_namespace: 'user-abc123',
            deployed_at: new Date().toISOString(),
            resources: [{ kind: 'Deployment', name: AGENT, status: 'created' }],
          });
        }),
      );

      const user = userEvent.setup();
      renderInstallWithAgentsRoute();
      await waitForForm();

      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-required');
      await user.click(screen.getByRole('button', { name: /slack/i }));

      await waitFor(() => {
        expect(screen.getByLabelText('Slack App Token')).toBeInTheDocument();
      });

      // The template does not define this field, so UI should not require/show it.
      expect(screen.queryByLabelText('Slack Bot Token')).not.toBeInTheDocument();

      // Fill only required adapter token; intentionally leave optional config empty.
      await user.type(screen.getByLabelText('Slack App Token'), 'xapp-required');

      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(capturedRequests).toHaveLength(1);
      });

      const payload = capturedRequests[0] as {
        variables?: Record<string, { value?: string }>;
      };
      const variables = payload.variables ?? {};

      expect(variables.OPENAI_API_KEY?.value).toBe('sk-required');
      expect(variables.SLACK_APP_TOKEN?.value).toBe('xapp-required');
      expect(variables.SLACK_BOT_TOKEN).toBeUndefined();
      expect(variables.SLACK_CONFIG?.value ?? '').toBe('');
    });

    it('shows error panel when deploy fails', async () => {
      server.use(
        http.post('/api/v1/deploy', () =>
          HttpResponse.json(
            { error: 'Insufficient quota' },
            { status: 400 },
          ),
        ),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByText('Insufficient quota')).toBeInTheDocument();
      });
    });

    it('shows validation errors from API response', async () => {
      server.use(
        http.post('/api/v1/deploy', () =>
          HttpResponse.json(
            {
              error: 'validation_error',
              validation_errors: [{ field: 'OPENAI_API_KEY', message: 'invalid key format' }],
            },
            { status: 422 },
          ),
        ),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      await user.type(screen.getByLabelText('OpenAI API Key'), 'bad-key');
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByText(/OPENAI_API_KEY: invalid key format/)).toBeInTheDocument();
      });
    });

    it('shows missing credentials error from API response', async () => {
      server.use(
        http.post('/api/v1/deploy', () =>
          HttpResponse.json(
            {
              error: 'missing_variables',
              missing_variables: ['SECRET_TOKEN'],
            },
            { status: 400 },
          ),
        ),
      );

      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByText(/Missing variables: SECRET_TOKEN/)).toBeInTheDocument();
      });
    });
  });

  // ── Server-Side Validation Failure ─────────────────────────────────

  describe('server-side validation failure', () => {
    it('shows error and does not navigate when finalize returns validation.valid=false', async () => {
      // Override the template endpoint to return an invalid validation on finalize.
      server.use(
        http.post('/api/v1/agents/:account/:name/deployment-template', async ({ request }) => {
          const body = (await request.json().catch(() => ({}))) as Record<string, unknown>;
          const resp = wrapTemplateResponse(mockTemplate, body as { interfaces?: { adapters?: string[] }; variables?: Record<string, { value?: string; ref?: string }> });
          // Force validation failure regardless of inputs.
          resp.validation = {
            valid: false,
            errors: [{ field: 'variables.SOME_CRED', message: 'required variable is empty' }],
          };
          return HttpResponse.json(resp);
        }),
      );

      const user = userEvent.setup();
      renderInstallWithAgentsRoute();
      await waitForForm();

      // Fill credentials so client-side validation passes.
      await user.type(screen.getByLabelText('OpenAI API Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      // Should show the validation error, not navigate.
      await waitFor(() => {
        expect(screen.getByText(/Validation failed/)).toBeInTheDocument();
      });
      expect(screen.getByText(/SOME_CRED/)).toBeInTheDocument();
      expect(screen.queryByText('Dashboard Page')).not.toBeInTheDocument();
    });
  });

  // ── Cancel Link ───────────────────────────────────────────────────

  describe('cancel link', () => {
    it('links back to the agent detail page', async () => {
      renderInstall();
      await waitForForm();

      expect(screen.getByRole('link', { name: /cancel/i })).toHaveAttribute(
        'href',
        `/${ACCOUNT}/${AGENT}`,
      );
    });
  });
});

// ── Regression: interface-section gating (open-cohort feature) ────────────────
//
// Messaging support is keyed off the interfaces sidecar image, and the custom
// interface section off an exposed agent endpoint — independently. A custom-only
// agent (no image) must NOT show the Chat section, and a messaging agent must
// not show the Custom interface section. This guards the gate that previously
// regressed when keyed off mere `interfaces` presence.
describe('interface section gating', () => {
  it('shows Messaging interface (not Custom) for a messaging agent', async () => {
    renderInstall();
    await waitFor(() => {
      expect(screen.getByText('Messaging interface')).toBeInTheDocument();
    });
    expect(screen.queryByText('Custom interface')).not.toBeInTheDocument();
  });

  it('shows Custom interface (not Messaging) for a custom-interface-only agent', async () => {
    const customTemplate = {
      ...mockTemplate,
      // No messaging sidecar image → chat section is gated out.
      interfaces: { auth: { custom: { public: false } } },
      // Agent exposes its own endpoint → custom interface section is shown.
      agent: { ...mockTemplate.agent, endpoints: { http: { port: 8080, expose: { enabled: true } } } },
    };
    server.use(
      http.post('/api/v1/agents/:account/:name/deployment-template', async ({ request }) => {
        const body = (await request.json().catch(() => ({}))) as Parameters<typeof wrapTemplateResponse>[1];
        return HttpResponse.json(wrapTemplateResponse(customTemplate, body));
      }),
    );
    renderInstall();
    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 1, name: /Deploy/ })).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByText('Custom interface')).toBeInTheDocument();
    });
    expect(screen.queryByText('Messaging interface')).not.toBeInTheDocument();
  });
});
