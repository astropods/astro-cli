import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { mockTemplate } from '@/test/msw/handlers';
import { renderRoute } from '@/test/test-utils';
import DeployBlueprint from './DeployBlueprint';

afterEach(cleanup);

const ROUTE_PATH = '/:account/:agentSlug/install';
const ACCOUNT = 'testuser';
const AGENT = 'code-reviewer';

function renderInstall({ account = ACCOUNT, agent = AGENT } = {}) {
  return renderRoute(
    [
      {
        path: ROUTE_PATH,
        // @ts-expect-error: `matches` won't align between test code and app code
        Component: DeployBlueprint,
      },
    ],
    { initialEntries: [`/${account}/${agent}/install`] },
  );
}

function renderInstallWithAgentsRoute() {
  return renderRoute(
    [
      {
        path: ROUTE_PATH,
        // @ts-expect-error: `matches` won't align between test code and app code
        Component: DeployBlueprint,
      },
      {
        path: '/dashboard',
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
    expect(screen.getByText('Messaging')).toBeInTheDocument();
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
  });

  // ── Template Error ──────────────────────────────────────────────────

  describe('template error', () => {
    it('shows error panel when template fails to load', async () => {
      server.use(
        http.get('/api/v1/agents/:account/:name/deployment-template', () =>
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
      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');

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
      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');

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

      const webButton = screen.getByRole('button', { name: /web/i });
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

  // ── Credential Fields ─────────────────────────────────────────────

  describe('credential fields', () => {
    it('renders required credential fields from template', async () => {
      renderInstall();
      await waitForForm();

      expect(screen.getByText('Configuration')).toBeInTheDocument();
      expect(screen.getByLabelText('Openai Api Key')).toBeInTheDocument();
    });

    it('renders optional credential fields from template', async () => {
      renderInstall();
      await waitForForm();

      expect(screen.getByText('Optional credentials')).toBeInTheDocument();
      expect(screen.getByLabelText('Sentry Dsn')).toBeInTheDocument();
    });

    it('hides sections when template has no credentials', async () => {
      server.use(
        http.get('/api/v1/agents/:account/:name/deployment-template', () =>
          HttpResponse.json({ ...mockTemplate, variables: {} }),
        ),
      );

      renderInstall();
      await waitForForm();

      expect(screen.queryByText('Configuration')).not.toBeInTheDocument();
      expect(screen.queryByText('Optional credentials')).not.toBeInTheDocument();
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
        expect(screen.getByLabelText('Openai Api Key')).toHaveAttribute('aria-invalid', 'true');
      });
      expect(screen.getByText('Required')).toBeInTheDocument();
    });

    it('does not show inline errors before first submit attempt', async () => {
      renderInstall();
      await waitForForm();

      expect(screen.getByLabelText('Openai Api Key')).not.toHaveAttribute('aria-invalid');
      expect(screen.queryByText('Required')).not.toBeInTheDocument();
    });

    it('clears credential errors when fields are filled after submit', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Submit with empty fields
      await user.click(screen.getByRole('button', { name: /deploy/i }));
      await waitFor(() => {
        expect(screen.getByLabelText('Openai Api Key')).toHaveAttribute('aria-invalid', 'true');
      });

      // Fill the field
      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');

      await waitFor(() => {
        expect(screen.getByLabelText('Openai Api Key')).not.toHaveAttribute('aria-invalid');
      });
      expect(screen.queryByText('Required')).not.toBeInTheDocument();
    });

    it('shows messaging error when all types are deselected and form is submitted', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Fill credentials so that's not the issue
      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');

      // Deselect Web
      await user.click(screen.getByRole('button', { name: /web/i }));

      // No error yet (haven't submitted)
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();

      // Submit
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByRole('alert')).toHaveTextContent('Select at least one messaging type');
      });
    });

    it('clears messaging error when a type is reselected after submit', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /web/i }));
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByRole('alert')).toBeInTheDocument();
      });

      // Reselect Web
      await user.click(screen.getByRole('button', { name: /web/i }));

      await waitFor(() => {
        expect(screen.queryByRole('alert')).not.toBeInTheDocument();
      });
    });

    it('shows inline errors on Slack credentials when submitted with empty tokens', async () => {
      const user = userEvent.setup();
      renderInstall();
      await waitForForm();

      // Fill agent credential
      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');

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
        expect(screen.getByLabelText('Openai Api Key')).toHaveAttribute('aria-invalid', 'true');
      });
      expect(capturedRequests).toHaveLength(0);
    });
  });

  // ── Deployment Submission ─────────────────────────────────────────

  describe('deployment submission', () => {
    it('navigates to /dashboard on successful deploy', async () => {
      const user = userEvent.setup();
      renderInstallWithAgentsRoute();
      await waitForForm();

      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');
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
      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');

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

      server.use(
        http.get('/api/v1/agents/:account/:name/deployment-template', () =>
          HttpResponse.json({
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
          }),
        ),
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

      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-required');
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

      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');
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

      await user.type(screen.getByLabelText('Openai Api Key'), 'bad-key');
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

      await user.type(screen.getByLabelText('Openai Api Key'), 'sk-test123');
      await user.click(screen.getByRole('button', { name: /deploy/i }));

      await waitFor(() => {
        expect(screen.getByText(/Missing variables: SECRET_TOKEN/)).toBeInTheDocument();
      });
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
