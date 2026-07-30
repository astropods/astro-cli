import { describe, it, expect, afterEach } from 'vitest'
import { useState, type ReactNode } from 'react'
import { fireEvent, render, cleanup, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { AuthContext } from '@/lib/auth-context'
import { mockAuthContext } from '@/test/test-utils'
import { isVariableFilled, VariableField } from './VariableField'
import type { VariableDisplay } from './VariableFields'

afterEach(cleanup)

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

describe('configured inline secrets', () => {
  it('isVariableFilled treats configured secrets as filled', () => {
    const meta: VariableDisplay = { secret: true, configured: true }
    expect(isVariableFilled(meta, '')).toBe(true)
    expect(isVariableFilled(meta, undefined)).toBe(true)
  })

  it('renders masked placeholder when configured without value', () => {
    const meta: VariableDisplay = { secret: true, configured: true, label: 'API Key' }
    render(
      <Providers>
        <VariableField
          fieldKey="API_KEY"
          meta={meta}
          value=""
          onChange={() => {}}
        />
      </Providers>,
    )
    expect(screen.getByRole('button', { name: /API Key.*Configured/i })).toBeInTheDocument()
    expect(screen.getByText('Configured')).toBeInTheDocument()
  })

  it('restores mask after click-away without typing', async () => {
    const user = userEvent.setup()
    const meta: VariableDisplay = { secret: true, configured: true, label: 'API Key' }
    render(
      <Providers>
        <VariableField
          fieldKey="API_KEY"
          meta={meta}
          value=""
          onChange={() => {}}
        />
      </Providers>,
    )

    await user.click(screen.getByRole('button', { name: /API Key.*Configured/i }))
    const input = document.getElementById('API_KEY') as HTMLInputElement
    expect(input).toHaveValue('')

    fireEvent.blur(input)
    expect(screen.getByRole('button', { name: /API Key.*Configured/i })).toBeInTheDocument()
  })

  it('shows reveal toggle only when user has typed a replacement value', async () => {
    const user = userEvent.setup()
    const meta: VariableDisplay = { secret: true, configured: true, label: 'API Key' }

    function Harness() {
      const [val, setVal] = useState('')
      return (
        <VariableField fieldKey="API_KEY" meta={meta} value={val} onChange={setVal} />
      )
    }

    render(
      <Providers>
        <Harness />
      </Providers>,
    )

    expect(screen.queryByRole('button', { name: /reveal value/i })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /API Key.*Configured/i }))
    await user.type(document.getElementById('API_KEY')!, 'new-secret')
    expect(screen.getByRole('button', { name: /reveal value/i })).toBeInTheDocument()
  })
})
