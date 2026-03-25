import { Skeleton } from "@/components/ui/skeleton";

export function BlueprintDetailSkeleton() {
  return (
    <div className="flex flex-col flex-1 min-h-0 bg-surface">
      {/* Breadcrumb skeleton */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-border bg-stone-200 dark:bg-background">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-3.5 w-3.5" />
          <Skeleton className="h-4 w-40" />
        </div>
        <div className="flex gap-1">
          <Skeleton className="size-7 rounded" />
          <Skeleton className="size-7 rounded" />
        </div>
      </div>

      <div className="flex flex-1 overflow-y-auto">
        <div className="flex min-w-0 flex-1 max-w-[1200px] mx-auto">
          {/* Left column */}
          <div className="flex-1 min-w-0 p-6 md:p-8">
            {/* Header: avatar + name + byline */}
            <div className="flex items-start gap-4 mb-4">
              <Skeleton className="size-14 rounded-md shrink-0" />
              <div>
                <Skeleton className="h-6 w-48 mb-2" />
                <Skeleton className="h-3.5 w-24" />
              </div>
            </div>
            {/* Tags */}
            <div className="flex gap-1.5 mb-8">
              <Skeleton className="h-6 w-20 rounded-full" />
              <Skeleton className="h-6 w-16 rounded-full" />
            </div>
            {/* README body */}
            <div className="space-y-3">
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-4 w-5/6" />
              <Skeleton className="h-4 w-2/3" />
            </div>
          </div>

          {/* Right sidebar skeleton */}
          <div className="hidden min-[900px]:block w-[340px] shrink-0 pl-0 pr-8 pt-10 pb-6">
            <div className="rounded-lg border border-border p-5 space-y-5">
              <Skeleton className="h-10 w-full rounded" />
              <div className="h-px bg-border" />
              <div className="space-y-1">
                <Skeleton className="h-3 w-16" />
                <Skeleton className="h-4 w-full" />
              </div>
              <div className="h-px bg-border" />
              <div className="flex items-center gap-3">
                <Skeleton className="size-10 rounded-full" />
                <Skeleton className="h-4 w-24" />
              </div>
              <div className="h-px bg-border" />
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <Skeleton className="h-3 w-12" />
                  <Skeleton className="h-4 w-16" />
                </div>
                <div className="space-y-1">
                  <Skeleton className="h-3 w-12" />
                  <Skeleton className="h-4 w-16" />
                </div>
              </div>
              <div className="h-px bg-border" />
              <div className="space-y-2">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-4 w-28" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
