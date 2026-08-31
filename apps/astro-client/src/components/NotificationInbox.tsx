import { useEffect, useState, type SyntheticEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import {
  Archive,
  ArchiveRestore,
  Bell,
  CheckCheck,
  Loader2,
  Settings,
  X,
} from 'lucide-react'
import {
  NovuProvider,
  useCounts,
  useNotifications,
  type Notification,
} from '@novu/react'
import { Popover as PopoverPrimitive } from 'radix-ui'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { InlineBadge } from '@/components/InlineBadge'
import { TabButton } from '@/components/TabButton'
import {
  resolveNotificationIcon,
  type NotificationIconTone,
} from '@/components/notification-inbox-icons'
import { cn } from '@/lib/utils'
import { useAuth } from '@/lib/auth'
import { formatRelativeTime } from '@/lib/deployment-utils'
import { useNotificationInboxConfig } from '@/api/queries'
import { useIsMobile } from '@/hooks/use-compact-layout'

type Redirect = { url?: string; target?: string } | undefined

const NOTIFICATION_ICON_TONE: Record<NotificationIconTone, string> = {
  neutral: 'text-muted-foreground',
  primary: 'text-foreground-accent',
  info: 'text-info',
  warning: 'text-warning',
  critical: 'text-destructive',
  success: 'text-success',
}

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
  const isMobile = useIsMobile()
  const [open, setOpen] = useState(false)

  const trigger = (
    <button
      type="button"
      aria-label={unread > 0 ? `Notifications (${unread} unread)` : 'Notifications'}
      className="relative rounded p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      <Bell className="size-5" />
      {unread > 0 && (
        <InlineBadge
          variant="soft"
          className="absolute -right-0.5 -top-0.5 ml-0 h-4 min-w-4 justify-center rounded-full border-0 bg-primary px-1 font-sans text-[10px] font-medium tracking-normal text-primary-foreground"
        >
          {unread > 9 ? '9+' : unread}
        </InlineBadge>
      )}
    </button>
  )

  if (isMobile) {
    return (
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger asChild>{trigger}</SheetTrigger>
        <SheetContent
          side="bottom"
          showCloseButton={false}
          className="h-[min(86dvh,760px)] max-h-[calc(100dvh-0.75rem)] gap-0 overflow-hidden rounded-t-2xl border-border bg-popover p-0 text-foreground shadow-2xl"
        >
          <SheetTitle className="sr-only">Notifications</SheetTitle>
          <TooltipProvider delayDuration={200}>
            <InboxList unread={unread} mobile onClose={() => setOpen(false)} />
          </TooltipProvider>
        </SheetContent>
      </Sheet>
    )
  }

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={setOpen}>
      <PopoverPrimitive.Trigger asChild>{trigger}</PopoverPrimitive.Trigger>
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
          <TooltipProvider delayDuration={200}>
            <InboxList unread={unread} onClose={() => setOpen(false)} />
          </TooltipProvider>
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  )
}

type Tab = 'inbox' | 'archived'

function InboxList({
  unread,
  mobile = false,
  onClose,
}: {
  unread: number
  mobile?: boolean
  onClose?: () => void
}) {
  const [tab, setTab] = useState<Tab>('inbox')
  const { notifications, isLoading, hasMore, fetchMore, readAll } = useNotifications({
    archived: tab === 'archived',
    limit: 20,
  })
  const [loadingMore, setLoadingMore] = useState(false)

  const items = notifications ?? []
  const canMarkAllAsRead = tab === 'inbox' && unread > 0
  const onLoadMore = async () => {
    setLoadingMore(true)
    try {
      await fetchMore()
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <div className={cn('flex flex-col', mobile ? 'h-full min-h-0' : 'max-h-[32rem]')}>
      <div className="shrink-0 border-b border-border">
        <div className="flex items-center justify-between px-4 pt-4">
          <h2 className="text-heading-2 font-medium text-foreground">Notifications</h2>
          <div className={cn('flex items-center', mobile ? 'gap-3' : 'gap-1')}>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Mark all as read"
                    disabled={!canMarkAllAsRead}
                    onClick={() => readAll()}
                  >
                    <CheckCheck />
                  </Button>
                </span>
              </TooltipTrigger>
              <TooltipContent side="bottom" sideOffset={4}>
                {tab === 'archived'
                  ? 'Mark all as read is only available in Inbox'
                  : unread === 0
                    ? 'No unread notifications'
                    : 'Mark all as read'}
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button asChild variant="ghost" size="icon-sm">
                  <Link
                    to="/settings/notifications"
                    aria-label="Notification settings"
                    onClick={onClose}
                  >
                    <Settings />
                  </Link>
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom" sideOffset={4}>
                Notification settings
              </TooltipContent>
            </Tooltip>
            {mobile && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Close notifications"
                    onClick={onClose}
                  >
                    <X />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="bottom" sideOffset={4}>
                  Close notifications
                </TooltipContent>
              </Tooltip>
            )}
          </div>
        </div>
        <div className="mt-4 flex items-end gap-5 px-4">
          <TabButton padding="compact" active={tab === 'inbox'} onClick={() => setTab('inbox')}>
            Inbox
            {unread > 0 && (
              <InlineBadge
                variant="soft"
                shape="square"
                className="ml-1 h-4 min-w-4 justify-center border-0 bg-secondary px-1 py-0 font-sans text-[10px] font-medium tracking-normal text-secondary-foreground"
              >
                {unread}
              </InlineBadge>
            )}
          </TabButton>
          <TabButton padding="compact" active={tab === 'archived'} onClick={() => setTab('archived')}>
            Archived
          </TabButton>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
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
              <NotificationRow
                key={n.id}
                notification={n}
                tab={tab}
                mobile={mobile}
                onClose={onClose}
              />
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

export function NotificationRow({
  notification,
  tab,
  mobile = false,
  onClose,
}: {
  notification: Notification
  tab: Tab
  mobile?: boolean
  onClose?: () => void
}) {
  const navigate = useNavigate()

  const follow = (redirect: Redirect) => {
    const url = redirect?.url
    if (!url) return
    if (/^https?:\/\//.test(url)) {
      window.open(url, redirect?.target ?? '_blank', 'noopener,noreferrer')
    } else {
      navigate(url)
      onClose?.()
    }
  }

  const onRowClick = () => {
    if (!notification.isRead) notification.read()
    follow(notification.redirect)
  }

  const activatePrimaryAction = (event: SyntheticEvent) => {
    event.stopPropagation()
    notification.completePrimary()
    follow(notification.primaryAction?.redirect)
  }

  const activateSecondaryAction = (event: SyntheticEvent) => {
    event.stopPropagation()
    notification.completeSecondary()
    follow(notification.secondaryAction?.redirect)
  }

  return (
    <li
      className={cn(
        'group relative border-b border-border px-4 py-3 transition-colors last:border-b-0 hover:bg-muted/60',
        !notification.isRead && 'bg-muted/30',
        mobile && 'pr-12',
      )}
    >
      <button
        type="button"
        onClick={onRowClick}
        className="flex w-full cursor-pointer gap-3 rounded-sm text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      >
        <NotificationTypeIcon notification={notification} />
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-baseline gap-3">
            {notification.subject && (
              <span className="min-w-0 flex-1 truncate text-body-sm font-medium text-foreground">
                {notification.subject}
              </span>
            )}
            {!mobile && (
              <span className="ml-auto shrink-0 text-[11px] text-faint-foreground transition-opacity group-hover:opacity-0 group-focus-within:opacity-0">
                {formatRelativeTime(notification.createdAt)}
              </span>
            )}
          </span>
          <span className="mt-0.5 block text-xs text-muted-foreground">
            {notification.body}
          </span>
          {mobile && (
            <span className="mt-1 block text-[11px] text-faint-foreground">
              {formatRelativeTime(notification.createdAt)}
            </span>
          )}
          {!notification.isRead && <span className="sr-only">Unread</span>}
        </span>
      </button>

      {(notification.primaryAction || notification.secondaryAction) && (
        <span className="relative z-10 ml-10 mt-2 flex gap-2">
          {notification.primaryAction && (
            <Button asChild size="xs">
              <button type="button" onClick={activatePrimaryAction}>
                {notification.primaryAction.label}
              </button>
            </Button>
          )}
          {notification.secondaryAction && (
            <Button asChild variant="outline" size="xs">
              <button type="button" onClick={activateSecondaryAction}>
                {notification.secondaryAction.label}
              </button>
            </Button>
          )}
        </span>
      )}

      {/* Touch layouts keep row actions visible; pointer layouts reveal them on hover or focus. */}
      <div
        className={cn(
          'absolute right-3 top-2 flex w-24 items-center justify-end transition-opacity',
          mobile
            ? 'opacity-100'
            : 'pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100',
        )}
      >
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

function NotificationTypeIcon({ notification }: { notification: Notification }) {
  const config = resolveNotificationIcon(notification.workflow?.identifier)
  const Icon = config.icon

  return (
    <span className="relative mt-0.5 size-7 shrink-0" aria-hidden>
      {notification.avatar ? (
        <img src={notification.avatar} alt="" className="size-7 rounded-full object-cover" />
      ) : (
        <span
          className={cn(
            'flex size-7 items-center justify-center rounded-sm border border-border bg-transparent',
            NOTIFICATION_ICON_TONE[config.tone],
          )}
        >
          <Icon className="size-3.5" />
        </span>
      )}
      {!notification.isRead && (
        <span className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-primary ring-2 ring-popover" />
      )}
    </span>
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
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          onClick={(e) => {
            e.stopPropagation()
            onClick()
          }}
          className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          {children}
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom" sideOffset={4}>
        {label}
      </TooltipContent>
    </Tooltip>
  )
}
