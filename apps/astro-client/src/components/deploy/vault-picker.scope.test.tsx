import { describe, it, expect, afterEach, vi } from 'vitest'
import { type ReactNode } from 'react'
import { render, screen, cleanup, waitFor, act } from '@testing-library/react'
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

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function Providers({ auth, queryClient, children }: { auth: AuthContextType; queryClient: QueryClient; children: ReactNode }) {
  return (
    <AuthContext.Provider value={auth}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    </AuthContext.Provider>
  )
}

describe('VaultPicker scope switching', () => {
  it('switches org once across multiple pickers and reveals + New after the session updates', async () => {
    const acmeOrg: Account = {
      id: 'acct-acme',
      name: 'acme',
      type: 'organization',
      role: 'admin',
      organization_id: 'wos-acme-1',
    }
    let resolveSwitch: () => void = () => {}
    const switchOrg = vi.fn().mockImplementation(
      () => new Promise<void>((resolve) => { resolveSwitch = resolve }),
    )
    const authBefore: AuthContextType = {
      ...mockAuthContext,
      organizationId: 'wos-other',
      accounts: [acmeOrg],
      switchOrg,
    }

    const queryClient = makeQueryClient()
    const tree = (auth: AuthContextType) => (
      <Providers auth={auth} queryClient={queryClient}>
        <VaultPicker onSelect={() => {}} accountName="acme" entries={entries} open />
        <VaultPicker onSelect={() => {}} accountName="acme" entries={entries} open />
      </Providers>
    )

    const { rerender } = render(tree(authBefore))

    expect(switchOrg).toHaveBeenCalledTimes(1)
    expect(switchOrg).toHaveBeenCalledWith('wos-acme-1')
    expect(screen.queryAllByRole('button', { name: /^new$/i })).toHaveLength(0)

    await act(async () => {
      resolveSwitch()
      await Promise.resolve()
    })

    rerender(tree({ ...authBefore, organizationId: 'wos-acme-1' }))

    await waitFor(() => {
      expect(screen.getAllByRole('button', { name: /^new$/i }).length).toBeGreaterThan(0)
    })
    expect(switchOrg).toHaveBeenCalledTimes(1)
  })

  it('shows + New immediately when session is already scoped to the target org', () => {
    const matchingOrg: Account = {
      id: 'acct-acme-2',
      name: 'acme-2',
      type: 'organization',
      role: 'admin',
      organization_id: 'wos-acme-2',
    }
    const switchOrg = vi.fn()
    render(
      <Providers
        auth={{
          ...mockAuthContext,
          organizationId: 'wos-acme-2',
          accounts: [matchingOrg],
          switchOrg,
        }}
        queryClient={makeQueryClient()}
      >
        <VaultPicker onSelect={() => {}} accountName="acme-2" entries={entries} open />
      </Providers>,
    )

    expect(switchOrg).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /^new$/i })).toBeInTheDocument()
  })

  it('shows + New immediately for a personal account without switching org', () => {
    const personal: Account = { id: 'acct-personal', name: 'testuser', type: 'personal' }
    const switchOrg = vi.fn()
    render(
      <Providers
        auth={{
          ...mockAuthContext,
          organizationId: 'wos-something',
          accounts: [personal],
          switchOrg,
        }}
        queryClient={makeQueryClient()}
      >
        <VaultPicker onSelect={() => {}} accountName="testuser" entries={entries} open />
      </Providers>,
    )

    expect(switchOrg).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /^new$/i })).toBeInTheDocument()
  })

  it('scopes the session to a personal account org too', () => {
    const personal: Account = {
      id: 'acct-me',
      name: 'saswat',
      type: 'personal',
      organization_id: 'wos-personal',
    }
    const switchOrg = vi.fn().mockResolvedValue(undefined)
    render(
      <Providers
        auth={{
          ...mockAuthContext,
          organizationId: 'wos-other',
          accounts: [personal],
          switchOrg,
        }}
        queryClient={makeQueryClient()}
      >
        <VaultPicker onSelect={() => {}} accountName="saswat" entries={entries} open />
      </Providers>,
    )

    expect(switchOrg).toHaveBeenCalledWith('wos-personal')
  })
})
