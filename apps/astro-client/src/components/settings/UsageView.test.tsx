import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { UsageView } from './UsageView';

afterEach(cleanup);

function renderUsageView(props: { account: string; canRequestIncrease?: boolean }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <UsageView {...props} />
    </QueryClientProvider>
  );
}

const usageWithQuota = {
  account_id: 'acct-1',
  period_start: '2025-01-01T00:00:00Z',
  period_end: '2025-02-01T00:00:00Z',
  meters: {
    compute: { usage: 50, quota: 100 },
    agents: { usage: 3, quota: 10 },
  },
};

describe('UsageView', () => {
  it('renders the section header', async () => {
    renderUsageView({ account: 'testuser' });
    expect(await screen.findByText('Usage')).toBeInTheDocument();
  });

  it('shows billing period', async () => {
    renderUsageView({ account: 'testuser' });
    expect(await screen.findByText(/current billing period/i)).toBeInTheDocument();
  });

  it('renders a stat card for each meter', async () => {
    renderUsageView({ account: 'testuser' });
    expect(await screen.findByText('Compute')).toBeInTheDocument();
  });

  describe('canRequestIncrease', () => {
    it('shows Request increase button when canRequestIncrease=true', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () => HttpResponse.json(usageWithQuota))
      );
      renderUsageView({ account: 'testuser', canRequestIncrease: true });
      const buttons = await screen.findAllByText('Request increase');
      expect(buttons.length).toBeGreaterThan(0);
    });

    it('hides Request increase button when canRequestIncrease=false', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () => HttpResponse.json(usageWithQuota))
      );
      renderUsageView({ account: 'testuser', canRequestIncrease: false });
      // Wait for meters to render, then assert button is absent
      expect(await screen.findByText('Compute')).toBeInTheDocument();
      expect(screen.queryByText('Request increase')).not.toBeInTheDocument();
    });

    it('shows Request increase by default (canRequestIncrease defaults to true)', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () => HttpResponse.json(usageWithQuota))
      );
      renderUsageView({ account: 'testuser' });
      const buttons = await screen.findAllByText('Request increase');
      expect(buttons.length).toBeGreaterThan(0);
    });

    it('does not show Request increase for meters without a quota', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () =>
          HttpResponse.json({
            ...usageWithQuota,
            meters: { agents: { usage: 3 } }, // no quota
          })
        )
      );
      renderUsageView({ account: 'testuser', canRequestIncrease: true });
      expect(await screen.findByRole('heading', { name: 'Agents' })).toBeInTheDocument();
      expect(screen.queryByText('Request increase')).not.toBeInTheDocument();
    });
  });

  describe('Full tag', () => {
    it('shows Full tag when usage equals quota', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () =>
          HttpResponse.json({ ...usageWithQuota, meters: { compute: { usage: 100, quota: 100 } } })
        )
      );
      renderUsageView({ account: 'testuser' });
      expect(await screen.findByText('Full')).toBeInTheDocument();
    });

    it('does not show Full tag when usage is below quota', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () =>
          HttpResponse.json({ ...usageWithQuota, meters: { compute: { usage: 50, quota: 100 } } })
        )
      );
      renderUsageView({ account: 'testuser' });
      expect(await screen.findByText('Compute')).toBeInTheDocument();
      expect(screen.queryByText('Full')).not.toBeInTheDocument();
    });
  });

  describe('CU tooltip', () => {
    it('renders the info icon for CU-hours meters', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () =>
          HttpResponse.json({ ...usageWithQuota, meters: { compute: { usage: 50, quota: 100 } } })
        )
      );
      const { container } = render(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <UsageView account="testuser" />
        </QueryClientProvider>
      );
      await screen.findByText('Compute');
      expect(container.querySelector('.lucide-info')).toBeInTheDocument();
    });

    it('does not render the info icon for non-CU-hours meters', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () =>
          HttpResponse.json({ ...usageWithQuota, meters: { agents: { usage: 3, quota: 10 } } })
        )
      );
      const { container } = render(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <UsageView account="testuser" />
        </QueryClientProvider>
      );
      await screen.findByRole('heading', { name: 'Agents' });
      expect(container.querySelector('.lucide-info')).not.toBeInTheDocument();
    });
  });

  describe('category grouping', () => {
    it('renders Knowledge and Account sections for their respective meters', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () =>
          HttpResponse.json({
            ...usageWithQuota,
            meters: {
              compute: { usage: 10, quota: 100 },
              knowledge_stores: { usage: 2, quota: 5 },
              members: { usage: 4, quota: 10 },
            },
          })
        )
      );
      renderUsageView({ account: 'testuser' });
      expect(await screen.findByRole('heading', { name: 'Agents' })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'Knowledge' })).toBeInTheDocument();
      expect(screen.getByRole('heading', { name: 'Account' })).toBeInTheDocument();
    });

    it('buckets unknown meter keys into an Other section', async () => {
      server.use(
        http.get('/api/v1/accounts/:account/usage', () =>
          HttpResponse.json({
            ...usageWithQuota,
            meters: { storage_bandwidth: { usage: 7, quota: 50 } },
          })
        )
      );
      renderUsageView({ account: 'testuser' });
      expect(await screen.findByRole('heading', { name: 'Other' })).toBeInTheDocument();
      expect(screen.getByText('storage_bandwidth')).toBeInTheDocument();
    });
  });
});
