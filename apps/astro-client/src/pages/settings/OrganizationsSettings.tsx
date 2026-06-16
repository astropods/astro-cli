import { Link, type MetaFunction } from 'react-router'
import { ChevronRight } from 'lucide-react'
import { PlusIcon } from '@heroicons/react/24/outline'
import { Button } from '@/components/ui/button'
import { Tag, type TagColor } from '@/components/Tag'
import { UserAvatar } from '@/components/UserAvatar'
import { SectionHeader } from '@/components/settings/SettingsShared'
import { useAuth } from '@/lib/auth'

export const meta: MetaFunction = () => [{ title: "Organizations - Settings | Astro" }];

const ORG_ROLE_TAG: Record<string, { label: string; color: TagColor }> = {
  owner:  { label: 'Owner',  color: 'yellow'  },
  admin:  { label: 'Admin',  color: 'blue'    },
  member: { label: 'Member', color: 'default' },
}

export default function OrganizationsSettings() {
  const { accounts } = useAuth()
  const orgs = accounts.filter(a => a.type === 'organization')

  return (
    <>
      <SectionHeader
        title="Organizations"
        subtitle="Manage your organizations and access settings"
        action={
          <Button size="sm" asChild>
            <Link to="/organization/new">
              <PlusIcon className="size-3.5" />
              Create organization
            </Link>
          </Button>
        }
      />

      {orgs.length === 0 ? (
        <p className="text-[13px] text-muted-foreground py-4">You're not a member of any organizations yet.</p>
      ) : (
        <div className="space-y-3">
          {orgs.map(org => {
            const roleTag = org.role ? ORG_ROLE_TAG[org.role.toLowerCase()] : null
            return (
            <Link
              key={org.name}
              to={`/settings/org/${org.name}/general`}
              className="group flex items-center gap-4 px-4 py-3 rounded-lg border border-border transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <UserAvatar
                handle={org.name}
                name={org.display_name || org.name}
                className="size-9 shrink-0 ring-1 ring-border"
              />

              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-foreground">{org.display_name || org.name}</p>
                {roleTag && (
                  <div className="mt-1">
                    <Tag color={roleTag.color}>{roleTag.label}</Tag>
                  </div>
                )}
              </div>

              <ChevronRight
                aria-hidden
                className="size-4 shrink-0 text-muted-foreground/60 transition-colors group-hover:text-foreground"
              />
            </Link>
            )
          })}
        </div>
      )}
    </>
  )
}
