import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, cleanup, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OverwriteSecretDialog } from './OverwriteSecretDialog'

afterEach(cleanup)

describe('OverwriteSecretDialog', () => {
  it('does not show a reveal toggle', async () => {
    const user = userEvent.setup()

    render(
      <OverwriteSecretDialog
        secretName="API_KEY"
        open
        onClose={() => {}}
        onConfirm={vi.fn()}
      />,
    )

    expect(screen.queryByRole('button', { name: /reveal value/i })).not.toBeInTheDocument()

    await user.type(screen.getByLabelText('Value'), 'new-secret')
    expect(screen.queryByRole('button', { name: /reveal value/i })).not.toBeInTheDocument()
  })

  it('allows saving when only the description changes', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()

    render(
      <OverwriteSecretDialog
        secretName="API_KEY"
        description="Primary key"
        open
        onClose={() => {}}
        onConfirm={onConfirm}
      />,
    )

    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()

    await user.clear(screen.getByLabelText(/description/i))
    await user.type(screen.getByLabelText(/description/i), 'Updated description')

    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()

    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(onConfirm).toHaveBeenCalledWith({ description: 'Updated description' })
  })
})
