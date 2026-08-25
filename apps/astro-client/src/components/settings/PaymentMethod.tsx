import { useEffect, useMemo, useState } from "react";
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
import { Spinner } from "@/components/ui/spinner";
import { Tag } from "@/components/Tag";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { api, type SavedCard } from "@/lib/api";
import { useResolvedTheme } from "@/lib/theme";
import {
  useConfirmPaymentMethod,
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
  const [clientSecret, setClientSecret] = useState<string | null>(null);
  const [stripePromise, setStripePromise] = useState<Promise<Stripe | null> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const isDark = useResolvedTheme() === "dark";
  const appearance = useMemo(() => buildStripeAppearance(isDark), [isDark]);

  // On open, start a SetupIntent and load Stripe.js with the returned key.
  useEffect(() => {
    if (!open) {
      setClientSecret(null);
      setStripePromise(null);
      setError(null);
      return;
    }
    let cancelled = false;
    api
      .createSetupIntent(account)
      .then((res) => {
        if (cancelled) return;
        if (!res.publishable_key) {
          setError("Payments are not configured.");
          return;
        }
        setStripePromise(loadStripe(res.publishable_key));
        setClientSecret(res.client_secret);
      })
      .catch(() => {
        if (!cancelled) setError("Couldn't start card setup. Please try again.");
      });
    return () => {
      cancelled = true;
    };
  }, [open, account]);

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
        {error ? (
          <p className="py-4 text-body-sm text-destructive">{error}</p>
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

export function PaymentMethod({ account }: { account: string }) {
  const { data, isLoading } = usePaymentMethod(account);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);

  // Payments aren't configured for this environment (no Stripe) — show a
  // "coming soon" placeholder instead of a working card form.
  const notAvailable = !isLoading && !!data && !data.available;
  if (notAvailable) {
    return (
      <Card className="mb-6 flex flex-wrap items-center justify-between gap-3 p-4">
        <div className="flex items-center gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md border border-border bg-surface">
            <CreditCard size={16} className="text-muted-foreground" />
          </div>
          <div className="flex flex-col">
            <div className="flex items-center gap-2">
              <span className="text-body-sm font-medium text-foreground">Payment method</span>
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
      <Card className="mb-6 flex flex-wrap items-center justify-between gap-3 p-4">
        <div className="flex items-center gap-3">
          {card ? (
            <CardBrandIcon brand={card.brand} />
          ) : (
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md border border-border bg-surface">
              <CreditCard size={16} className="text-muted-foreground" />
            </div>
          )}
          <div className="flex flex-col">
            <span className="text-body-sm font-medium text-foreground">Payment method</span>
            {isLoading ? (
              <span className="text-body-sm text-muted-foreground">Loading…</span>
            ) : card ? (
              <span className="text-body-sm text-muted-foreground">
                {formatBrand(card.brand)} •••• {card.last4} · Expires{" "}
                {String(card.exp_month).padStart(2, "0")}/{String(card.exp_year).slice(-2)}
              </span>
            ) : (
              <span className="text-body-sm text-muted-foreground">No payment method on file</span>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {card && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setRemoveOpen(true)}
            >
              Remove
            </Button>
          )}
          <Button size="sm" variant="outline" onClick={() => setDialogOpen(true)}>
            {card ? "Update card" : "Add credit card"}
          </Button>
        </div>
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
