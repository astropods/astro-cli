import { describe, it, expect, afterEach, vi } from 'vitest'
import { useState, type ReactNode } from 'react'
import { render, cleanup, waitFor, act, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { AuthContext } from '@/lib/auth-context'
import { mockAuthContext } from '@/test/test-utils'
import { TooltipProvider } from '@/components/ui/tooltip'
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

const matchingPlainEntry: AccountVariable = {
  ...matchingEntry,
  secret: false,
}

function Providers({ children }: { children: ReactNode }) {
  return (
    <AuthContext.Provider value={mockAuthContext}>
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <MemoryRouter>
          <TooltipProvider>{children}</TooltipProvider>
        </MemoryRouter>
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

  it('keeps vault auto-fill disabled unless the caller explicitly enables it', async () => {
    const onChange = vi.fn()

    render(
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

    await act(async () => {
      await Promise.resolve()
    })
    expect(onChange).not.toHaveBeenCalled()
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
          vaultAutoFillEnabled={true}
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
          vaultAutoFillEnabled={true}
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
          vaultAutoFillEnabled={true}
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
          vaultAutoFillEnabled={true}
        />
      </Providers>,
    )

    await act(async () => {
      await Promise.resolve()
    })
    expect(onChange).not.toHaveBeenCalled()
  })

  it('consumes the suggestion opportunity when a prefilled value loads', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [value, setValue] = useState('{{secrets.SLACK_BOT_TOKEN}}')
      return (
        <VariableField
          fieldKey="SLACK_BOT_TOKEN"
          meta={meta}
          value={value}
          onChange={setValue}
          vaultEntries={[matchingEntry]}
          vaultEntriesLoaded={true}
          vaultAutoFillEnabled={true}
        />
      )
    }

    render(<Providers><Harness /></Providers>)
    await act(async () => {
      await Promise.resolve()
    })

    expect(screen.queryByText(/^Auto-filled/)).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Clear vault reference' }))

    await act(async () => {
      await Promise.resolve()
    })
    expect(document.getElementById('SLACK_BOT_TOKEN')).toHaveValue('')
    expect(screen.queryByRole('button', { name: 'Clear vault reference' })).not.toBeInTheDocument()
  })

  it('does not replace a configured inline secret with a matching vault entry', async () => {
    const onChange = vi.fn()

    render(
      <Providers>
        <VariableField
          fieldKey="SLACK_BOT_TOKEN"
          meta={{ ...meta, configured: true, label: 'Slack bot token' }}
          value=""
          onChange={onChange}
          vaultEntries={[matchingEntry]}
          vaultEntriesLoaded={true}
          vaultAutoFillEnabled={true}
        />
      </Providers>,
    )

    await act(async () => {
      await Promise.resolve()
    })
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /Slack bot token.*Configured/i })).toBeInTheDocument()
  })

  it('clears auto-fill provenance when the user selects the same entry explicitly', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [value, setValue] = useState('')
      return (
        <VariableField
          fieldKey="SLACK_BOT_TOKEN"
          meta={meta}
          value={value}
          onChange={setValue}
          vaultEntries={[matchingEntry]}
          vaultEntriesLoaded={true}
          vaultAutoFillEnabled={true}
        />
      )
    }

    render(<Providers><Harness /></Providers>)
    await waitFor(() => {
      expect(screen.getByText('Auto-filled')).toBeInTheDocument()
    })

    await user.click(screen.getByTitle('Insert vault reference'))
    const matchingLabels = screen.getAllByText('SLACK_BOT_TOKEN')
    const pickerEntry = matchingLabels
      .map((node) => node.closest('button'))
      .find((button) => button && button.getAttribute('aria-label') !== 'Clear vault reference')
    expect(pickerEntry).toBeTruthy()
    await user.click(pickerEntry!)

    expect(screen.queryByText(/^Auto-filled/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clear vault reference' })).toBeInTheDocument()
  })

  it('does not suggest or auto-fill an incompatible plain variable', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()

    render(
      <Providers>
        <VariableField
          fieldKey="SLACK_BOT_TOKEN"
          meta={meta}
          value=""
          onChange={onChange}
          vaultEntries={[matchingPlainEntry]}
          vaultEntriesLoaded={true}
          vaultAutoFillEnabled={true}
        />
      </Providers>,
    )

    await act(async () => {
      await Promise.resolve()
    })
    expect(onChange).not.toHaveBeenCalled()

    await user.click(screen.getByTitle('Insert vault reference'))
    expect(screen.getByText('No secret variables yet')).toBeInTheDocument()
    expect(screen.queryByText('SLACK_BOT_TOKEN')).not.toBeInTheDocument()
  })

  it('auto-fills a plain field and hides incompatible secret entries', async () => {
    const user = userEvent.setup()
    const plainEntry = {
      ...matchingPlainEntry,
      name: 'APP_ENV',
    }
    const incompatibleSecretEntry = {
      ...matchingEntry,
      name: 'SECRET_APP_ENV',
    }

    function Harness() {
      const [value, setValue] = useState('')
      return (
        <VariableField
          fieldKey="APP_ENV"
          meta={{ ...meta, secret: false }}
          value={value}
          onChange={setValue}
          vaultEntries={[plainEntry, incompatibleSecretEntry]}
          vaultEntriesLoaded={true}
          vaultAutoFillEnabled={true}
        />
      )
    }

    render(<Providers><Harness /></Providers>)
    await waitFor(() => {
      expect(screen.getByText('Auto-filled')).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Clear vault reference' })).toHaveTextContent('APP_ENV')

    await user.click(screen.getByTitle('Insert vault reference'))
    expect(screen.getByPlaceholderText('Find...')).toBeInTheDocument()
    expect(screen.queryByText('SECRET_APP_ENV')).not.toBeInTheDocument()
  })

  it('does not auto-fill again when a parent reset restores the empty initial value', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [value, setValue] = useState('')
      return (
        <>
          <VariableField
            fieldKey="SLACK_BOT_TOKEN"
            meta={meta}
            value={value}
            onChange={setValue}
            vaultEntries={[matchingEntry]}
            vaultEntriesLoaded={true}
            vaultAutoFillEnabled={true}
          />
          <button type="button" onClick={() => setValue('')}>Reset form</button>
        </>
      )
    }

    render(<Providers><Harness /></Providers>)
    await waitFor(() => {
      expect(screen.getByText('Auto-filled')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: 'Reset form' }))
    await act(async () => {
      await Promise.resolve()
    })

    expect(document.getElementById('SLACK_BOT_TOKEN')).toHaveValue('')
    expect(screen.queryByText(/^Auto-filled/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Clear vault reference' })).not.toBeInTheDocument()
  })

  it('does not auto-fill again after a cleared field unmounts and remounts', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [value, setValue] = useState('')
      const [mounted, setMounted] = useState(true)
      const [autoFillState, setAutoFillState] = useState<Record<string, string | null>>({})

      return (
        <>
          {mounted && (
            <VariableField
              fieldKey="SLACK_BOT_TOKEN"
              meta={meta}
              value={value}
              onChange={setValue}
              vaultEntries={[matchingEntry]}
              vaultEntriesLoaded={true}
              vaultAutoFillEnabled={true}
              vaultAutoFilledToken={autoFillState.SLACK_BOT_TOKEN}
              onVaultAutoFilledTokenChange={(token) =>
                setAutoFillState((current) => ({
                  ...current,
                  SLACK_BOT_TOKEN: token,
                }))
              }
            />
          )}
          <button type="button" onClick={() => setMounted((current) => !current)}>
            {mounted ? 'Unmount field' : 'Remount field'}
          </button>
        </>
      )
    }

    render(<Providers><Harness /></Providers>)
    await waitFor(() => {
      expect(screen.getByText('Auto-filled')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: 'Clear vault reference' }))
    await user.click(screen.getByRole('button', { name: 'Unmount field' }))
    await user.click(screen.getByRole('button', { name: 'Remount field' }))
    await act(async () => {
      await Promise.resolve()
    })

    expect(document.getElementById('SLACK_BOT_TOKEN')).toHaveValue('')
    expect(screen.queryByText(/^Auto-filled/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Clear vault reference' })).not.toBeInTheDocument()
  })

  it('does not offer vault references for display-only object sub-fields', async () => {
    const onChange = vi.fn()

    render(
      <Providers>
        <VariableField
          fieldKey="SLACK_CONFIG.allowed_channel_ids"
          meta={{ secret: false, datatype: 'string', vaultReferenceAllowed: false }}
          value=""
          onChange={onChange}
          vaultEntries={[{
            ...matchingPlainEntry,
            name: 'SLACK_CONFIG.allowed_channel_ids',
          }]}
          vaultEntriesLoaded={true}
          vaultAutoFillEnabled={true}
        />
      </Providers>,
    )

    await act(async () => {
      await Promise.resolve()
    })

    expect(onChange).not.toHaveBeenCalled()
    expect(screen.queryByTitle('Insert vault reference')).not.toBeInTheDocument()
    expect(document.getElementById('SLACK_CONFIG.allowed_channel_ids')).toHaveValue('')
  })
})
