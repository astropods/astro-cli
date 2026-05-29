import { useMemo, useState } from "react"
import type { Meta, StoryObj } from "@storybook/react-vite"
import { CircleStackIcon, EllipsisHorizontalIcon, PlusIcon } from "@heroicons/react/24/outline"
import { Users, Bot } from "lucide-react"

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  type SortDirection,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/StatusBadge"
import { FilterInput } from "@/components/FilterInput"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

const meta = {
  title: "Design System/Primitives/Table",
  component: Table,
} satisfies Meta<typeof Table>

export default meta
type Story = StoryObj<typeof meta>

type Row = {
  name: string
  status: "running" | "pending" | "failed"
  provider: string
  created: string
}

const rows: Row[] = [
  { name: "primary-store", status: "running", provider: "Postgres", created: "2h ago" },
  { name: "embeddings-cache", status: "running", provider: "Redis", created: "1d ago" },
  { name: "semantic-search", status: "pending", provider: "Qdrant", created: "3d ago" },
  { name: "archive-index", status: "failed", provider: "Pinecone", created: "1w ago" },
]

const statusColor: Record<Row["status"], "success" | "warning" | "error"> = {
  running: "success",
  pending: "warning",
  failed: "error",
}

export const Default: Story = {
  render: () => (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Provider</TableHead>
          <TableHead>Created</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((r) => (
          <TableRow key={r.name}>
            <TableCell className="font-medium text-foreground">{r.name}</TableCell>
            <TableCell>
              <StatusBadge color={statusColor[r.status]}>
                {r.status[0].toUpperCase() + r.status.slice(1)}
              </StatusBadge>
            </TableCell>
            <TableCell className="text-muted-foreground">{r.provider}</TableCell>
            <TableCell className="text-muted-foreground">{r.created}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  ),
}

export const InteractiveRows: Story = {
  render: () => (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Provider</TableHead>
          <TableHead>Created</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((r) => (
          <TableRow key={r.name} interactive onClick={() => alert(`Open ${r.name}`)}>
            <TableCell className="font-medium text-foreground">{r.name}</TableCell>
            <TableCell className="text-muted-foreground">{r.provider}</TableCell>
            <TableCell className="text-muted-foreground">{r.created}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  ),
}

export const WithActionsColumn: Story = {
  render: () => (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Provider</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="w-10" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((r) => (
          <TableRow key={r.name} interactive>
            <TableCell className="font-medium text-foreground">{r.name}</TableCell>
            <TableCell className="text-muted-foreground">{r.provider}</TableCell>
            <TableCell className="text-muted-foreground">{r.created}</TableCell>
            <TableCell onClick={(e) => e.stopPropagation()}>
              <Button variant="ghost" size="icon" className="size-7">
                <EllipsisHorizontalIcon className="size-4" />
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  ),
}

export const Loading: Story = {
  render: () => (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Provider</TableHead>
          <TableHead>Created</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {[0, 1, 2].map((i) => (
          <TableRow key={i}>
            <TableCell colSpan={3}>
              <Skeleton className="h-5 w-full rounded" />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  ),
}

export const Sortable: Story = {
  render: () => {
    type Col = "name" | "provider" | "created"
    const [sortKey, setSortKey] = useState<Col>("name")
    const [direction, setDirection] = useState<SortDirection>("asc")
    function handleSort(col: Col) {
      if (col === sortKey) setDirection((d) => (d === "asc" ? "desc" : "asc"))
      else { setSortKey(col); setDirection("asc") }
    }
    const dirFor = (col: Col): SortDirection | undefined => (col === sortKey ? direction : undefined)
    const sorted = [...rows].sort((a, b) => {
      const cmp = String(a[sortKey]).localeCompare(String(b[sortKey]))
      return direction === "asc" ? cmp : -cmp
    })
    return (
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead sortable sortDirection={dirFor("name")} onSort={() => handleSort("name")}>Name</TableHead>
            <TableHead sortable sortDirection={dirFor("provider")} onSort={() => handleSort("provider")}>Provider</TableHead>
            <TableHead sortable sortDirection={dirFor("created")} onSort={() => handleSort("created")}>Created</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.map((r) => (
            <TableRow key={r.name}>
              <TableCell className="font-medium text-foreground">{r.name}</TableCell>
              <TableCell className="text-muted-foreground">{r.provider}</TableCell>
              <TableCell className="text-muted-foreground">{r.created}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    )
  },
}

// `bare` strips the rounded-border card chrome so the table flows in the
// page surface. Use when a parent container already provides the framing
// (e.g. inside a Card or a Sheet) and the table's own border would nest.
export const Bare: Story = {
  render: () => (
    <Table bare>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Provider</TableHead>
          <TableHead>Created</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((r) => (
          <TableRow key={r.name}>
            <TableCell className="font-medium text-foreground">{r.name}</TableCell>
            <TableCell className="text-muted-foreground">{r.provider}</TableCell>
            <TableCell className="text-muted-foreground">{r.created}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  ),
}

// Panel pattern: title + view toggle + search live inside the bordered
// container above the table (Table's `header` slot). This is how the
// Insights "Spend" widget is composed.
//
// Fixtures live at module scope so they're stable references — keeps the
// useMemo deps below honest (no need to add them to the dep array because
// they never change identity across renders).
type PanelView = "people" | "agents"
const PANEL_PEOPLE = [
  { id: "u_alice", name: "Alice Chen", spend: "$4,210", share: "32.8%" },
  { id: "u_bob",   name: "Bob Singh",  spend: "$3,084", share: "24.0%" },
  { id: "u_carol", name: "Carol Park", spend: "$2,415", share: "18.8%" },
]
const PANEL_AGENTS = [
  { id: "swipefile",      name: "swipefile",      spend: "$0.02", share: "44.4%" },
  { id: "memory-box",     name: "memory-box",     spend: "$0.02", share: "33.3%" },
  { id: "astro-companion", name: "astro-companion", spend: "$0.01", share: "22.2%" },
]

export const WithPanelHeader: Story = {
  render: () => {
    const [view, setView] = useState<PanelView>("people")
    const [q, setQ] = useState("")

    const needle = q.trim().toLowerCase()
    const filteredPeople = useMemo(
      () => (needle ? PANEL_PEOPLE.filter((p) => p.name.toLowerCase().includes(needle)) : PANEL_PEOPLE),
      [needle],
    )
    const filteredAgents = useMemo(
      () => (needle ? PANEL_AGENTS.filter((a) => a.name.toLowerCase().includes(needle)) : PANEL_AGENTS),
      [needle],
    )

    const panelHeader = (
      <div className="flex flex-col gap-3">
        <h3 className="text-heading-4 text-foreground">Spend</h3>
        <div className="flex flex-col gap-3 @md:flex-row @md:items-center @md:justify-between">
          <ToggleGroup
            type="single"
            variant="word"
            value={view}
            onValueChange={(v) => { if (v === "people" || v === "agents") setView(v) }}
            className="h-8 rounded-sm border-input bg-card"
          >
            <ToggleGroupItem value="people" className="gap-2 py-1 text-body-sm">
              <Users className="size-3.5 text-muted-foreground" aria-hidden />
              People
              <span className="text-faint-foreground tabular-nums">{PANEL_PEOPLE.length}</span>
            </ToggleGroupItem>
            <ToggleGroupItem value="agents" className="gap-2 py-1 text-body-sm">
              <Bot className="size-3.5 text-muted-foreground" aria-hidden />
              Agents
              <span className="text-faint-foreground tabular-nums">{PANEL_AGENTS.length}</span>
            </ToggleGroupItem>
          </ToggleGroup>
          <FilterInput
            containerClassName="h-8 w-full @md:max-w-xs"
            placeholder="Search users or agents..."
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
      </div>
    )

    return (
      <Table header={panelHeader}>
        <TableHeader>
          <TableRow>
            <TableHead>{view === "people" ? "User" : "Agent"}</TableHead>
            <TableHead className="text-right">Total Spend</TableHead>
            <TableHead className="text-right">% Total</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(view === "people" ? filteredPeople : filteredAgents).map((r) => (
            <TableRow key={r.id}>
              <TableCell className="font-medium text-foreground">{r.name}</TableCell>
              <TableCell className="text-right font-medium text-foreground">{r.spend}</TableCell>
              <TableCell className="text-right text-muted-foreground">{r.share}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    )
  },
}

export const Empty: Story = {
  render: () => (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Provider</TableHead>
          <TableHead>Created</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell colSpan={3} className="p-0">
            <div className="flex min-h-[260px] flex-col items-center justify-center text-center">
              <div className="mb-3.5 flex size-10 items-center justify-center rounded-md bg-muted">
                <CircleStackIcon className="size-5 text-faint-foreground" />
              </div>
              <p className="text-heading-4 text-foreground mb-1.5">No results yet</p>
              <p className="text-body text-faint-foreground mb-6">
                Create your first entry to get started.
              </p>
              <Button>
                <PlusIcon className="size-4" />
                Create entry
              </Button>
            </div>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  ),
}
