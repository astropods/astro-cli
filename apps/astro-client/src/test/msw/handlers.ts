import { http, HttpResponse } from 'msw';
import type {
  AgentsListResponse,
  Agent,
  DeploymentsListResponse,
  DeployResponse,
  UndeployResponse,
} from '@/lib/api';

// Fixture data — realistic but minimal
export const mockAgents: Agent[] = [
  {
    name: 'code-reviewer',
    account: 'testuser',
    registry: 'registry.example.com',
    versions: [
      {
        build_id: 'a1b2c3d4e5f6',
        spec: {
          model: 'gpt-4o',
          meta: {
            description: 'Automated code review agent that analyzes pull requests for quality and security issues.',
            tags: ['Developer Tools', 'Security'],
          },
          integrations: {
            github: { provider: 'GitHub', type: 'tool' },
            slack: { provider: 'Slack', type: 'tool' },
          },
        },
        published_at: '2025-02-01T00:00:00Z',
      },
      {
        build_id: 'b2c3d4e5f6a7',
        spec: {
          model: 'gpt-4',
          meta: {
            description: 'Automated code review agent.',
            tags: ['Developer Tools'],
          },
        },
        published_at: '2025-01-01T00:00:00Z',
      },
    ],
  },
  {
    name: 'data-analyst',
    account: 'testuser',
    registry: 'registry.example.com',
    versions: [
      {
        build_id: 'c3d4e5f6a7b8',
        spec: {
          model: 'claude-3',
          meta: {
            description: 'Analyzes datasets and generates visual reports with actionable insights.',
            tags: ['Analytics'],
          },
          integrations: {
            snowflake: { provider: 'Snowflake', type: 'tool' },
            slack: { provider: 'Slack', type: 'tool' },
            gsheets: { provider: 'Google Sheets', type: 'tool' },
          },
        },
        published_at: '2025-03-01T00:00:00Z',
      },
    ],
  },
];

export const mockDeployments: DeploymentsListResponse = {
  deployments: [
    {
      name: 'code-reviewer',
      build_id: 'b2c3d4e5f6a7',
      namespace: 'astro-abc123def456',
      status: 'Running',
      replicas: 1,
      ready: 1,
      created_at: '2025-04-01T00:00:00Z',
      components: ['deployment', 'service'],
    },
  ],
  count: 1,
};

export const handlers = [
  // GET /api/v1/agents
  http.get('/api/v1/agents', () => {
    return HttpResponse.json<AgentsListResponse>({
      agents: mockAgents,
      count: mockAgents.length,
    });
  }),

  // GET /api/v1/agents/:account/:name
  http.get('/api/v1/agents/:account/:name', ({ params }) => {
    const agent = mockAgents.find((a) => a.account === params.account && a.name === params.name);
    if (!agent) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json(agent);
  }),

  // GET /api/v1/deployments
  http.get('/api/v1/deployments', () => {
    return HttpResponse.json(mockDeployments);
  }),

  // POST /api/v1/deploy
  http.post('/api/v1/deploy', async ({ request }) => {
    const body = (await request.json()) as { name: string; build_id: string };
    return HttpResponse.json<DeployResponse>({
      status: 'deployed',
      name: body.name,
      build_id: body.build_id || 'a1b2c3d4e5f6',
      k8s_namespace: 'user-abc123',
      deployed_at: new Date().toISOString(),
      resources: [{ kind: 'Deployment', name: body.name, status: 'created' }],
    });
  }),

  // POST /api/v1/undeploy
  http.post('/api/v1/undeploy', async ({ request }) => {
    const body = (await request.json()) as { name: string };
    return HttpResponse.json<UndeployResponse>({
      status: 'undeployed',
      name: body.name,
      build_id: 'a1b2c3d4e5f6',
      k8s_namespace: 'user-abc123',
      undeployed_at: new Date().toISOString(),
      resources: [{ kind: 'Deployment', name: body.name, status: 'deleted' }],
    });
  }),
];
