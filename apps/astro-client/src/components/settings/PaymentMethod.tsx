import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { CreditCard } from "lucide-react";
import { loadStripe, type Appearance, type Stripe } from "@stripe/stripe-js";
import {
  Elements,
  PaymentElement,
  useElements,
  useStripe,
} from "@stripe/react-stripe-js";
import { PaymentIcon } from "react-svg-credit-card-payment-icons";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { RemovePaymentMethodDialog } from "@/components/settings/RemovePaymentMethodDialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { Tag } from "@/components/Tag";
import { LoadError } from "@/components/settings/SettingsShared";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { type SavedCard } from "@/lib/api";
import { useResolvedTheme } from "@/lib/theme";
import { useAuth } from "@/lib/auth";
import {
  useConfirmPaymentMethod,
  useCreateSetupIntent,
  usePaymentMethod,
} from "@/api/queries/billing";

// resolveColor reads a theme CSS color (e.g. "var(--foreground)") and returns it
// resolved to rgb — our tokens are oklch and won't cross into the Stripe iframe.
function resolveColor(value: string): string {
  if (typeof document === "undefined") return "";
  const el = document.createElement("span");
  el.style.position = "absolute";
  el.style.color = value;
  document.body.appendChild(el);
  const resolved = getComputedStyle(el).color;
  el.remove();
  return resolved;
}

// buildStripeAppearance maps our semantic theme tokens onto Stripe Elements'
// appearance API so the embedded PaymentElement matches light/dark chrome.
function buildStripeAppearance(isDark: boolean): Appearance {
  return {
    theme: isDark ? "night" : "stripe",
    variables: {
      colorPrimary: resolveColor("var(--primary)"),
      colorBackground: resolveColor("var(--input-background)"),
      colorText: resolveColor("var(--foreground)"),
      colorTextSecondary: resolveColor("var(--muted-foreground)"),
      colorDanger: resolveColor("var(--destructive)"),
    },
  };
}

function formatBrand(brand: string): string {
  if (!brand) return "Card";
  return brand.charAt(0).toUpperCase() + brand.slice(1);
}

// Maps Stripe's card.brand values to the icon library's canonical type names.
const STRIPE_BRAND_TO_ICON = {
  visa: "Visa",
  mastercard: "Mastercard",
  amex: "AmericanExpress",
  discover: "Discover",
  diners: "DinersClub",
  jcb: "JCB",
  unionpay: "UnionPay",
} as const;

// CardBrandIcon renders the saved card's network logo (library-provided; Stripe
// only renders brand icons inside its own Elements, not for our summary row).
function CardBrandIcon({ brand }: { brand: string }) {
  const type =
    STRIPE_BRAND_TO_ICON[brand.toLowerCase() as keyof typeof STRIPE_BRAND_TO_ICON] ?? "Generic";
  return <PaymentIcon type={type} format="flat" className="h-8 w-auto shrink-0" />;
}

// CardForm renders Stripe's embedded PaymentElement (card + billing details) and
// confirms the SetupIntent. Must be a child of <Elements> so the hooks resolve.
function CardForm({ account, onDone }: { account: string; onDone: () => void }) {
  const stripe = useStripe();
  const elements = useElements();
  const confirm = useConfirmPaymentMethod(account);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    if (!stripe || !elements) return;
    setSubmitting(true);
    setError(null);

    // Confirm the SetupIntent client-side (runs SCA). redirect:if_required keeps
    // the user in-page for cards that don't need a redirect.
    const { error: confErr, setupIntent } = await stripe.confirmSetup({
      elements,
      redirect: "if_required",
    });
    if (confErr) {
      setError(confErr.message ?? "Card setup failed.");
      setSubmitting(false);
      return;
    }
    if (setupIntent?.status !== "succeeded") {
      setError("Card setup did not complete. Please try again.");
      setSubmitting(false);
      return;
    }

    // Save server-side: the server re-reads the SetupIntent from Stripe, stores
    // the card as default, and links it for billing.
    try {
      await confirm.mutateAsync(setupIntent.id);
      toast.success("Payment method saved");
      onDone();
    } catch {
      setError("Card confirmed but couldn't be saved. Please try again.");
    }
    setSubmitting(false);
  };

  return (
    <div className="space-y-4">
      <PaymentElement options={{ layout: "tabs" }} />
      {error && <p className="text-body-sm text-destructive">{error}</p>}
      <div className="flex justify-end pt-2">
        <Button size="sm" onClick={handleSubmit} disabled={submitting || !stripe}>
          {submitting ? "Saving…" : "Save card"}
        </Button>
      </div>
    </div>
  );
}

function AddCardDialog({
  account,
  open,
  onOpenChange,
}: {
  account: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [stripePromise, setStripePromise] = useState<Promise<Stripe | null> | null>(null);
  const [clientSecret, setClientSecret] = useState<string | null>(null);
  const isDark = useResolvedTheme() === "dark";
  const appearance = useMemo(() => buildStripeAppearance(isDark), [isDark]);
  const setupIntent = useCreateSetupIntent(account);
  const attempt = useRef(0);

  // `attempt` guards a slow response (initial open or retry) from landing
  // after a newer one, or after close. stripePromise and clientSecret are
  // set together here, not read separately off setupIntent.data, so a stale
  // response can't pair Elements with a mismatched secret.
  function startSetupIntent() {
    const mine = ++attempt.current;
    setupIntent.mutate(undefined, {
      onSuccess: (res) => {
        if (mine !== attempt.current || !res.publishable_key) return;
        setStripePromise(loadStripe(res.publishable_key));
        setClientSecret(res.client_secret);
      },
    });
  }

  useEffect(() => {
    if (!open) {
      // Bump attempt so a request already in flight when the dialog closes
      // fails its own mine !== attempt.current check instead of writing a
      // stale secret into state once it answers.
      attempt.current++;
      setStripePromise(null);
      setClientSecret(null);
      setupIntent.reset();
      return;
    }
    startSetupIntent();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, account]);

  const notConfigured = setupIntent.isSuccess && !setupIntent.data?.publishable_key;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add payment method</DialogTitle>
          <DialogDescription>
            Your card is stored securely by Stripe. It's charged automatically
            against your monthly usage.
          </DialogDescription>
        </DialogHeader>
        {setupIntent.isError ? (
          <div className="flex flex-col items-center gap-3 py-8">
            <p className="text-body-sm text-destructive">Couldn't start card setup.</p>
            <Button size="sm" variant="outline" disabled={setupIntent.isPending} onClick={startSetupIntent}>
              Retry
            </Button>
          </div>
        ) : notConfigured ? (
          <p className="py-4 text-body-sm text-destructive">Payments are not configured.</p>
        ) : clientSecret && stripePromise ? (
          <Elements stripe={stripePromise} options={{ clientSecret, appearance }}>
            <CardForm account={account} onDone={() => onOpenChange(false)} />
          </Elements>
        ) : (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[140px_1fr] items-center gap-3 border-t border-border/60 py-3 first:border-0">
      <span className="text-body-sm text-foreground">{label}</span>
      <span className="text-body-sm text-muted-foreground">{children}</span>
    </div>
  );
}

// Card presence decides the header buttons (Add vs Update/Remove); a shaped
// skeleton avoids flashing the no-card state before the query settles.
function PaymentMethodSkeleton() {
  return (
    <Card id="payment-details" className="flex flex-col gap-1 p-5">
      <div className="flex items-center justify-between gap-3 pb-2">
        <Skeleton className="h-5 w-32" />
        <Skeleton className="h-8 w-28" />
      </div>
      <Skeleton className="h-9 w-full" />
      <Skeleton className="h-9 w-full" />
      <Skeleton className="h-9 w-full" />
    </Card>
  );
}

// Payment method, billing cycle, and billing email in one card; only the
// card has its own write path (Stripe) — the other two are read-only facts.
export function PaymentMethod({ account }: { account: string }) {
  const { data, isLoading, isLoadingError, refetch } = usePaymentMethod(account);
  const { user } = useAuth();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);

  if (isLoading) return <PaymentMethodSkeleton />;
  if (isLoadingError) {
    return (
      <div id="payment-details">
        <LoadError onRetry={() => refetch()} />
      </div>
    );
  }

  // Payments aren't configured for this environment (no Stripe) — show a
  // "coming soon" placeholder instead of a working card form.
  const notAvailable = !!data && !data.available;
  if (notAvailable) {
    return (
      <Card id="payment-details" className="flex flex-wrap items-center justify-between gap-3 p-4">
        <div className="flex items-center gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md border border-border bg-surface">
            <CreditCard size={16} className="text-muted-foreground" />
          </div>
          <div className="flex flex-col">
            <div className="flex items-center gap-2">
              <span className="text-body-sm font-medium text-foreground">Payment details</span>
              <Tag color="blue">Coming soon</Tag>
            </div>
            <span className="text-body-sm text-muted-foreground">
              Adding a card isn't available yet.
            </span>
          </div>
        </div>
        <Button size="sm" variant="outline" disabled>
          Add credit card
        </Button>
      </Card>
    );
  }

  const card: SavedCard | undefined = data?.card;

  return (
    <>
      <Card id="payment-details" className="flex flex-col gap-1 p-5">
        <div className="flex items-center justify-between gap-3 pb-2">
          <h3 className="text-heading-4 text-foreground">Payment details</h3>
          <div className="flex items-center gap-2">
            {card && (
              <Button size="sm" variant="ghost" onClick={() => setRemoveOpen(true)}>
                Remove
              </Button>
            )}
            <Button size="sm" variant="outline" onClick={() => setDialogOpen(true)}>
              {card ? "Update" : "Add credit card"}
            </Button>
          </div>
        </div>

        <DetailRow label="Payment method">
          {card ? (
            <span className="flex items-center gap-2">
              <CardBrandIcon brand={card.brand} />
              {formatBrand(card.brand)} •••• {card.last4} · Expires{" "}
              {String(card.exp_month).padStart(2, "0")}/{String(card.exp_year).slice(-2)}
            </span>
          ) : (
            "No payment method on file"
          )}
        </DetailRow>
        <DetailRow label="Billing cycle">Monthly</DetailRow>
        <DetailRow label="Billing email">{user?.email ?? "—"}</DetailRow>
      </Card>

      <AddCardDialog account={account} open={dialogOpen} onOpenChange={setDialogOpen} />
      <RemovePaymentMethodDialog
        account={account}
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        onUpdateCard={() => setDialogOpen(true)}
      />
    </>
  );
}
