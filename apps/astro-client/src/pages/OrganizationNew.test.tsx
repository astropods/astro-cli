import { screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, afterEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { renderRoute } from '@/test/test-utils';
import { ORG_DISPLAY_NAME_MAX_LENGTH } from '@/lib/constants';
import { server } from '@/test/msw/server';
import OrganizationNew from './OrganizationNew';

afterEach(cleanup);

function renderPage() {
  return renderRoute(
    [{ path: '/organization/new', Component: OrganizationNew }],
    { initialEntries: ['/organization/new'] },
  );
}

describe('OrganizationNew', () => {
  it('surfaces an inline error when the organization name is too long', () => {
    renderPage();

    fireEvent.change(screen.getByPlaceholderText('My Organization'), {
      target: { value: 'a'.repeat(ORG_DISPLAY_NAME_MAX_LENGTH + 1) },
    });

    expect(
      screen.getByText(
        `Organization names cannot exceed ${ORG_DISPLAY_NAME_MAX_LENGTH} characters.`,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /create organization/i }),
    ).toBeEnabled();
  });

  it('surfaces feedback when the organization username is unavailable on submit', async () => {
    const user = userEvent.setup();

    server.use(
      http.get('/api/v1/accounts/check/taken-org', () =>
        HttpResponse.json({ available: false, reason: 'Already taken' }),
      ),
    );

    renderPage();

    fireEvent.change(screen.getByPlaceholderText('My Organization'), {
      target: { value: 'Valid Org' },
    });
    fireEvent.change(screen.getByPlaceholderText('my-org'), {
      target: { value: 'taken-org' },
    });

    await waitFor(() => {
      expect(screen.getByText('Already taken')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /create organization/i }));

    await waitFor(() => {
      expect(screen.getAllByText('Already taken')).toHaveLength(2);
    });
  });

  it('keeps submit enabled while username availability is checking and shows feedback on click', async () => {
    const user = userEvent.setup();

    renderPage();

    fireEvent.change(screen.getByPlaceholderText('My Organization'), {
      target: { value: 'Valid Org' },
    });
    fireEvent.change(screen.getByPlaceholderText('my-org'), {
      target: { value: 'checking-org' },
    });

    const createButton = screen.getByRole('button', { name: /create organization/i });
    expect(createButton).toBeEnabled();

    await user.click(createButton);

    expect(
      screen.getByText('Checking username availability, try again in a moment'),
    ).toBeInTheDocument();
  });
});
