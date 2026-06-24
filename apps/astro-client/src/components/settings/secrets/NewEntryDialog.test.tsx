import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NewEntryDialog } from './NewEntryDialog'

afterEach(cleanup)

describe('NewEntryDialog seeding', () => {
  it('seeds the first row key and value on open', async () => {
    render(
      <NewEntryDialog open initialName="FOO" initialValue="bar" onClose={() => {}} onCreate={() => {}} />,
    )
    expect(await screen.findByLabelText('Key')).toHaveValue('FOO')
    expect(screen.getByLabelText('Value')).toHaveValue('bar')
  })

  it('seeds a non-secret row when initialSecret is false', async () => {
    render(
      <NewEntryDialog open initialName="DATABASE_URL" initialValue="postgres://x" initialSecret={false} onClose={() => {}} onCreate={() => {}} />,
    )
    await screen.findByLabelText('Key')
    expect(screen.getByRole('switch')).not.toBeChecked()
    // Non-secret value is shown as plain text, not masked.
    expect(screen.getByLabelText('Value')).toHaveAttribute('type', 'text')
  })

  it('defaults the seeded row to secret when initialSecret is omitted', async () => {
    render(
      <NewEntryDialog open initialName="API_KEY" initialValue="sk-1" onClose={() => {}} onCreate={() => {}} />,
    )
    await screen.findByLabelText('Key')
    expect(screen.getByRole('switch')).toBeChecked()
  })

  it('does not re-seed (wiping edits) when the seed prop changes while open', async () => {
    const user = userEvent.setup()
    const { rerender } = render(
      <NewEntryDialog open initialName="FOO" initialValue="" onClose={() => {}} onCreate={() => {}} />,
    )
    const value = await screen.findByLabelText('Value')
    await user.type(value, 'user-typed')

    // The launching field's value changes behind the open dialog (e.g. auto-fill
    // or post-create query invalidation), pushing a new initialValue prop.
    rerender(
      <NewEntryDialog open initialName="FOO" initialValue="changed" onClose={() => {}} onCreate={() => {}} />,
    )

    expect(screen.getByLabelText('Value')).toHaveValue('user-typed')
    expect(screen.getByLabelText('Key')).toHaveValue('FOO')
  })

  it('does not drop rows added via "Add another" when the seed prop changes while open', async () => {
    const user = userEvent.setup()
    const { rerender } = render(
      <NewEntryDialog open initialName="FOO" initialValue="" onClose={() => {}} onCreate={() => {}} />,
    )
    await screen.findByLabelText('Key')
    await user.click(screen.getByRole('button', { name: /add another/i }))
    expect(screen.getAllByLabelText('Key')).toHaveLength(2)

    rerender(
      <NewEntryDialog open initialName="FOO" initialValue="changed" onClose={() => {}} onCreate={() => {}} />,
    )

    expect(screen.getAllByLabelText('Key')).toHaveLength(2)
  })
})
