import { InfoHint } from "@/components/InfoHint";
import { StatusBadge, type StatusBadgeColor } from "@/components/StatusBadge";
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
import type { TraceEvaluatorResult } from "@/lib/api";
import { cn } from "@/lib/utils";

function evaluatorLabel(result: TraceEvaluatorResult) {
  return result.label ?? result.key;
}

function displayValue(result: TraceEvaluatorResult) {
  if (result.status !== "completed") {
    return result.status === "failed" ? "Error" : "Pending";
  }
  if (typeof result.value === "boolean") {
    return result.value ? "True" : "False";
  }
  if (result.value === null || result.value === undefined) {
    return "—";
  }
  const value = String(result.value);
  return value.charAt(0).toUpperCase() + value.slice(1).replace(/_/g, " ");
}

function detailText(result: TraceEvaluatorResult) {
  if (result.explanation) {
    return result.explanation;
  }
  return result.error ? "Evaluation could not be generated." : null;
}

function valueColor(result: TraceEvaluatorResult): StatusBadgeColor {
  if (result.status === "completed") {
    return "primary";
  }
  return result.status === "failed" ? "error" : "muted";
}

function confidencePercent(result: TraceEvaluatorResult) {
  return Math.round(Math.min(1, Math.max(0, result.confidence)) * 100);
}

export function ReviewQueueEvaluationResults({
  evaluators,
  loading = false,
  noResults = false,
}: {
  evaluators: TraceEvaluatorResult[];
  loading?: boolean;
  noResults?: boolean;
}) {
  if (loading && !noResults && evaluators.length === 0) {
    return (
      <div className="flex items-center justify-center py-6">
        <Spinner delay={300} />
      </div>
    );
  }

  if (noResults || evaluators.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-6 text-center">
        <p className="text-body font-medium text-foreground">
          The evaluator couldn’t return a result for this trace.
        </p>
        <p className="mt-1 text-body-sm text-muted-foreground">
          You can still evaluate it manually.
        </p>
      </div>
    );
  }

  return (
    <TooltipProvider delayDuration={300}>
      <Table
        bare
        className="w-full table-fixed"
        containerClassName="animate-in fade-in slide-in-from-top-1 duration-200 ease-out"
      >
      <TableHeader className="bg-transparent dark:bg-transparent">
        <TableRow>
          <TableHead className="w-[70%] pl-0 text-faint-foreground">
            Evaluator
          </TableHead>
          <TableHead className="w-[18%] text-faint-foreground">Result</TableHead>
          <TableHead className="w-[12%] pr-0 text-right text-faint-foreground">
            Confidence
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {evaluators.map((result) => {
          const detail = detailText(result);
          return (
            <TableRow key={result.key} className="border-b-0">
              <TableCell className="py-2 pl-0 align-top">
                <div className="flex items-center gap-1.5 text-body-sm font-medium text-foreground">
                  {evaluatorLabel(result)}
                  {result.description && (
                    <InfoHint label={`About ${evaluatorLabel(result)}`}>
                      {result.description}
                    </InfoHint>
                  )}
                </div>
                {detail && (
                  <p className="mt-1 max-w-3xl text-body-sm leading-relaxed text-muted-foreground">
                    {detail}
                  </p>
                )}
              </TableCell>
              <TableCell
                className={cn("pb-2 align-top", detail ? "pt-8" : "pt-2")}
              >
                <StatusBadge
                  color={valueColor(result)}
                  size="md"
                  className="font-sans tracking-normal"
                >
                  {displayValue(result)}
                </StatusBadge>
              </TableCell>
              <TableCell
                className={cn(
                  "pb-2 pr-0 text-right align-top tabular-nums text-foreground",
                  detail ? "pt-8" : "pt-2",
                )}
              >
                {result.status === "completed"
                  ? `${confidencePercent(result)}%`
                  : "—"}
              </TableCell>
            </TableRow>
          );
        })}
        </TableBody>
      </Table>
    </TooltipProvider>
  );
}
