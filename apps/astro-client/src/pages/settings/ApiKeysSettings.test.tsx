import { describe, it, expect, afterEach } from 'vitest';
import { screen, waitFor, within, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/msw/server';
import { renderWithProviders } from '@/test/test-utils';
import { IngestKeysPanel } from './ApiKeysSettings';

afterEach(cleanup);

const key = {
  id: 'key-1',
  name: 'Engineering laptops',
  token_prefix: 'astotel_abcd1234',
  created_at: '2026-01-01T00:00:00Z',
  excluded_emails: ['old@x.com'],
};

function listReturns(tokens: unknown[]) {
  server.use(
    http.get('/api/v1/accounts/:account/otel-keys', () =>
      HttpResponse.json({ tokens, endpoint: 'https://ingest.example.com' }),
    ),
  );
}

describe('IngestKeysPanel exclusions', () => {
  it('edits exclusions from the menu without revealing the key', async () => {
    listReturns([key]);
    let patchBody: { excluded_emails: string[] } | null = null;
    server.use(
      http.patch('/api/v1/accounts/:account/otel-keys/:id/exclusions', async ({ request }) => {
        patchBody = (await request.json()) as { excluded_emails: string[] };
        return HttpResponse.json({ excluded_emails: patchBody.excluded_emails });
      }),
    );

    const user = userEvent.setup();
    renderWithProviders(<IngestKeysPanel account="myorg" />);

    // Count badge reflects the stored exclusions.
    const row = (await screen.findByText('Engineering laptops')).closest('tr')!;
    expect(within(row).getByText('1 excluded')).toBeInTheDocument();

    // Open the row menu and the edit dialog.
    await user.click(within(row).getByRole('button'));
    await user.click(await screen.findByText('Edit exclusions'));

    // Existing exclusion is prefilled; the secret key is never shown here.
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('old@x.com')).toBeInTheDocument();
    expect(within(dialog).queryByText(/astotel_/)).toBeNull();

    // Add a second email and save.
    await user.type(within(dialog).getByPlaceholderText('person@company.com'), 'new@x.com');
    await user.click(within(dialog).getByRole('button', { name: 'Add' }));
    await user.click(within(dialog).getByRole('button', { name: /^save$/i }));

    await waitFor(() => expect(patchBody).toEqual({ excluded_emails: ['old@x.com', 'new@x.com'] }));
  });

  it('rejects an invalid email in the editor', async () => {
    listReturns([key]);
    const user = userEvent.setup();
    renderWithProviders(<IngestKeysPanel account="myorg" />);

    const row = (await screen.findByText('Engineering laptops')).closest('tr')!;
    await user.click(within(row).getByRole('button'));
    await user.click(await screen.findByText('Edit exclusions'));

    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByPlaceholderText('person@company.com'), 'not-an-email');
    await user.click(within(dialog).getByRole('button', { name: 'Add' }));

    expect(within(dialog).getByText('Enter a valid email address.')).toBeInTheDocument();
  });

  it('creates a key by name only, then sets exclusions in the reveal dialog', async () => {
    listReturns([]);
    let postBody: { name: string } | null = null;
    let patchBody: { excluded_emails: string[] } | null = null;
    server.use(
      http.post('/api/v1/accounts/:account/otel-keys', async ({ request }) => {
        postBody = (await request.json()) as { name: string };
        return HttpResponse.json(
          {
            id: 'key-2',
            name: postBody.name,
            token_prefix: 'astotel_new',
            created_at: '2026-01-02T00:00:00Z',
            excluded_emails: [],
            token: 'astotel_secretplaintext',
            endpoint: 'https://ingest.example.com',
          },
          { status: 201 },
        );
      }),
      http.patch('/api/v1/accounts/:account/otel-keys/key-2/exclusions', async ({ request }) => {
        patchBody = (await request.json()) as { excluded_emails: string[] };
        return HttpResponse.json({ excluded_emails: patchBody.excluded_emails });
      }),
    );

    const user = userEvent.setup();
    renderWithProviders(<IngestKeysPanel account="myorg" />);

    // Create dialog is name-only — no exclusions field before the key exists.
    await user.click(await screen.findByRole('button', { name: /add a source/i }));
    const createDialog = await screen.findByRole('dialog');
    expect(within(createDialog).queryByPlaceholderText('person@company.com')).toBeNull();
    await user.type(within(createDialog).getByLabelText('Name'), 'Contractors');
    await user.click(within(createDialog).getByRole('button', { name: /create source/i }));

    await waitFor(() => expect(postBody).toEqual({ name: 'Contractors' }));

    // Reveal dialog appears; the key is masked until revealed.
    const revealDialog = await screen.findByRole('dialog');
    expect(within(revealDialog).getByText('Save your ingestion key')).toBeInTheDocument();
    await user.click(within(revealDialog).getByRole('button', { name: 'Reveal key' }));
    expect(within(revealDialog).getByText('astotel_secretplaintext')).toBeInTheDocument();

    // ...and it offers exclusions.
    await user.type(within(revealDialog).getByPlaceholderText('person@company.com'), 'excluded@x.com');
    await user.click(within(revealDialog).getByRole('button', { name: 'Add' }));

    await waitFor(() => expect(patchBody).toEqual({ excluded_emails: ['excluded@x.com'] }));
  });

  it('renames a source from the menu without revealing the key', async () => {
    listReturns([key]);
    let patchBody: { name: string } | null = null;
    server.use(
      http.patch('/api/v1/accounts/:account/otel-keys/:id/name', async ({ request }) => {
        patchBody = (await request.json()) as { name: string };
        return HttpResponse.json({ name: patchBody.name });
      }),
    );

    const user = userEvent.setup();
    renderWithProviders(<IngestKeysPanel account="myorg" />);

    const row = (await screen.findByText('Engineering laptops')).closest('tr')!;
    await user.click(within(row).getByRole('button'));
    await user.click(await screen.findByText('Rename'));

    // Current name is prefilled; the secret key is never shown here.
    const dialog = await screen.findByRole('dialog');
    const input = within(dialog).getByLabelText('Name');
    expect(input).toHaveValue('Engineering laptops');
    expect(within(dialog).queryByText(/astotel_/)).toBeNull();

    // Save is inert until the name actually changes.
    expect(within(dialog).getByRole('button', { name: /^save$/i })).toBeDisabled();

    await user.clear(input);
    await user.type(input, 'Contractor laptops');
    await user.click(within(dialog).getByRole('button', { name: /^save$/i }));

    await waitFor(() => expect(patchBody).toEqual({ name: 'Contractor laptops' }));
  });

  it('opens the create dialog when hotlinked with ?new=1', async () => {
    listReturns([]);
    renderWithProviders(<IngestKeysPanel account="myorg" />, { initialEntries: ['/settings/api-keys?new=1'] });

    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('New data source')).toBeInTheDocument();
    expect(within(dialog).getByLabelText('Name')).toBeInTheDocument();
  });
});
