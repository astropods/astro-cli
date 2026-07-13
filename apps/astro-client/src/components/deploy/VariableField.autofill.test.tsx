import { describe, it, expect, afterEach, vi } from 'vitest'
import { type ReactNode } from 'react'
import { render, cleanup, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { AuthContext } from '@/lib/auth-context'
import { mockAuthContext } from '@/test/test-utils'
import type { AccountVariable } from '@/lib/api'
import { VariableField } from './VariableField'
import type { VariableDisplay } from './VariableFields'

afterEach(cleanup)

const meta: VariableDisplay = { secret: true, datatype: 'string' }

const matchingEntry: AccountVariable = {
  name: 'SLACK_BOT_TOKEN',
  secret: true,
  description: '',
  created_at: '',
  updated_at: '',
}

function Providers({ children }: { children: ReactNode }) {
  return (
    <AuthContext.Provider value={mockAuthContext}>
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    </AuthContext.Provider>
  )
}

describe('VariableField vault auto-fill readiness gate', () => {
  it('renders secret inputs with stable field identity for browser autofill', () => {
    render(
      <Providers>
        <VariableField
          fieldKey="ANTHROPIC_API_KEY"
          meta={{ ...meta, label: 'Anthropic API Key' }}
          value=""
          onChange={() => {}}
          vaultEntries={[]}
          vaultEntriesLoaded={true}
        />
      </Providers>,
    )

    const input = document.getElementById('ANTHROPIC_API_KEY') as HTMLInputElement
    expect(input).toHaveAttribute('name', 'ANTHROPIC_API_KEY')
    expect(input).toHaveAttribute('autocomplete', 'new-password')
  })

  it('does not auto-fill while the vault query is still loading, then fills once it resolves', async () => {
    const onChange = vi.fn()

    // First render: vault entries are technically present but the query hasn't
    // reported success yet. Auto-fill must wait — firing now would race the
    // parent form's seeding effect, which replaces the value in the same commit.
    const { rerender } = render(
      <Providers>
        <VariableField
          fieldKey="SLACK_BOT_TOKEN"
          meta={meta}
          value=""
          onChange={onChange}
          vaultEntries={[matchingEntry]}
          vaultEntriesLoaded={false}
        />
      </Providers>,
    )

    // Give any pending effects a tick. onChange must not have been called.
    await act(async () => {
      await Promise.resolve()
    })
    expect(onChange).not.toHaveBeenCalled()

    // Vault query resolves — readiness flips to true.
    rerender(
      <Providers>
        <VariableField
          fieldKey="SLACK_BOT_TOKEN"
          meta={meta}
          value=""
          onChange={onChange}
          vaultEntries={[matchingEntry]}
          vaultEntriesLoaded={true}
        />
      </Providers>,
    )

    await waitFor(() => {
      expect(onChange).toHaveBeenCalledWith('{{secrets.SLACK_BOT_TOKEN}}')
    })
    expect(onChange).toHaveBeenCalledTimes(1)
  })

  it('does not auto-fill once loaded if there is no matching vault entry', async () => {
    const onChange = vi.fn()

    render(
      <Providers>
        <VariableField
          fieldKey="SLACK_BOT_TOKEN"
          meta={meta}
          value=""
          onChange={onChange}
          vaultEntries={[]}
          vaultEntriesLoaded={true}
        />
      </Providers>,
    )

    await act(async () => {
      await Promise.resolve()
    })
    expect(onChange).not.toHaveBeenCalled()
  })

  it('does not auto-fill once loaded if the field already has a value', async () => {
    const onChange = vi.fn()

    render(
      <Providers>
        <VariableField
          fieldKey="SLACK_BOT_TOKEN"
          meta={meta}
          value="user-typed-value"
          onChange={onChange}
          vaultEntries={[matchingEntry]}
          vaultEntriesLoaded={true}
        />
      </Providers>,
    )

    await act(async () => {
      await Promise.resolve()
    })
    expect(onChange).not.toHaveBeenCalled()
  })
})
