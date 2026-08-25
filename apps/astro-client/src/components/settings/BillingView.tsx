import { useState } from "react";
import { Download } from "lucide-react";
import { toast } from "sonner";
import { useBillingInvoices, useDownloadInvoicePdf } from "@/api/queries";
import { EmptyState, LoadError, LoadingRows, SectionHeader, Unavailable } from "@/components/settings/SettingsShared";
import { PayAsYouGoCard } from "@/components/settings/PayAsYouGoCard";
import { PaymentMethod } from "@/components/settings/PaymentMethod";
import { getApiErrorMessage, type BillingInvoice } from "@/lib/api";
import { formatCreditAmount } from "@/lib/billing-balances";
import { formatShortDate } from "@/lib/date-utils";
import { StatusBadge, type StatusBadgeColor } from "@/components/StatusBadge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

function invoiceStatusColor(status?: string): StatusBadgeColor {
  switch ((status ?? "").toUpperCase()) {
    case "FINALIZED":
    case "PAID":
      return "success";
    case "VOID":
      return "error";
    default:
      return "warning";
  }
}

function invoicePeriod(inv: BillingInvoice): string {
  if (inv.start_timestamp && inv.end_timestamp) {
    return `${formatShortDate(inv.start_timestamp)} – ${formatShortDate(inv.end_timestamp)}`;
  }
  return formatShortDate(inv.issued_at) || inv.id || "Invoice";
}

// A draft invoice is the period still accruing, so it gets a date rather than
// the flat "no PDF" a voided one does.
function noPdfReason(inv: BillingInvoice): string {
  if ((inv.status ?? "").toUpperCase() !== "DRAFT") return "No PDF for this invoice.";
  const closes = formatShortDate(inv.end_timestamp);
  return closes
    ? `Available to download once this period closes on ${closes}.`
    : "Available to download once this period closes.";
}

// The handle isn't on a BillingInvoice, so the caller supplies it.
function invoiceFilename(account: string, inv: BillingInvoice): string {
  const date = inv.issued_at?.slice(0, 10) || inv.id || "invoice";
  return `${account}-invoice-${date}.pdf`;
}

/** The page owns its own anchors: a card that scrolled to an id would only
 *  work on the page that happens to define it. */
function scrollToSection(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
}

function Invoices({ account }: { account: string }) {
  const { data, isLoading, isLoadingError, refetch } = useBillingInvoices(account);
  const download = useDownloadInvoicePdf(account);
  const invoices = data?.data ?? [];
  // download.isPending is shared across every row; track in-flight ids in a
  // Set so two concurrent downloads keep independent disabled state.
  const [downloadingIds, setDownloadingIds] = useState<ReadonlySet<string>>(new Set());

  return (
    <div id="invoices" className="flex flex-col gap-3">
      <h3 className="text-heading-4 text-foreground">Invoices</h3>
      {isLoading ? (
        <LoadingRows rows={4} />
      ) : isLoadingError ? (
        <LoadError onRetry={() => refetch()} />
      ) : !data?.available ? (
        <Unavailable />
      ) : !invoices.length ? (
        <EmptyState message="No invoices yet." />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Period</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Total</TableHead>
              <TableHead className="w-[80px]" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {invoices.map((inv, i) => {
              // Only finalized invoices have a downloadable PDF; drafts 404.
              const hasPdf = (inv.status ?? "").toUpperCase() === "FINALIZED" && !!inv.id;
              return (
                <TableRow key={inv.id ?? i}>
                  <TableCell className="font-medium">{invoicePeriod(inv)}</TableCell>
                  <TableCell>
                    <StatusBadge color={invoiceStatusColor(inv.status)}>{inv.status ?? "—"}</StatusBadge>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatCreditAmount(inv.total, inv.credit_type?.name, inv.credit_type?.id)}
                  </TableCell>
                  <TableCell className="text-right">
                    {hasPdf ? (
                      <button
                        type="button"
                        aria-label="Download invoice"
                        disabled={downloadingIds.has(inv.id!)}
                        onClick={() => {
                          const id = inv.id!;
                          setDownloadingIds((prev) => new Set(prev).add(id));
                          download.mutate(
                            { invoiceId: id, filename: invoiceFilename(account, inv) },
                            {
                              onError: (err) =>
                                toast.error(getApiErrorMessage(err, "Couldn't download the invoice.")),
                              onSettled: () =>
                                setDownloadingIds((prev) => {
                                  const next = new Set(prev);
                                  next.delete(id);
                                  return next;
                                }),
                            },
                          );
                        }}
                        className="inline-flex items-center gap-1 text-body-sm text-foreground-accent hover:opacity-80 disabled:opacity-50"
                      >
                        <Download className="size-3.5" />
                        Download
                      </button>
                    ) : (
                      <TooltipProvider delayDuration={300}>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="inline-flex">
                              <button
                                type="button"
                                disabled
                                aria-label="Download invoice"
                                className="inline-flex cursor-not-allowed items-center gap-1 text-body-sm text-muted-foreground opacity-50"
                              >
                                <Download className="size-3.5" />
                                Download
                              </button>
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>{noPdfReason(inv)}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    )}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
    </div>
  );
}

export function BillingView({ account }: { account: string }) {
  return (
    <>
      <SectionHeader title="Billing" subtitle="Manage your payment and plan information." />
      <div className="flex flex-col gap-6">
        <PayAsYouGoCard
          account={account}
          onAddPayment={() => scrollToSection("payment-details")}
          onViewInvoices={() => scrollToSection("invoices")}
        />
        <PaymentMethod account={account} />
        <Invoices account={account} />
      </div>
    </>
  );
}
