import { describe, it, expect, vi, afterEach } from 'vitest';
import { screen, cleanup, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderWithProviders } from '@/test/test-utils';
import { ConfigurePanel } from './ConfigurePanel';
import type { AgentDeployment, DeploymentTemplate } from '@/lib/api';
import { wrapTemplateResponse } from '@/test/msw/handlers';

afterEach(cleanup);
afterEach(() => server.resetHandlers());

// Deployment currently on the OLD build
const OLD_BUILD_ID = 'b2c3d4e5f6a7';
const NEW_BUILD_ID = 'a1b2c3d4e5f6';

const mockDeployment: AgentDeployment = {
  id: 'dep-code-reviewer',
  name: 'code-reviewer',
  display_name: 'Code Reviewer',
  build_id: OLD_BUILD_ID,
  namespace: 'astro-abc123',
  status: 'Running',
  replicas: 1,
  ready: 1,
  created_at: '2025-04-01T00:00:00Z',
  components: ['deployment', 'service'],
};

// Template the server returns — source.build reflects the requested build
const mockTemplateOldBuild: DeploymentTemplate = {
  spec: 'deployment-template/v1',
  source: { account: 'testuser', name: 'code-reviewer', build: OLD_BUILD_ID, registry: 'registry.example.com' },
  target: { runtime: 'kubernetes', display_name: 'Code Reviewer', deployment_id: 'dep-code-reviewer' },
  agent: {},
  variables: {},
  editable: ['variables.*.value'],
};

const mockTemplateNewBuild: DeploymentTemplate = {
  ...mockTemplateOldBuild,
  source: { ...mockTemplateOldBuild.source, build: NEW_BUILD_ID },
};

function setupTemplateHandler() {
  server.use(
    http.post('/api/v1/agents/:account/:name/deployment-template', async ({ request }) => {
      const body = (await request.json().catch(() => ({}))) as { build?: string; deployment_id?: string; interfaces?: { adapters?: string[] }; variables?: Record<string, { value?: string; ref?: string }> };
      const tmpl = body.build === NEW_BUILD_ID ? mockTemplateNewBuild : mockTemplateOldBuild;
      return HttpResponse.json(wrapTemplateResponse(tmpl, body));
    }),
  );
}

function renderPanel(props: Partial<React.ComponentProps<typeof ConfigurePanel>> = {}) {
  return renderWithProviders(
    <ConfigurePanel
      deployment={mockDeployment}
      account="testuser"
      onClose={vi.fn()}
      {...props}
    />,
  );
}

describe('ConfigurePanel — new build upgrade', () => {
  it('shows the Update banner with old→new build hashes when isNewBuild and newBuildId are set', async () => {
    setupTemplateHandler();

    renderPanel({ isNewBuild: true, newBuildId: NEW_BUILD_ID });

    await waitFor(() => expect(screen.getByText('Update')).toBeInTheDocument());

    expect(screen.getByText(OLD_BUILD_ID.slice(0, 8))).toBeInTheDocument();
    expect(screen.getByText('→')).toBeInTheDocument();
    expect(screen.getByText(NEW_BUILD_ID.slice(0, 8))).toBeInTheDocument();
  });

  it('does not show the Update banner without isNewBuild', async () => {
    setupTemplateHandler();

    renderPanel();

    await waitFor(() => expect(screen.getByText('Configure')).toBeInTheDocument());

    expect(screen.queryByText('Update')).not.toBeInTheDocument();
    expect(screen.queryByText('→')).not.toBeInTheDocument();
  });

  it('patches template.source.build with newBuildId before deploying', async () => {
    setupTemplateHandler();

    let capturedBuildId: string | undefined;
    server.use(
      http.post('/api/v1/deploy', async ({ request }) => {
        const body = (await request.json()) as { source?: { build?: string } };
        capturedBuildId = body.source?.build;
        return HttpResponse.json({
          status: 'deployed',
          name: 'code-reviewer',
          build_id: NEW_BUILD_ID,
          k8s_namespace: 'astro-abc123',
          deployed_at: new Date().toISOString(),
          resources: [],
        });
      }),
    );

    const user = userEvent.setup();
    renderPanel({ isNewBuild: true, newBuildId: NEW_BUILD_ID });

    await waitFor(() => expect(screen.getByRole('button', { name: /redeploy/i })).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: /redeploy/i }));

    await waitFor(() => expect(capturedBuildId).toBe(NEW_BUILD_ID));
  });

  it('does not patch template.source.build when newBuildId is not provided', async () => {
    setupTemplateHandler();

    renderPanel({ isNewBuild: false });

    // Banner absent means template wasn't patched — no build upgrade in flight
    await waitFor(() => expect(screen.getByText('Configure')).toBeInTheDocument());
    expect(screen.queryByText('→')).not.toBeInTheDocument();
    expect(screen.queryByText(NEW_BUILD_ID.slice(0, 8))).not.toBeInTheDocument();
  });
});
