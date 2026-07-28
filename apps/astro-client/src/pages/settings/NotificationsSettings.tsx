import type { MetaFunction } from 'react-router'
import { Loader2, Lock } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SectionHeader } from '@/components/settings/SettingsShared'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import {
  useNotificationPreferences,
  useUpdateNotificationPreference,
  useSendTestNotification,
} from '@/api/queries'
import { useAuth } from '@/lib/auth'
import { ErrorPanel } from '@/components/ui/status-panel'
import type { NotificationPreference } from '@/lib/api'

export const meta: MetaFunction = () => [{ title: "Notifications - Settings | Astro" }]

type Channel = 'email' | 'in_app'

/** One catalog row: name/description + email and in-app toggles. Critical rows
 * are locked on. */
function PreferenceRow({ pref, account }: { pref: NotificationPreference; account: string }) {
  const update = useUpdateNotificationPreference(account)

  const toggle = (channel: Channel, enabled: boolean) => {
    update.mutate({
      type: pref.type,
      email: channel === 'email' ? enabled : pref.email,
      in_app: channel === 'in_app' ? enabled : pref.in_app,
    })
  }

  const channelCheckbox = (channel: Channel, checked: boolean, label: string) => {
    const control = (
      <label className="flex items-center gap-2 text-xs text-muted-foreground cursor-pointer">
        <input
          type="checkbox"
          checked={checked}
          disabled={pref.critical || update.isPending}
          onChange={(e) => toggle(channel, e.target.checked)}
          className="size-4 shrink-0 accent-primary disabled:opacity-50 disabled:cursor-not-allowed"
        />
        {label}
      </label>
    )
    if (!pref.critical) return control
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex">{control}</span>
        </TooltipTrigger>
        <TooltipContent>Always on — required for account safety</TooltipContent>
      </Tooltip>
    )
  }

  return (
    <div className="flex items-center justify-between gap-4 py-3 border-b border-border last:border-b-0">
      <div className="min-w-0">
        <p className="text-body-sm font-medium text-foreground flex items-center gap-1.5">
          {pref.name}
          {pref.critical && <Lock className="size-3 text-muted-foreground" />}
        </p>
        {pref.description && <p className="text-xs text-muted-foreground">{pref.description}</p>}
      </div>
      <div className="flex items-center gap-6 shrink-0">
        {channelCheckbox('email', pref.email, 'Email')}
        {channelCheckbox('in_app', pref.in_app, 'In-app')}
      </div>
    </div>
  )
}

export function NotificationsPanel({ account }: { account: string }) {
  const { data, isLoading, isError, refetch } = useNotificationPreferences(account)
  const sendTest = useSendTestNotification(account)

  // Group the catalog by category, preserving first-seen order.
  const groups: { category: string; prefs: NotificationPreference[] }[] = []
  for (const pref of data?.preferences ?? []) {
    let group = groups.find((g) => g.category === pref.category)
    if (!group) {
      group = { category: pref.category, prefs: [] }
      groups.push(group)
    }
    group.prefs.push(pref)
  }

  return (
    <TooltipProvider>
      <div>
        <SectionHeader
          title="Notifications"
          subtitle="Choose which alerts you receive, and on which channels."
          action={
            <Button
              size="sm"
              variant="outline"
              disabled={sendTest.isPending}
              onClick={() => sendTest.mutate()}
            >
              {sendTest.isPending && <Loader2 className="size-3.5 animate-spin" />}
              Send test
            </Button>
          }
        />

        {sendTest.isSuccess && (
          <p className="text-xs text-muted-foreground mt-3">
            Test notification queued — check your email and in-app inbox.
          </p>
        )}

        {data && !data.delivery_enabled && (
          <p className="text-xs text-muted-foreground mt-4 rounded-md border border-border bg-card px-3 py-2">
            Notifications aren't configured for this environment yet.
          </p>
        )}

        <div className="mt-4">
          {isLoading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground py-8 justify-center">
              <Loader2 className="size-4 animate-spin" />
              Loading preferences…
            </div>
          ) : isError ? (
            <ErrorPanel title="Couldn't load preferences">
              <Button size="sm" variant="outline" onClick={() => refetch()}>
                Retry
              </Button>
            </ErrorPanel>
          ) : (
            <div className="flex flex-col gap-6">
              {groups.map((group) => (
                <div key={group.category}>
                  <p className="text-label uppercase text-faint-foreground mb-1">{group.category}</p>
                  {group.prefs.map((pref) => (
                    <PreferenceRow key={pref.type} pref={pref} account={account} />
                  ))}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </TooltipProvider>
  )
}

export default function NotificationsSettings() {
  const { personalAccount } = useAuth()
  return <NotificationsPanel account={personalAccount?.name ?? ''} />
}
