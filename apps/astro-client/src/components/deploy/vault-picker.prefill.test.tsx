import { describe, it, expect, afterEach, vi } from 'vitest'
import { type ReactNode } from 'react'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { AuthContext, type AuthContextType } from '@/lib/auth-context'
import { mockAuthContext } from '@/test/test-utils'
import type { Account, AccountVariable } from '@/lib/api'
import { VaultPicker } from './VaultPicker'

afterEach(cleanup)

const entries: AccountVariable[] = [
  { name: 'API_KEY', secret: true, description: '', created_at: '', updated_at: '' },
]

const personal: Account = { id: 'acct-p', name: 'testuser', type: 'personal' }

function Providers({ children }: { children: ReactNode }) {
  const auth: AuthContextType = {
    ...mockAuthContext,
    organizationId: null,
    accounts: [personal],
    switchOrg: vi.fn(),
  }
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return (
    <AuthContext.Provider value={auth}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    </AuthContext.Provider>
  )
}

describe('VaultPicker new-variable prefill', () => {
  it('seeds the dialog Key and Value from the launching field', async () => {
    const user = userEvent.setup()
    render(
      <Providers>
        <VaultPicker
          onSelect={() => {}}
          accountName="testuser"
          entries={entries}
          newVarName="OPENAI_API_KEY"
          newVarValue="sk-typed-123"
          open
        />
      </Providers>,
    )

    await user.click(screen.getByRole('button', { name: /^new$/i }))

    expect(await screen.findByLabelText('Key')).toHaveValue('OPENAI_API_KEY')
    expect(screen.getByLabelText('Value')).toHaveValue('sk-typed-123')
  })

  it('prefills from the empty-vault "New variable" affordance too', async () => {
    const user = userEvent.setup()
    render(
      <Providers>
        <VaultPicker
          onSelect={() => {}}
          accountName="testuser"
          entries={[]}
          newVarName="STRIPE_SECRET_KEY"
          newVarValue="sk_live_456"
          open
        />
      </Providers>,
    )

    await user.click(screen.getByRole('button', { name: /new variable/i }))

    expect(await screen.findByLabelText('Key')).toHaveValue('STRIPE_SECRET_KEY')
    expect(screen.getByLabelText('Value')).toHaveValue('sk_live_456')
  })

  it('normalizes whitespace in the seeded key', async () => {
    const user = userEvent.setup()
    render(
      <Providers>
        <VaultPicker
          onSelect={() => {}}
          accountName="testuser"
          entries={entries}
          newVarName="my api key"
          newVarValue=""
          open
        />
      </Providers>,
    )

    await user.click(screen.getByRole('button', { name: /^new$/i }))

    expect(await screen.findByLabelText('Key')).toHaveValue('my_api_key')
  })
})
