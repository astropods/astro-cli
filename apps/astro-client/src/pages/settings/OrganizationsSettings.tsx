import { Link } from 'react-router'
import { Settings } from 'lucide-react'
import { BuildingOffice2Icon, PlusIcon } from '@heroicons/react/24/outline'
import { Separator } from '@/components/ui/separator'
import { Button } from '@/components/ui/button'
import { InlineBadge } from '@/components/InlineBadge'
import { useAuth } from '@/lib/auth'

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
              <div className="size-9 rounded-md shrink-0 flex items-center justify-center bg-stone-200 dark:bg-stone-700">
                <BuildingOffice2Icon className="size-4 text-stone-500 dark:text-stone-400" />
              </div>

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
                <Link to={`/settings/org/${org.name}/secrets`}>
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
