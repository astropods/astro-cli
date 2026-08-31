import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Notification } from '@novu/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it, vi } from 'vitest'

import { NotificationRow } from './NotificationInbox'
import { TooltipProvider } from './ui/tooltip'

interface NotificationOptions {
  row?: string
  primary?: string
  secondary?: string
  isRead?: boolean
}

function createNotification(options: NotificationOptions = {}) {
  const read = vi.fn()
  const archive = vi.fn()
  const completePrimary = vi.fn()
  const completeSecondary = vi.fn()

  const notification = {
    id: 'notification-1',
    subject: 'Agent deployment needs approval',
    body: 'Customer support agent is ready to deploy to production.',
    createdAt: new Date().toISOString(),
    isRead: options.isRead ?? false,
    redirect: options.row ? { url: options.row } : undefined,
    primaryAction: {
      label: 'Approve',
      redirect: options.primary ? { url: options.primary } : undefined,
    },
    secondaryAction: {
      label: 'View deployment',
      redirect: options.secondary ? { url: options.secondary } : undefined,
    },
    workflow: { identifier: 'build.failed' },
    read,
    archive,
    unarchive: vi.fn(),
    completePrimary,
    completeSecondary,
  } as unknown as Notification

  return { notification, read, archive, completePrimary, completeSecondary }
}

function renderRow(notification: Notification, onClose?: () => void) {
  return render(
    <MemoryRouter>
      <TooltipProvider>
        <ul>
          <NotificationRow notification={notification} tab="inbox" onClose={onClose} />
        </ul>
      </TooltipProvider>
    </MemoryRouter>,
  )
}

describe('NotificationRow', () => {
  it('exposes separate row and action controls in visual focus order', async () => {
    const user = userEvent.setup()
    const { notification } = createNotification()
    const { container } = renderRow(notification)
    const controls = screen.getAllByRole('button')

    expect(controls).toHaveLength(4)
    expect(controls[0].tagName).toBe('BUTTON')
    expect(controls[0]).toHaveAccessibleName(/Agent deployment needs approval/)
    expect(controls[1]).toHaveAccessibleName('Approve')
    expect(controls[2]).toHaveAccessibleName('View deployment')
    expect(controls[3]).toHaveAccessibleName('Archive')
    expect(
      container.querySelector(
        'button button, button [role="button"], [role="button"] button, [role="button"] [role="button"]',
      ),
    ).not.toBeInTheDocument()

    for (const control of controls) {
      await user.tab()
      expect(control).toHaveFocus()
    }
  })

  it('adds unread state to the row accessible name only while unread', () => {
    const { notification: unreadNotification } = createNotification()
    const { unmount } = renderRow(unreadNotification)
    const unreadRow = screen.getByRole('button', {
      name: /Agent deployment needs approval/,
    })

    expect(unreadRow).toHaveAccessibleName(/Agent deployment needs approval/)
    expect(unreadRow).toHaveAccessibleName(
      /Customer support agent is ready to deploy to production/,
    )
    expect(unreadRow).toHaveAccessibleName(/unread/i)

    unmount()
    const { notification: readNotification } = createNotification({ isRead: true })
    renderRow(readNotification)
    const readRow = screen.getByRole('button', { name: /Agent deployment needs approval/ })

    expect(readRow).toHaveAccessibleName(/Agent deployment needs approval/)
    expect(readRow).toHaveAccessibleName(
      /Customer support agent is ready to deploy to production/,
    )
    expect(readRow).not.toHaveAccessibleName(/unread/i)
  })

  it('activates the row with Enter and Space', async () => {
    const user = userEvent.setup()
    const { notification, read } = createNotification()
    renderRow(notification)
    const row = screen.getByRole('button', { name: /Agent deployment needs approval/ })

    row.focus()
    await user.keyboard('{Enter}')
    await user.keyboard(' ')

    expect(read).toHaveBeenCalledTimes(2)
  })

  it('activates CTAs without activating the row', async () => {
    const user = userEvent.setup()
    const { notification, read, completePrimary, completeSecondary } = createNotification()
    renderRow(notification)

    screen.getByRole('button', { name: 'Approve' }).focus()
    await user.keyboard('{Enter}')
    screen.getByRole('button', { name: 'View deployment' }).focus()
    await user.keyboard(' ')

    expect(completePrimary).toHaveBeenCalledOnce()
    expect(completeSecondary).toHaveBeenCalledOnce()
    expect(read).not.toHaveBeenCalled()
  })

  it.each([
    ['row', 'Agent deployment needs approval'],
    ['primary action', 'Approve'],
    ['secondary action', 'View deployment'],
  ])('closes after internal navigation from the %s', async (_, controlName) => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    const { notification } = createNotification({
      row: '/agents/agent-1',
      primary: '/deployments/deployment-1',
      secondary: '/deployments/deployment-1/logs',
    })
    renderRow(notification, onClose)

    await user.click(screen.getByRole('button', { name: new RegExp(controlName, 'i') }))

    expect(onClose).toHaveBeenCalledOnce()
  })

  it('keeps the inbox open when following an external link', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    const open = vi.spyOn(window, 'open').mockImplementation(() => null)
    const { notification } = createNotification({ row: 'https://example.com/notification' })
    renderRow(notification, onClose)

    await user.click(
      screen.getByRole('button', { name: /Agent deployment needs approval/ }),
    )

    expect(open).toHaveBeenCalledWith(
      'https://example.com/notification',
      '_blank',
      'noopener,noreferrer',
    )
    expect(onClose).not.toHaveBeenCalled()
  })
})
