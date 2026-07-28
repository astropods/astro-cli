import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { Archive, ArchiveRestore, Bell, Check, Circle, Loader2 } from 'lucide-react'
import {
  NovuProvider,
  useCounts,
  useNotifications,
  type Notification,
} from '@novu/react'
import { Popover as PopoverPrimitive } from 'radix-ui'
import { cn } from '@/lib/utils'
import { useAuth } from '@/lib/auth'
import { formatRelativeTime } from '@/lib/deployment-utils'
import { useNotificationInboxConfig } from '@/api/queries'

type Redirect = { url?: string; target?: string } | undefined

/**
 * In-app notification feed. Built fully headless on Novu's hooks (NovuProvider +
 * useNotifications/useCounts) with our own DOM — no Novu-rendered components — so
 * it's styled with our tokens and has no preferences view (preferences live in
 * account settings). Self-gates on mount (client-only) and server config.
 */
export function NotificationInbox() {
  const { isAuthenticated } = useAuth()
  const [mounted, setMounted] = useState(false)

  useEffect(() => setMounted(true), [])

  const { data } = useNotificationInboxConfig(isAuthenticated && mounted)

  if (!mounted || !data?.enabled || !data.application_identifier || !data.subscriber_id) {
    return null
  }

  return (
    <NovuProvider
      applicationIdentifier={data.application_identifier}
      subscriberId={data.subscriber_id}
      subscriberHash={data.subscriber_hash}
      backendUrl={data.backend_url}
      socketUrl={data.socket_url}
    >
      <InboxBell />
    </NovuProvider>
  )
}

function InboxBell() {
  const { counts } = useCounts({ filters: [{ read: false, archived: false }] })
  const unread = counts?.[0]?.count ?? 0

  return (
    <PopoverPrimitive.Root>
      <PopoverPrimitive.Trigger asChild>
        <button
          type="button"
          aria-label={unread > 0 ? `Notifications (${unread} unread)` : 'Notifications'}
          className="relative rounded p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <Bell className="size-5" />
          {unread > 0 && (
            <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium leading-none text-primary-foreground">
              {unread > 9 ? '9+' : unread}
            </span>
          )}
        </button>
      </PopoverPrimitive.Trigger>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          align="end"
          sideOffset={8}
          collisionPadding={16}
          className={cn(
            'z-50 w-96 overflow-hidden rounded-md border border-border bg-popover text-foreground shadow-lg',
            'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
          )}
        >
          <InboxList />
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  )
}

type Tab = 'inbox' | 'archived'

function InboxList() {
  const [tab, setTab] = useState<Tab>('inbox')
  const { notifications, isLoading, hasMore, fetchMore, readAll } = useNotifications({
    archived: tab === 'archived',
    limit: 20,
  })
  const [loadingMore, setLoadingMore] = useState(false)

  const items = notifications ?? []
  const hasUnread = items.some((n) => !n.isRead)

  const onLoadMore = async () => {
    setLoadingMore(true)
    try {
      await fetchMore()
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <div className="flex max-h-[32rem] flex-col">
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <div className="flex items-center gap-1">
          <TabButton active={tab === 'inbox'} onClick={() => setTab('inbox')}>
            Inbox
          </TabButton>
          <TabButton active={tab === 'archived'} onClick={() => setTab('archived')}>
            Archived
          </TabButton>
        </div>
        {tab === 'inbox' && hasUnread && (
          <button
            type="button"
            onClick={() => readAll()}
            className="flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
          >
            <Check className="size-3.5" />
            Mark all read
          </button>
        )}
      </div>

      <div className="flex-1 overflow-y-auto">
        {isLoading ? (
          <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            Loading…
          </div>
        ) : items.length === 0 ? (
          <div className="px-4 py-10 text-center">
            <Bell className="mx-auto mb-2 size-6 text-muted-foreground" />
            <p className="text-sm font-medium text-foreground">
              {tab === 'archived' ? 'Nothing archived' : "You're all caught up"}
            </p>
            <p className="text-xs text-muted-foreground">
              {tab === 'archived'
                ? 'Archived notifications will appear here.'
                : 'New notifications will show up here.'}
            </p>
          </div>
        ) : (
          <ul>
            {items.map((n) => (
              <NotificationRow key={n.id} notification={n} tab={tab} />
            ))}
            {hasMore && (
              <li className="p-2">
                <button
                  type="button"
                  onClick={onLoadMore}
                  disabled={loadingMore}
                  className="flex w-full items-center justify-center gap-2 rounded-md py-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  {loadingMore && <Loader2 className="size-3.5 animate-spin" />}
                  Load more
                </button>
              </li>
            )}
          </ul>
        )}
      </div>
    </div>
  )
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'rounded px-2 py-1 text-heading-4 transition-colors',
        active ? 'bg-muted text-foreground' : 'text-muted-foreground hover:text-foreground',
      )}
    >
      {children}
    </button>
  )
}

function NotificationRow({ notification, tab }: { notification: Notification; tab: Tab }) {
  const navigate = useNavigate()

  const follow = (redirect: Redirect) => {
    const url = redirect?.url
    if (!url) return
    if (/^https?:\/\//.test(url)) {
      window.open(url, redirect?.target ?? '_blank', 'noopener,noreferrer')
    } else {
      navigate(url)
    }
  }

  const onRowClick = () => {
    if (!notification.isRead) notification.read()
    follow(notification.redirect)
  }

  return (
    <li
      className={cn(
        'group relative border-b border-border transition-colors last:border-b-0 hover:bg-muted/60',
        !notification.isRead && 'bg-muted/30',
      )}
    >
      <button type="button" onClick={onRowClick} className="flex w-full gap-3 px-4 py-3 text-left">
        {notification.avatar ? (
          <img src={notification.avatar} alt="" className="mt-0.5 size-7 shrink-0 rounded-full object-cover" />
        ) : (
          <span
            className={cn('mt-1.5 size-2 shrink-0 rounded-full', notification.isRead ? 'bg-transparent' : 'bg-primary')}
            aria-hidden
          />
        )}
        <span className="min-w-0 flex-1">
          {notification.subject && (
            <span className="block truncate text-body-sm font-medium text-foreground">{notification.subject}</span>
          )}
          <span className="block text-xs text-muted-foreground">{notification.body}</span>
          <span className="mt-1 block text-[11px] text-faint-foreground">
            {formatRelativeTime(notification.createdAt)}
          </span>

          {(notification.primaryAction || notification.secondaryAction) && (
            <span className="mt-2 flex gap-2">
              {notification.primaryAction && (
                <span
                  role="button"
                  tabIndex={0}
                  onClick={(e) => {
                    e.stopPropagation()
                    notification.completePrimary()
                    follow(notification.primaryAction?.redirect)
                  }}
                  className="rounded-md bg-primary px-2.5 py-1 text-xs font-medium text-primary-foreground hover:opacity-90"
                >
                  {notification.primaryAction.label}
                </span>
              )}
              {notification.secondaryAction && (
                <span
                  role="button"
                  tabIndex={0}
                  onClick={(e) => {
                    e.stopPropagation()
                    notification.completeSecondary()
                    follow(notification.secondaryAction?.redirect)
                  }}
                  className="rounded-md border border-border px-2.5 py-1 text-xs font-medium text-foreground hover:bg-muted"
                >
                  {notification.secondaryAction.label}
                </span>
              )}
            </span>
          )}
        </span>
      </button>

      {/* Hover actions: read/unread toggle + archive/unarchive */}
      <div className="absolute right-2 top-2 flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
        {notification.isRead ? (
          <RowAction label="Mark as unread" onClick={() => notification.unread()}>
            <Circle className="size-3.5" />
          </RowAction>
        ) : (
          <RowAction label="Mark as read" onClick={() => notification.read()}>
            <Check className="size-3.5" />
          </RowAction>
        )}
        {tab === 'archived' ? (
          <RowAction label="Unarchive" onClick={() => notification.unarchive()}>
            <ArchiveRestore className="size-3.5" />
          </RowAction>
        ) : (
          <RowAction label="Archive" onClick={() => notification.archive()}>
            <Archive className="size-3.5" />
          </RowAction>
        )}
      </div>
    </li>
  )
}

function RowAction({
  label,
  onClick,
  children,
}: {
  label: string
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={(e) => {
        e.stopPropagation()
        onClick()
      }}
      className="rounded bg-popover p-1 text-muted-foreground shadow-sm ring-1 ring-border transition-colors hover:text-foreground"
    >
      {children}
    </button>
  )
}
