import { useState } from "react";
import { ArrowUpRight, Download } from "lucide-react";
import { toast } from "sonner";
import { useBillingInvoices, useDownloadInvoicePdf } from "@/api/queries";
import { useWatchInvoicePayments } from "@/api/queries/billing";
import { EmptyState, LoadError, LoadingRows, SectionHeader, Unavailable } from "@/components/settings/SettingsShared";
import { Card } from "@/components/ui/card";
import { PayAsYouGoCard } from "@/components/settings/PayAsYouGoCard";
import { PaymentMethod } from "@/components/settings/PaymentMethod";
import { getApiErrorMessage, type BillingInvoice } from "@/lib/api";
import {
  EXTERNAL_DELETED,
  EXTERNAL_PAID,
  EXTERNAL_PARTIALLY_PAID,
  EXTERNAL_PAYMENT_FAILED,
  EXTERNAL_UNCOLLECTIBLE,
  EXTERNAL_VOID,
  INVOICE_DRAFT,
  INVOICE_FINALIZED,
  INVOICE_PAID,
  INVOICE_VOID,
  normalizeStatus,
} from "@/lib/billing-provider";
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

type InvoiceStatus = { label: string; color: StatusBadgeColor };

const STATUS_PAID: InvoiceStatus = { label: "Paid", color: "success" };
const STATUS_VOID: InvoiceStatus = { label: "Void", color: "error" };
const STATUS_PENDING: InvoiceStatus = { label: "Pending", color: "warning" };
// Closed, with no outcome reported yet. Neither good news nor bad, so muted.
const STATUS_ISSUED: InvoiceStatus = { label: "Issued", color: "muted" };

const EXTERNAL_STATUS: Record<string, InvoiceStatus> = {
  [EXTERNAL_PAID]: STATUS_PAID,
  [EXTERNAL_PARTIALLY_PAID]: { label: "Partially paid", color: "warning" },
  [EXTERNAL_PAYMENT_FAILED]: { label: "Payment failed", color: "error" },
  [EXTERNAL_UNCOLLECTIBLE]: { label: "Uncollectible", color: "error" },
  [EXTERNAL_VOID]: STATUS_VOID,
  [EXTERNAL_DELETED]: STATUS_VOID,
};

// Metronome's status says whether it closed the invoice, not whether anyone
// paid it. Only the external invoice reports that.
function invoiceStatus(inv: BillingInvoice): InvoiceStatus {
  switch (normalizeStatus(inv.status)) {
    case INVOICE_PAID:
      return STATUS_PAID;
    case INVOICE_FINALIZED:
      return EXTERNAL_STATUS[normalizeStatus(inv.external_invoice?.external_status)] ?? STATUS_ISSUED;
    case INVOICE_VOID:
      return STATUS_VOID;
    case INVOICE_DRAFT:
      return STATUS_PENDING;
    default:
      return { label: inv.status ?? "—", color: "muted" };
  }
}

// Only finalized invoices have a downloadable PDF; drafts 404.
function hasDownloadablePdf(inv: BillingInvoice): boolean {
  return normalizeStatus(inv.status) === INVOICE_FINALIZED && !!inv.id;
}

// A draft has not been issued, so it falls back to the day its period closes.
function invoiceDate(inv: BillingInvoice): string {
  return formatShortDate(inv.issued_at) || formatShortDate(inv.end_timestamp) || "—";
}

// Metronome issues no invoice number, so the id stands in.
function invoiceNumber(inv: BillingInvoice): string {
  if (!inv.id) return "—";
  return inv.id.replace(/-/g, "").slice(0, 8).toUpperCase();
}

// A draft invoice is the period still accruing, so it gets a date rather than
// the flat "no PDF" a voided one does.
function noPdfReason(inv: BillingInvoice): string {
  if (normalizeStatus(inv.status) !== INVOICE_DRAFT) return "No PDF for this invoice.";
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

const DOWNLOAD_BUTTON_CLASS =
  "inline-flex size-7 items-center justify-center rounded text-foreground-accent " +
  "hover:bg-muted disabled:cursor-not-allowed disabled:text-muted-foreground " +
  "disabled:opacity-50 disabled:hover:bg-transparent";

/** Icon-only, so the tooltip names it and covers the enabled case too. */
function InvoiceDownloadButton({
  invoice,
  disabled,
  onDownload,
}: {
  invoice: BillingInvoice;
  disabled: boolean;
  onDownload: () => void;
}) {
  const downloadable = hasDownloadablePdf(invoice);
  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex">
            <button
              type="button"
              aria-label="Download invoice"
              disabled={!downloadable || disabled}
              onClick={onDownload}
              className={DOWNLOAD_BUTTON_CLASS}
            >
              <Download className="size-3.5" />
            </button>
          </span>
        </TooltipTrigger>
        <TooltipContent>
          {downloadable ? "Download invoice" : noPdfReason(invoice)}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function InvoiceRow({
  invoice,
  downloading,
  onDownload,
}: {
  invoice: BillingInvoice;
  downloading: boolean;
  onDownload: () => void;
}) {
  const status = invoiceStatus(invoice);
  return (
    <TableRow>
      <TableCell className="font-medium">{invoiceDate(invoice)}</TableCell>
      <TableCell className="font-mono text-body-sm text-muted-foreground" title={invoice.id}>
        {invoiceNumber(invoice)}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {formatCreditAmount(invoice.total, invoice.credit_type?.name, invoice.credit_type?.id)}
      </TableCell>
      <TableCell>
        <StatusBadge color={status.color} size="sm">
          {status.label}
        </StatusBadge>
      </TableCell>
      <TableCell className="w-12">
        <InvoiceDownloadButton invoice={invoice} disabled={downloading} onDownload={onDownload} />
      </TableCell>
    </TableRow>
  );
}

function InvoicesTable({
  invoices,
  downloadingIds,
  onDownload,
}: {
  invoices: BillingInvoice[];
  downloadingIds: ReadonlySet<string>;
  onDownload: (invoice: BillingInvoice) => void;
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Date</TableHead>
          <TableHead>Invoice number</TableHead>
          <TableHead className="text-right">Amount</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="w-12" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {invoices.map((invoice, i) => (
          <InvoiceRow
            key={invoice.id ?? i}
            invoice={invoice}
            downloading={!!invoice.id && downloadingIds.has(invoice.id)}
            onDownload={() => onDownload(invoice)}
          />
        ))}
      </TableBody>
    </Table>
  );
}

// download.isPending is shared across every row, so in-flight ids are tracked
// in a Set instead: two concurrent downloads keep independent disabled state.
function useInvoiceDownload(account: string) {
  const download = useDownloadInvoicePdf(account);
  const [downloadingIds, setDownloadingIds] = useState<ReadonlySet<string>>(new Set());

  function start(invoice: BillingInvoice) {
    const id = invoice.id;
    if (!id) return;
    setDownloadingIds((prev) => new Set(prev).add(id));
    download.mutate(
      { invoiceId: id, filename: invoiceFilename(account, invoice) },
      {
        onError: (err) => toast.error(getApiErrorMessage(err, "Couldn't download the invoice.")),
        onSettled: () =>
          setDownloadingIds((prev) => {
            const next = new Set(prev);
            next.delete(id);
            return next;
          }),
      },
    );
  }

  return { downloadingIds, start };
}

function InvoicesBody({ account }: { account: string }) {
  const { data, isLoading, isLoadingError, refetch } = useBillingInvoices(account);
  const { downloadingIds, start } = useInvoiceDownload(account);

  if (isLoading) return <LoadingRows rows={4} />;
  if (isLoadingError) return <LoadError onRetry={refetch} />;
  if (!data?.available) return <Unavailable />;

  const invoices = data.data ?? [];
  if (!invoices.length) return <EmptyState message="No invoices yet." />;

  return (
    <InvoicesTable invoices={invoices} downloadingIds={downloadingIds} onDownload={start} />
  );
}

function Invoices({ account }: { account: string }) {
  useWatchInvoicePayments(account);
  return (
    <Card id="invoices" className="flex flex-col overflow-hidden">
      <div className="border-b border-border/60 px-5 py-4">
        <h3 className="text-heading-4 text-foreground">Invoices</h3>
      </div>
      <div className="px-5 py-4">
        <InvoicesBody account={account} />
      </div>
    </Card>
  );
}

export function BillingView({ account }: { account: string }) {
  return (
    <>
      <SectionHeader
        title="Billing"
        subtitle="Manage your payment and plan information."
        action={
          <a
            href="https://docs.astropods.com/usage-limits"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-body-sm text-foreground-accent hover:opacity-80"
          >
            Open docs
            <ArrowUpRight className="size-3.5" aria-hidden />
          </a>
        }
      />
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
