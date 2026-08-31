import { Loader2 } from "lucide-react";
import { InfoHint } from "@/components/InfoHint";
import { InlineBadge } from "@/components/InlineBadge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Spinner } from "@/components/ui/spinner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { EvaluatorValueControl } from "../EvaluatorValueControl";
import type { EvaluationRow } from "./evaluation-rows";

interface ReviewQueueEvaluationResultsProps {
  rows: EvaluationRow[];
  values: Map<string, unknown>;
  editedKeys: Set<string>;
  scored: boolean;
  disabled?: boolean;
  loading?: boolean;
  onChange: (key: string, value: unknown) => void;
}

export function ReviewQueueEvaluationResults({
  rows,
  values,
  editedKeys,
  scored,
  disabled,
  loading = false,
  onChange,
}: ReviewQueueEvaluationResultsProps) {
  if (loading && rows.length === 0) {
    return (
      <div className="flex items-center justify-center py-6">
        <Spinner delay={300} />
      </div>
    );
  }

  if (rows.length === 0) {
    return null;
  }

  const showConfidence = scored || loading;

  return (
    <TooltipProvider delayDuration={300}>
      <Table
        bare
        className="w-full table-fixed"
        containerClassName="animate-in fade-in slide-in-from-top-1 duration-200 ease-out"
      >
        <TableHeader className="bg-transparent dark:bg-transparent">
          <TableRow>
            <TableHead className="pl-0 text-faint-foreground">
              Evaluator
            </TableHead>
            <TableHead
              className={cn(
                "w-[11.5rem] text-faint-foreground",
                !showConfidence && "pr-0 text-right",
              )}
            >
              Result
            </TableHead>
            {showConfidence && (
              <TableHead className="w-[6rem] pr-0 text-right text-faint-foreground">
                Confidence
              </TableHead>
            )}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => {
            const edited = row.evaluated && editedKeys.has(row.key);
            const detail = row.explanation;
            const pending = loading && !row.evaluated;
            return (
              <TableRow key={row.key} className="border-b-0">
                <TableCell className="py-2 pl-0 align-top">
                  <div className="flex items-center gap-1.5 text-body-sm font-medium text-foreground">
                    {row.label}
                    {row.description && (
                      <InfoHint label={`About ${row.label}`}>
                        {row.description}
                      </InfoHint>
                    )}
                    {edited && (
                      <InlineBadge
                        variant="soft"
                        className="text-foreground-accent"
                      >
                        Updated
                      </InlineBadge>
                    )}
                  </div>
                  {detail && (
                    <p className="mt-1 max-w-3xl text-body-sm leading-relaxed text-muted-foreground">
                      {detail}
                    </p>
                  )}
                </TableCell>
                <TableCell
                  className={cn(
                    "pb-2 align-top",
                    detail ? "pt-8" : "pt-2",
                    !showConfidence && "pr-0",
                  )}
                >
                  <div className={cn("flex", !showConfidence && "justify-end")}>
                    {pending ? (
                      <span className="flex h-7 items-center gap-1.5 text-body-sm text-muted-foreground">
                        <Loader2
                          aria-hidden
                          className="dp-spin size-3.5 flex-none"
                        />
                        Pending
                      </span>
                    ) : (
                      row.output && (
                        <EvaluatorValueControl
                          output={row.output}
                          label={row.label}
                          value={values.get(row.key)}
                          disabled={disabled}
                          onChange={(value) => onChange(row.key, value)}
                        />
                      )
                    )}
                  </div>
                </TableCell>
                {showConfidence && (
                  <TableCell
                    className={cn(
                      "pb-2 pr-0 text-right align-top tabular-nums text-foreground",
                      detail ? "pt-8" : "pt-2",
                    )}
                  >
                    {row.confidence === null ? "—" : `${row.confidence}%`}
                  </TableCell>
                )}
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TooltipProvider>
  );
}
