import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup, within } from '@testing-library/react';
import { ConnectorRow, ConnectorRowList, ConnectorRowItem } from './ConnectorRow';

afterEach(cleanup);

describe('ConnectorRow', () => {
  it('renders icon, name, description, and action when not loading', () => {
    render(
      <ConnectorRow
        icon={<svg data-testid="row-icon" />}
        name="GitHub"
        description="Build and deploy agents directly from your repositories."
        action={<button type="button">Connect</button>}
      />,
    );

    expect(screen.getByTestId('row-icon')).toBeInTheDocument();
    expect(screen.getByText('GitHub')).toBeInTheDocument();
    expect(
      screen.getByText('Build and deploy agents directly from your repositories.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect' })).toBeInTheDocument();
  });

  it('hides name/description/action and shows a skeleton while loading', () => {
    render(
      <ConnectorRow
        icon={<svg data-testid="row-icon" />}
        name="GitHub"
        description="should not render"
        action={<button type="button">Connect</button>}
        isLoading
      />,
    );

    expect(screen.queryByText('GitHub')).not.toBeInTheDocument();
    expect(screen.queryByText('should not render')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Connect' })).not.toBeInTheDocument();
  });

  it('renders children (nested list) below the row', () => {
    render(
      <ConnectorRow
        icon={<svg />}
        name="GitHub"
        description="desc"
      >
        <ConnectorRowList>
          <ConnectorRowItem>org-one</ConnectorRowItem>
          <ConnectorRowItem>org-two</ConnectorRowItem>
        </ConnectorRowList>
      </ConnectorRow>,
    );

    const list = screen.getByRole('list');
    const items = within(list).getAllByRole('listitem');
    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent('org-one');
    expect(items[1]).toHaveTextContent('org-two');
  });

  it('does not render an action slot wrapper when no action is provided', () => {
    const { container } = render(
      <ConnectorRow icon={<svg />} name="GitHub" description="desc" />,
    );

    expect(container.querySelectorAll('.shrink-0')).toHaveLength(1);
  });
});

describe('ConnectorRowList / ConnectorRowItem', () => {
  it('renders ConnectorRowList as a <ul> containing ConnectorRowItem <li>s', () => {
    render(
      <ConnectorRowList>
        <ConnectorRowItem>one</ConnectorRowItem>
        <ConnectorRowItem>two</ConnectorRowItem>
      </ConnectorRowList>,
    );

    const list = screen.getByRole('list');
    expect(list.tagName).toBe('UL');
    const items = within(list).getAllByRole('listitem');
    expect(items).toHaveLength(2);
    expect(items[0].tagName).toBe('LI');
  });
});
