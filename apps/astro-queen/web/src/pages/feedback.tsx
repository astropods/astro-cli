import { useState } from "react";
import { useFeedback } from "@/api/admin";
import type { FeedbackSubmission } from "@/api/admin";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { formatDateTime } from "@/lib/utils";

export function FeedbackPage() {
  const { data, isLoading, error } = useFeedback();
  const [search, setSearch] = useState("");

  const filtered = data?.submissions?.filter((f) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      f.message.toLowerCase().includes(q) ||
      f.user_email.toLowerCase().includes(q) ||
      f.page_url.toLowerCase().includes(q)
    );
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">User Feedback</h2>
          <p className="text-[10px] text-muted-foreground">
            Feedback submissions from users.
            {data?.count != null && ` ${data.count} total.`}
          </p>
        </div>
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search feedback..."
          className="w-56"
        />
      </div>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}

      <div className="overflow-x-auto rounded-lg glass">
        <table className="w-full text-[11px] whitespace-nowrap">
          <thead>
            <tr className="border-b border-glass-border-honey glass-subtle">
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">
                Date
              </th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">
                User
              </th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">
                Message
              </th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">
                Page
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered?.length === 0 && (
              <tr>
                <td
                  colSpan={4}
                  className="px-2 py-4 text-center text-muted-foreground"
                >
                  {search ? "No matching feedback." : "No feedback yet."}
                </td>
              </tr>
            )}
            {filtered?.map((f) => (
              <FeedbackRow key={f.id} feedback={f} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function FeedbackRow({ feedback: f }: { feedback: FeedbackSubmission }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <>
      <tr
        className="border-b border-glass-border-honey hover:bg-glass-light cursor-pointer"
        onClick={() => setExpanded((e) => !e)}
      >
        <td className="px-2 py-1.5 text-muted-foreground">
          {formatDateTime(f.created_at)}
        </td>
        <td className="px-2 py-1.5">{f.user_email || f.user_id}</td>
        <td className="px-2 py-1.5 max-w-[400px] truncate" title={f.message}>
          {f.message}
        </td>
        <td
          className="px-2 py-1.5 text-muted-foreground max-w-[200px] truncate"
          title={f.page_url}
        >
          {f.page_url || "—"}
        </td>
      </tr>
      {expanded && (
        <tr className="border-b border-glass-border-honey bg-glass-light">
          <td colSpan={4} className="px-3 py-2">
            <p className="text-xs whitespace-pre-wrap break-words">
              {f.message}
            </p>
          </td>
        </tr>
      )}
    </>
  );
}
