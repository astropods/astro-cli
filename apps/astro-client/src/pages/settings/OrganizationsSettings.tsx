import { Link, type MetaFunction } from 'react-router'
import { Settings } from 'lucide-react'
import { PlusIcon } from '@heroicons/react/24/outline'
import { Separator } from '@/components/ui/separator'
import { Button } from '@/components/ui/button'
import { InlineBadge } from '@/components/InlineBadge'
import { UserAvatar } from '@/components/UserAvatar'
import { useAuth } from '@/lib/auth'

export const meta: MetaFunction = () => [{ title: "Organizations - Settings | Astro" }];

export default function OrganizationsSettings() {
  const { accounts } = useAuth()
  const orgs = accounts.filter(a => a.type === 'organization')

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-heading-2 text-foreground">Organizations</h2>
          <p className="text-[13px] text-muted-foreground mt-1">
            Manage your organizations and access settings
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button size="sm" asChild>
            <Link to="/organization/new">
              <PlusIcon className="size-3.5" />
              Create organization
            </Link>
          </Button>
        </div>
      </div>

      <Separator />

      {orgs.length === 0 ? (
        <p className="text-[13px] text-muted-foreground py-4">You're not a member of any organizations yet.</p>
      ) : (
        <div className="space-y-2">
          {orgs.map(org => (
            <div
              key={org.name}
              className="flex items-center gap-4 px-5 py-4 rounded-lg border border-border"
            >
              <UserAvatar
                handle={org.name}
                name={org.display_name || org.name}
                className="size-9 shrink-0 ring-1 ring-border"
              />

              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2.5">
                  <p className="text-sm font-medium text-foreground">{org.display_name || org.name}</p>
                  {org.role && (
                    <InlineBadge
                      variant="soft"
                      className={org.role === 'admin' ? 'bg-teal-100 text-teal-800 dark:bg-teal-900/40 dark:text-teal-300' : 'bg-stone-200 dark:bg-stone-700'}
                    >
                      {org.role}
                    </InlineBadge>
                  )}
                </div>
              </div>

              <Button variant="outline" size="sm" asChild>
                <Link to={`/settings/org/${org.name}/general`}>
                  <Settings className="size-3.5" />
                  Settings
                </Link>
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
