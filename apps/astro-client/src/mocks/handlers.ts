import { http, HttpResponse } from 'msw';
import type {
  AuthResponse,
  AgentsListResponse,
  AgentDeployment,
  DeploymentsListResponse,
  DeploymentTemplate,
  DeploymentHistoryResponse,
  ProfileResponse,
  ObservabilityMetricsResponse,
  ObservabilitySummaryResponse,
  ObservabilityTracesResponse,
} from '@/lib/api';

// ── Fake user ────────────────────────────────────────────────────────────────

const MOCK_USER = {
  id: 'mock-user-001',
  email: 'dev@astropods.local',
  first_name: 'Local',
  last_name: 'Developer',
  email_verified: true,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

const MOCK_ACCOUNTS = [
  { id: 'acct-personal', name: 'astro-labs', type: 'personal' as const },
];

const mockAuthResponse: AuthResponse = {
  user: MOCK_USER,
  session_id: 'mock-session-001',
  role: 'admin',
  permissions: ['read', 'write', 'deploy', 'admin'],
  expires_at: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
  accounts: MOCK_ACCOUNTS,
};

// ── Agents (10 across all categories) ────────────────────────────────────────

const mockAgents = [
  {
    name: 'code-reviewer',
    account: 'astro-labs',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'cr-b001',
      spec: {
        meta: {
          description: 'Automated code review agent that analyzes pull requests for quality, security vulnerabilities, and style enforcement.',
          tags: ['Development'],
        },
        integrations: {
          github: { provider: 'GitHub', type: 'tool' },
          slack: { provider: 'Slack', type: 'tool' },
        },
      },
      published_at: '2025-12-15T00:00:00Z',
    }],
  },
  {
    name: 'sql-query-optimizer',
    account: 'astro-labs',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'sql-b001',
      spec: {
        meta: {
          description: 'Analyze and optimize slow database queries with AI-powered index suggestions and automatic rewrites.',
          tags: ['Development'],
        },
        integrations: {
          postgres: { provider: 'PostgreSQL', type: 'tool' },
          datadog: { provider: 'Datadog', type: 'tool' },
        },
      },
      published_at: '2025-11-20T00:00:00Z',
    }],
  },
  {
    name: 'customer-insights-engine',
    account: 'data-team',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'ci-b001',
      spec: {
        meta: {
          description: 'Analyze customer behavior patterns and surface actionable insights from your data pipeline.',
          tags: ['Data'],
        },
        integrations: {
          snowflake: { provider: 'Snowflake', type: 'tool' },
          gsheets: { provider: 'Google Sheets', type: 'tool' },
        },
      },
      published_at: '2025-10-01T00:00:00Z',
    }],
  },
  {
    name: 'pipeline-monitor',
    account: 'data-team',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'pm-b001',
      spec: {
        meta: {
          description: 'Real-time monitoring and anomaly detection for ETL pipelines, data sources, and warehouse freshness.',
          tags: ['Data'],
        },
        integrations: {
          pagerduty: { provider: 'PagerDuty', type: 'tool' },
          slack: { provider: 'Slack', type: 'tool' },
        },
      },
      published_at: '2025-09-10T00:00:00Z',
    }],
  },
  {
    name: 'content-brief-generator',
    account: 'growth-eng',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'cb-b001',
      spec: {
        meta: {
          description: 'Generate SEO-optimized content briefs from keyword research and competitor analysis.',
          tags: ['Marketing'],
        },
        integrations: {
          semrush: { provider: 'Semrush', type: 'tool' },
          notion: { provider: 'Notion', type: 'tool' },
        },
      },
      published_at: '2025-08-22T00:00:00Z',
    }],
  },
  {
    name: 'social-listener',
    account: 'growth-eng',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'sl-b001',
      spec: {
        meta: {
          description: 'Monitor brand mentions and sentiment across social platforms in real time with automated alerts.',
          tags: ['Marketing'],
        },
        integrations: {
          twitter: { provider: 'X (Twitter)', type: 'tool' },
          slack: { provider: 'Slack', type: 'tool' },
        },
      },
      published_at: '2025-07-15T00:00:00Z',
    }],
  },
  {
    name: 'lead-qualifier',
    account: 'rev-ops',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'lq-b001',
      spec: {
        meta: {
          description: 'Score and qualify inbound leads using behavioral signals and firmographic data from your CRM.',
          tags: ['Sales'],
        },
        integrations: {
          salesforce: { provider: 'Salesforce', type: 'tool' },
          hubspot: { provider: 'HubSpot', type: 'tool' },
        },
      },
      published_at: '2025-06-01T00:00:00Z',
    }],
  },
  {
    name: 'outreach-sequencer',
    account: 'rev-ops',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'os-b001',
      spec: {
        meta: {
          description: 'Create personalized multi-channel outreach sequences powered by AI and behavioral data.',
          tags: ['Sales'],
        },
        integrations: {
          gmail: { provider: 'Gmail', type: 'tool' },
          linkedin: { provider: 'LinkedIn', type: 'tool' },
        },
      },
      published_at: '2025-05-18T00:00:00Z',
    }],
  },
  {
    name: 'support-ticket-responder',
    account: 'astro-labs',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'str-b001',
      spec: {
        meta: {
          description: 'Auto-triage and draft responses for incoming support tickets with full context awareness.',
          tags: ['Support'],
        },
        integrations: {
          zendesk: { provider: 'Zendesk', type: 'tool' },
          slack: { provider: 'Slack', type: 'tool' },
          jira: { provider: 'Jira', type: 'tool' },
        },
      },
      published_at: '2025-04-10T00:00:00Z',
    }],
  },
  {
    name: 'incident-responder',
    account: 'astro-labs',
    registry: 'registry.astropods.ai',
    versions: [{
      build_id: 'ir-b001',
      spec: {
        meta: {
          description: 'Automate incident triage, escalation, and post-mortem generation for your on-call team.',
          tags: ['Support'],
        },
        integrations: {
          pagerduty: { provider: 'PagerDuty', type: 'tool' },
          github: { provider: 'GitHub', type: 'tool' },
          slack: { provider: 'Slack', type: 'tool' },
        },
      },
      published_at: '2025-03-05T00:00:00Z',
    }],
  },
];

// ── Deployments (5 with varied statuses) ─────────────────────────────────────

const mockDeployments: AgentDeployment[] = [
  {
    id: 'dep-cr-001',
    name: 'code-reviewer',
    display_name: 'Code Reviewer',
    build_id: 'cr-b001',
    namespace: 'astro-local-dev-cr',
    status: 'Running',
    replicas: 1,
    ready: 1,
    created_at: '2025-12-20T10:00:00Z',
    components: ['deployment', 'service'],
    workloads: [{
      name: 'code-reviewer-7d9f6b',
      kind: 'Deployment',
      component: 'agent',
      age: '94d',
      containers: [{ name: 'agent', state: 'Running', ready: true, restart_count: 0 }],
      urls: [{ name: 'http', url: 'https://code-reviewer.astro-local-dev-cr.svc.cluster.local', type: 'internal' }],
    }],
  },
  {
    id: 'dep-str-001',
    name: 'support-ticket-responder',
    display_name: 'Support Ticket Responder',
    build_id: 'str-b001',
    namespace: 'astro-local-dev-str',
    status: 'Running',
    replicas: 2,
    ready: 2,
    created_at: '2025-11-15T08:30:00Z',
    components: ['deployment', 'service', 'ingestion'],
    workloads: [
      {
        name: 'support-ticket-responder-4c8a2e',
        kind: 'Deployment',
        component: 'agent',
        age: '128d',
        containers: [{ name: 'agent', state: 'Running', ready: true, restart_count: 1 }],
      },
      {
        name: 'support-ticket-responder-ingestion-9b3f1d',
        kind: 'Deployment',
        component: 'ingestion',
        age: '128d',
        containers: [{ name: 'ingestion', state: 'Running', ready: true, restart_count: 0 }],
      },
    ],
  },
  {
    id: 'dep-ci-001',
    name: 'customer-insights-engine',
    display_name: 'Customer Insights Engine',
    build_id: 'ci-b001',
    namespace: 'astro-local-dev-ci',
    status: 'Pending',
    replicas: 1,
    ready: 0,
    created_at: '2026-01-10T14:00:00Z',
    components: ['deployment'],
    workloads: [{
      name: 'customer-insights-engine-2e7c4a',
      kind: 'Deployment',
      component: 'agent',
      age: '2d',
      containers: [{ name: 'agent', state: 'Waiting', ready: false, restart_count: 0, reason: 'ContainerCreating' }],
    }],
  },
  {
    id: 'dep-pm-001',
    name: 'pipeline-monitor',
    display_name: 'Pipeline Monitor',
    build_id: 'pm-b001',
    namespace: 'astro-local-dev-pm',
    status: 'Failed',
    replicas: 1,
    ready: 0,
    created_at: '2025-10-05T09:00:00Z',
    components: ['deployment', 'service'],
    workloads: [{
      name: 'pipeline-monitor-8f1b3c',
      kind: 'Deployment',
      component: 'agent',
      age: '169d',
      containers: [{
        name: 'agent',
        state: 'Waiting',
        ready: false,
        restart_count: 14,
        reason: 'CrashLoopBackOff',
        message: 'Back-off restarting failed container agent in pod pipeline-monitor-8f1b3c',
      }],
    }],
  },
  {
    id: 'dep-lq-001',
    name: 'lead-qualifier',
    display_name: 'Lead Qualifier',
    build_id: 'lq-b001',
    namespace: 'astro-local-dev-lq',
    status: 'Running',
    replicas: 1,
    ready: 1,
    created_at: '2025-09-01T12:00:00Z',
    components: ['deployment', 'service'],
    workloads: [{
      name: 'lead-qualifier-5a2d9e',
      kind: 'Deployment',
      component: 'agent',
      age: '203d',
      containers: [{ name: 'agent', state: 'Running', ready: true, restart_count: 0 }],
    }],
  },
];

// ── Default deployment template (returned for any agent) ─────────────────────

const mockTemplate: DeploymentTemplate = {
  spec: 'deployment-template/v1',
  source: { account: 'astro-labs', name: 'code-reviewer', build: 'cr-b001', registry: 'registry.astropods.ai' },
  target: { runtime: 'kubernetes' },
  agent: { image: 'registry.astropods.ai/astro-labs/code-reviewer:cr-b001', endpoints: { http: { port: 8080 } } },
  variables: {
    OPENAI_API_KEY: { default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI API key' },
  },
  editable: ['variables.*.value', 'target.namespace'],
};

// ── Handlers ─────────────────────────────────────────────────────────────────

export const handlers = [
  // Auth — return mock authenticated user for any /auth/me request
  http.get('*/auth/me', () => {
    return HttpResponse.json(mockAuthResponse);
  }),

  // Auth refresh
  http.post('*/auth/refresh', () => {
    return HttpResponse.json(mockAuthResponse);
  }),

  // Profile
  http.get('/api/v1/me', () => {
    return HttpResponse.json<ProfileResponse>({
      user: MOCK_USER,
      accounts: MOCK_ACCOUNTS,
    });
  }),

  // List agents
  http.get('/api/v1/agents', () => {
    return HttpResponse.json<AgentsListResponse>({
      agents: mockAgents as AgentsListResponse['agents'],
      count: mockAgents.length,
    });
  }),

  // Get single agent
  http.get('/api/v1/agents/:account/:name', ({ params }) => {
    const agent = mockAgents.find(
      (a) => a.account === params.account && a.name === params.name,
    );
    if (!agent) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json(agent);
  }),

  // Deployment template
  http.get('/api/v1/agents/:account/:name/deployment-template', ({ params }) => {
    const agent = mockAgents.find(
      (a) => a.account === params.account && a.name === params.name,
    );
    if (!agent) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json({
      ...mockTemplate,
      source: {
        ...mockTemplate.source,
        account: agent.account,
        name: agent.name,
        build: agent.versions[0].build_id,
      },
    });
  }),

  // List deployments (filtered by ?account= if provided)
  http.get('/api/v1/deployments', ({ request }) => {
    const account = new URL(request.url).searchParams.get('account');
    const filtered = account
      ? mockDeployments.filter((d) => mockAgents.find((a) => a.name === d.name && a.account === account))
      : mockDeployments;
    return HttpResponse.json<DeploymentsListResponse>({ deployments: filtered, count: filtered.length });
  }),

  // Deployment logs
  http.get('/api/v1/deployments/:deploymentId/logs', ({ params, request }) => {
    const url = new URL(request.url);
    const workload = url.searchParams.get('workload') ?? 'agent';
    const deployment = mockDeployments.find((d) => d.id === params.deploymentId);
    const isFailed = deployment?.status === 'Failed';
    const now = new Date();
    const lines = isFailed ? [
      `${new Date(now.getTime() - 120000).toISOString()} INFO  Starting agent process...`,
      `${new Date(now.getTime() - 118000).toISOString()} INFO  Loading configuration from /etc/agent/config.yaml`,
      `${new Date(now.getTime() - 116000).toISOString()} INFO  Connecting to PostgreSQL at db.internal:5432`,
      `${new Date(now.getTime() - 114000).toISOString()} ERROR Failed to connect to database: connection refused`,
      `${new Date(now.getTime() - 113000).toISOString()} ERROR dial tcp db.internal:5432: connect: connection refused`,
      `${new Date(now.getTime() - 112000).toISOString()} FATAL Agent process exited with code 1`,
    ] : [
      `${new Date(now.getTime() - 300000).toISOString()} INFO  Starting ${workload} process...`,
      `${new Date(now.getTime() - 299000).toISOString()} INFO  Loading configuration from /etc/agent/config.yaml`,
      `${new Date(now.getTime() - 298000).toISOString()} INFO  Registered 4 tools: search, read_file, write_file, http_request`,
      `${new Date(now.getTime() - 297000).toISOString()} INFO  HTTP server listening on :8080`,
      `${new Date(now.getTime() - 240000).toISOString()} INFO  [trace:tr-0041] Received request`,
      `${new Date(now.getTime() - 239800).toISOString()} INFO  [trace:tr-0041] Invoking tool: search query="quarterly earnings report"`,
      `${new Date(now.getTime() - 239200).toISOString()} INFO  [trace:tr-0041] Tool returned 12 results in 612ms`,
      `${new Date(now.getTime() - 238000).toISOString()} INFO  [trace:tr-0041] Generating response (model=claude-3-5-sonnet, tokens_in=1842)`,
      `${new Date(now.getTime() - 236400).toISOString()} INFO  [trace:tr-0041] Response complete tokens_out=394 latency=1601ms`,
      `${new Date(now.getTime() - 180000).toISOString()} INFO  [trace:tr-0042] Received request`,
      `${new Date(now.getTime() - 179700).toISOString()} WARN  [trace:tr-0042] Tool call failed: http_request status=429 retrying in 2s`,
      `${new Date(now.getTime() - 177200).toISOString()} INFO  [trace:tr-0042] Retry succeeded`,
      `${new Date(now.getTime() - 176000).toISOString()} INFO  [trace:tr-0042] Response complete tokens_out=218 latency=3012ms`,
      `${new Date(now.getTime() - 60000).toISOString()}  INFO  [trace:tr-0043] Received request`,
      `${new Date(now.getTime() - 59800).toISOString()}  INFO  [trace:tr-0043] Generating response (model=claude-3-5-sonnet, tokens_in=924)`,
      `${new Date(now.getTime() - 58900).toISOString()}  INFO  [trace:tr-0043] Response complete tokens_out=187 latency=901ms`,
    ];
    return new HttpResponse(lines.join('\n'), { headers: { 'Content-Type': 'text/plain' } });
  }),

  // Deployment history
  http.get('/api/v1/agents/:account/:name/deployment/history', ({ params }) => {
    const agentName = params.name as string;
    const deployment = mockDeployments.find((d) => d.name === agentName);
    if (!deployment) {
      return HttpResponse.json<DeploymentHistoryResponse>({ deployments: [], count: 0 });
    }
    const records = [
      {
        id: deployment.id,
        agent_name: agentName,
        build_id: deployment.build_id,
        namespace: deployment.namespace,
        status: deployment.status,
        deployed_at: deployment.created_at,
        spec: {},
      },
      {
        id: `${deployment.id}-prev`,
        agent_name: agentName,
        build_id: `${deployment.build_id.replace(/\d+$/, '')}000`,
        namespace: deployment.namespace,
        status: 'Undeployed',
        deployed_at: new Date(new Date(deployment.created_at).getTime() - 14 * 24 * 60 * 60 * 1000).toISOString(),
        undeployed_at: deployment.created_at,
        spec: {},
      },
      {
        id: `${deployment.id}-init`,
        agent_name: agentName,
        build_id: `${deployment.build_id.replace(/\d+$/, '')}pre`,
        namespace: deployment.namespace,
        status: 'Undeployed',
        deployed_at: new Date(new Date(deployment.created_at).getTime() - 30 * 24 * 60 * 60 * 1000).toISOString(),
        undeployed_at: new Date(new Date(deployment.created_at).getTime() - 14 * 24 * 60 * 60 * 1000).toISOString(),
        spec: {},
      },
    ];
    return HttpResponse.json<DeploymentHistoryResponse>({ deployments: records, count: records.length });
  }),

  // Observability — metrics (time-bucketed chart data)
  http.get('/api/v1/deployments/:deploymentId/observability/metrics', ({ request }) => {
    const url = new URL(request.url);
    const windowHours = parseInt(url.searchParams.get('window_hours') ?? '24', 10);
    const bucketCount = windowHours <= 1 ? 12 : windowHours <= 24 ? 24 : 28;
    const intervalMinutes = windowHours <= 1 ? 5 : windowHours <= 24 ? 60 : 360;
    const now = Date.now();
    const buckets = Array.from({ length: bucketCount }, (_, i) => {
      const ts = new Date(now - (bucketCount - i) * intervalMinutes * 60 * 1000).toISOString();
      const base = 40 + Math.sin(i / 3) * 20;
      const spike = i === Math.floor(bucketCount * 0.6) ? 80 : 0;
      const traceCount = Math.max(0, Math.round(base + spike + Math.random() * 10));
      return {
        timestamp: ts,
        trace_count: traceCount,
        avg_latency_ms: Math.round(320 + Math.sin(i / 4) * 80 + Math.random() * 40),
        p95_latency_ms: Math.round(720 + Math.sin(i / 4) * 150 + Math.random() * 80),
        input_tokens: traceCount * Math.round(180 + Math.random() * 60),
        output_tokens: traceCount * Math.round(420 + Math.random() * 100),
        error_count: Math.random() < 0.15 ? Math.floor(Math.random() * 3) + 1 : 0,
      };
    });
    const start = buckets[0].timestamp;
    const end = buckets[buckets.length - 1].timestamp;
    return HttpResponse.json<ObservabilityMetricsResponse>({ buckets, time_range: { start, end }, interval_minutes: intervalMinutes });
  }),

  // Observability — summary (headline KPIs)
  http.get('/api/v1/deployments/:deploymentId/observability/summary', ({ request }) => {
    const url = new URL(request.url);
    const windowHours = parseInt(url.searchParams.get('window_hours') ?? '24', 10);
    const multiplier = windowHours <= 1 ? 1 : windowHours <= 24 ? 18 : 120;
    const now = Date.now();
    const start = new Date(now - windowHours * 60 * 60 * 1000).toISOString();
    const end = new Date(now).toISOString();
    return HttpResponse.json<ObservabilitySummaryResponse>({
      total_traces: Math.round(52 * multiplier),
      time_range: { start, end },
      metrics: {
        avg_latency_ms: 347,
        p95_latency_ms: 812,
        total_tokens: Math.round(34800 * multiplier),
        error_rate: 0.032,
        traces_per_hour: 52,
      },
    });
  }),

  // Observability — traces (paginated list)
  http.get('/api/v1/deployments/:deploymentId/observability/traces', ({ request }) => {
    const url = new URL(request.url);
    const limit = parseInt(url.searchParams.get('limit') ?? '20', 10);
    const offset = parseInt(url.searchParams.get('offset') ?? '0', 10);
    const now = Date.now();
    const TRACE_NAMES = [
      'process_user_query', 'fetch_context', 'generate_response',
      'tool_call:search', 'tool_call:read_file', 'summarize_document',
      'classify_intent', 'route_request', 'validate_output',
    ];
    const STATUSES = ['success', 'success', 'success', 'success', 'error', 'timeout'] as const;
    const SAMPLE_INPUTS = [
      `Analyze the latest quarterly report and summarize key risks.\n\n**Context:** We're presenting to the board on Friday. Focus on financial risks and any operational issues flagged by auditors. Keep it under 300 words.`,
      `What were the top 5 support tickets last week?\n\n- Include ticket volume and resolution status\n- Flag anything still open or escalated\n- Compare to the previous week if possible`,
      `Draft a follow-up email for the Acme Corp deal.\n\n**Deal context:**\n- Stage: Proposal sent 6 days ago\n- Contact: Marcus Chen, CTO\n- Key asks: custom SLA, migration support, enterprise pricing\n- Last touch: intro call on Thursday`,
      `Review this pull request for security issues.\n\n\`\`\`\nPR #4821 — feat: add user search endpoint\nFiles changed: 12\nAdditions: +847  Deletions: -23\n\`\`\`\n\nFocus on: input validation, auth checks, and any data exposure risks.`,
      `Generate a content brief for "AI in healthcare" targeting CTOs.\n\n**Goals:**\n1. Drive demo signups from health system leadership\n2. Address common objections (HIPAA, hallucination risk)\n3. Differentiate from generic AI tools\n\n**Format:** Long-form article, 1,400–1,800 words`,
    ];
    const SAMPLE_OUTPUTS = [
      `## Q3 Risk Summary\n\nRevenue growth decelerated to **8% YoY** (down from 14% in Q2). Three key risk areas identified:\n\n1. **Customer churn** — NRR dropped to 104%, lowest since Q1 2024\n2. **Burn rate** — runway now at 14 months assuming flat ARR\n3. **Market concentration** — top 3 customers represent 41% of ARR\n\n> Recommend immediate review of enterprise retention playbook before Q4 QBRs.`,
      `### Top 5 Support Tickets (Last 7 Days)\n\n| # | Issue | Volume | Status |\n|---|-------|--------|--------|\n| 1 | Login failures after SSO update | 142 | Resolved |\n| 2 | API rate limit errors on \`/v2/ingest\` | 87 | In progress |\n| 3 | Webhook delivery delays >30s | 54 | Monitoring |\n| 4 | CSV export encoding bug (UTF-8) | 31 | Fixed in 2.4.1 |\n| 5 | Mobile app crash on iOS 17.4 | 28 | Escalated |\n\nTotal tickets opened: **342** — up 18% WoW.`,
      `Subject: Following up on Acme Corp proposal\n\nHi Marcus,\n\nWanted to circle back on the proposal we discussed last Thursday. A few things worth highlighting:\n\n- **Custom SLA** — we can offer 99.95% uptime with a 4-hour response window\n- **Migration support** — our team can handle the data migration from your current vendor at no extra cost\n- **Pricing** — happy to revisit the enterprise tier given your projected seat count\n\nLet me know if a 30-min call this week works. I can send a revised deck beforehand.\n\nBest,\nAlex`,
      "## Security Review — PR #4821\n\n### Critical\n- **SQL injection risk** on `/api/search` (line 142) — user input passed directly to query builder without sanitization\n\n### High\n- Missing `Authorization` check on `DELETE /api/v1/users/:id` — any authenticated user can delete any account\n\n### Medium\n- `console.log` statements leak internal stack traces (lines 88, 203, 319)\n- Session token stored in `localStorage` — should use `httpOnly` cookies\n\n```ts\n// line 142 — vulnerable\nconst results = db.query(`SELECT * FROM items WHERE name = '${req.query.q}'`);\n\n// fix\nconst results = db.query('SELECT * FROM items WHERE name = $1', [req.query.q]);\n```",
      `# Content Brief: AI in Healthcare\n\n**Primary keyword:** AI in healthcare  \n**Secondary:** clinical AI, hospital automation, medical AI tools  \n**Target audience:** CTOs at health systems (500–5,000 beds)\n\n## Recommended Structure\n\n1. **Hook** — a specific statistic on diagnostic error rates or admin burden\n2. **Problem** — where manual processes create the most friction today\n3. **Solution framing** — AI as infrastructure, not a feature\n4. **3 use cases** — prior auth, clinical documentation, supply chain\n5. **Objections** — HIPAA, hallucination risk, integration cost\n6. **CTA** — product demo or ROI calculator\n\n**Suggested word count:** 1,400–1,800  \n**Reading level:** Grade 12 (Flesch-Kincaid)`,
    ];
    const total = 148;
    const traces = Array.from({ length: Math.min(limit, total - offset) }, (_, i) => {
      const idx = offset + i;
      const status = STATUSES[idx % STATUSES.length];
      return {
        trace_id: `tr-${String(idx + 1).padStart(4, '0')}`,
        name: TRACE_NAMES[idx % TRACE_NAMES.length],
        status,
        latency_ms: status === 'timeout' ? 30000 : status === 'error' ? Math.round(150 + Math.random() * 200) : Math.round(200 + Math.random() * 800),
        total_tokens: status === 'error' ? undefined : Math.round(400 + Math.random() * 600),
        input: SAMPLE_INPUTS[idx % SAMPLE_INPUTS.length],
        output: status === 'error' ? 'Error: upstream tool returned 500' : status === 'timeout' ? 'Error: execution timed out after 30s' : SAMPLE_OUTPUTS[idx % SAMPLE_OUTPUTS.length],
        timestamp: new Date(now - (idx * 4 + Math.random() * 2) * 60 * 1000).toISOString(),
      };
    });
    return HttpResponse.json<ObservabilityTracesResponse>({ traces, total, limit, offset });
  }),
];
