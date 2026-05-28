import * as React from "react"

import { cn } from "@/lib/utils"

function Table({
  className,
  containerClassName,
  children,
  ...props
}: React.ComponentProps<"table"> & { containerClassName?: string }) {
  return (
    <div
      data-slot="table-container"
      className={cn("rounded-sm border border-border overflow-hidden", containerClassName)}
    >
      <div className="relative w-full overflow-auto">
        <table
          data-slot="table"
          className={cn("w-full border-collapse text-body leading-5", className)}
          {...props}
        >
          {children}
        </table>
      </div>
    </div>
  )
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("bg-muted border-b border-border", className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn(className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        "border-t border-border font-medium [&>tr]:border-0",
        className,
      )}
      {...props}
    />
  )
}

function TableRow({
  className,
  interactive,
  ...props
}: React.ComponentProps<"tr"> & { interactive?: boolean }) {
  return (
    <tr
      data-slot="table-row"
      data-interactive={interactive ? "true" : undefined}
      className={cn(
        "border-b border-border last:border-b-0",
        interactive && "cursor-pointer hover:bg-muted/40 transition-colors",
        className,
      )}
      {...props}
    />
  )
}

function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "px-4 py-2 text-left text-body-sm font-medium text-muted-foreground",
        className,
      )}
      {...props}
    />
  )
}

function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  return (
    <td
      data-slot="table-cell"
      className={cn("px-4 py-2 align-middle", className)}
      {...props}
    />
  )
}

function TableCaption({
  className,
  ...props
}: React.ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("text-muted-foreground mt-4 text-body-sm", className)}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableRow,
  TableHead,
  TableCell,
  TableCaption,
}
