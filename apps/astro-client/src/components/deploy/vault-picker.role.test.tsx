import { describe, it, expect, afterEach, vi } from 'vitest'
import { type ReactNode } from 'react'
import { render, screen, cleanup } from '@testing-library/react'
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

function Providers({ auth, children }: { auth: AuthContextType; children: ReactNode }) {
  return (
    <AuthContext.Provider value={auth}>
      <QueryClientProvider client={makeQueryClient()}>
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    </AuthContext.Provider>
  )
}

describe('VaultPicker role gate', () => {
  it('shows + New for a personal account', () => {
    const personal: Account = { id: 'acct-p', name: 'testuser', type: 'personal' }
    render(
      <Providers
        auth={{
          ...mockAuthContext,
          organizationId: null,
          accounts: [personal],
          switchOrg: vi.fn(),
        }}
      >
        <VaultPicker onSelect={() => {}} accountName="testuser" entries={entries} open />
      </Providers>,
    )

    expect(screen.getByRole('button', { name: /^new$/i })).toBeInTheDocument()
  })

  it('shows + New for an org admin', () => {
    const orgAdmin: Account = {
      id: 'acct-admin',
      name: 'acme',
      type: 'organization',
      role: 'admin',
      organization_id: 'wos-acme',
    }
    render(
      <Providers
        auth={{
          ...mockAuthContext,
          organizationId: 'wos-acme',
          accounts: [orgAdmin],
          switchOrg: vi.fn(),
        }}
      >
        <VaultPicker onSelect={() => {}} accountName="acme" entries={entries} open />
      </Providers>,
    )

    expect(screen.getByRole('button', { name: /^new$/i })).toBeInTheDocument()
  })

  it('shows + New for an org owner', () => {
    const orgOwner: Account = {
      id: 'acct-owner',
      name: 'acme',
      type: 'organization',
      role: 'owner',
      organization_id: 'wos-acme',
    }
    render(
      <Providers
        auth={{
          ...mockAuthContext,
          organizationId: 'wos-acme',
          accounts: [orgOwner],
          switchOrg: vi.fn(),
        }}
      >
        <VaultPicker onSelect={() => {}} accountName="acme" entries={entries} open />
      </Providers>,
    )

    expect(screen.getByRole('button', { name: /^new$/i })).toBeInTheDocument()
  })

  it('hides + New for an org member but keeps existing entries selectable', () => {
    const orgMember: Account = {
      id: 'acct-member',
      name: 'acme',
      type: 'organization',
      role: 'member',
      organization_id: 'wos-acme',
    }
    render(
      <Providers
        auth={{
          ...mockAuthContext,
          organizationId: 'wos-acme',
          accounts: [orgMember],
          switchOrg: vi.fn(),
        }}
      >
        <VaultPicker onSelect={() => {}} accountName="acme" entries={entries} open />
      </Providers>,
    )

    expect(screen.queryByRole('button', { name: /^new$/i })).not.toBeInTheDocument()
    expect(screen.getByText('API_KEY')).toBeInTheDocument()
  })

  it('hides empty-state "New variable" button for an org member', () => {
    const orgMember: Account = {
      id: 'acct-member',
      name: 'acme',
      type: 'organization',
      role: 'member',
      organization_id: 'wos-acme',
    }
    render(
      <Providers
        auth={{
          ...mockAuthContext,
          organizationId: 'wos-acme',
          accounts: [orgMember],
          switchOrg: vi.fn(),
        }}
      >
        <VaultPicker onSelect={() => {}} accountName="acme" entries={[]} open />
      </Providers>,
    )

    expect(screen.queryByRole('button', { name: /new variable/i })).not.toBeInTheDocument()
    expect(screen.getByText('No variables yet')).toBeInTheDocument()
  })
})
