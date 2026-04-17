import type { Meta, StoryObj } from "@storybook/react-vite"
import { EllipsisHorizontalIcon } from "@heroicons/react/24/outline"

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/StatusBadge"

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
              <StatusBadge color={statusColor[r.status]} indicator>
                {r.status}
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
          <TableCell colSpan={3}>
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <h3 className="text-heading-4 text-foreground mb-1">No results yet</h3>
              <p className="text-body-sm text-muted-foreground mb-6 max-w-md">
                Create your first entry to get started.
              </p>
              <Button>Create entry</Button>
            </div>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  ),
}
