/** One fixed dark illustration in both themes, so its colors are literals
 *  rather than tokens, and the gradients stay in `style={}` because Tailwind
 *  arbitrary values do not parse the commas inside them. */
import { useEffect, useState, type CSSProperties } from "react";
import { Dialog as DialogPrimitive } from "radix-ui";
import { ArrowRight, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { useIsMobile } from "@/hooks/use-compact-layout";
import { usePrefersReducedMotion } from "@/hooks/use-prefers-reduced-motion";
import { StarField } from "@/components/agent-detail/starfield/StarField";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";

// The card transitions in, then the balance waits, rolls, and pops.
const CARD_ENTER_DELAY_MS = 30;
const BALANCE_REVEAL_DELAY_MS = 500;
const BALANCE_ROLL_MS = 850;
const BALANCE_ROLL_TICK_MS = 60;
const BALANCE_POP_MS = 900;

const formatCredits = (value: number) => `$${value.toFixed(2)}`;

export interface FreeTrialModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Dollar amount of the grant, e.g. 20 for "$20.00". */
  credits: number;
  ctaLabel?: string;
  onCta: () => void;
}

export function FreeTrialModal({
  open,
  onOpenChange,
  credits,
  ctaLabel = "Deploy an agent",
  onCta,
}: FreeTrialModalProps) {
  const [entered, setEntered] = useState(false);
  // Bottom sheet below the mobile breakpoint, as SidePanel does.
  const isMobile = useIsMobile();

  useEffect(() => {
    if (!open) {
      setEntered(false);
      return;
    }
    const t = setTimeout(() => setEntered(true), CARD_ENTER_DELAY_MS);
    return () => clearTimeout(t);
  }, [open]);

  if (!open) return null;

  const card = (
    <FreeTrialCard
      credits={credits}
      ctaLabel={ctaLabel}
      onCta={onCta}
      entered={entered}
      compact={isMobile}
    />
  );

  if (isMobile) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          side="bottom"
          showCloseButton={false}
          className="h-auto max-h-[calc(100dvh-1.5rem)] gap-0 overflow-hidden border-0 bg-transparent p-0 shadow-none"
        >
          <SheetTitle className="sr-only">Free credits on us</SheetTitle>
          {card}
        </SheetContent>
      </Sheet>
    );
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className="fixed inset-0 z-50 backdrop-blur-md"
          style={{
            background:
              "radial-gradient(120% 90% at 50% 45%, rgba(6,10,18,.4), rgba(6,10,18,.72))",
            animation: "ftv4-scrim .5s ease-out both",
          }}
        />
        <DialogPrimitive.Content
          className="fixed inset-0 z-50 flex items-center justify-center p-7 outline-none"
          aria-describedby={undefined}
        >
          <DialogPrimitive.Title className="sr-only">Free credits on us</DialogPrimitive.Title>
          {card}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

// Transparent so the card's own gradient shows through the field.
function CardBackdrop() {
  return (
    <div aria-hidden className="pointer-events-none absolute inset-0 z-0">
      <StarField
        direction="right"
        backgroundColor="transparent"
        starColor="#dfe8f6"
        starDensity={1.3}
        speed={0.5}
        seed={95522}
      />
      <div
        className="absolute bottom-[-16%] left-1/2 h-[46%] w-[170%] -translate-x-1/2 rounded-full blur-2xl"
        style={{
          background:
            "radial-gradient(ellipse at center bottom,rgba(150,183,210,.5),rgba(120,158,190,.2) 42%,transparent 72%)",
        }}
      />
      <div
        className="absolute top-[14%] left-[54%] h-[1.4px] w-[100px] rounded-sm"
        style={{
          background: "linear-gradient(90deg,rgba(255,255,255,0),rgba(255,255,255,.9))",
          boxShadow: "0 0 6px rgba(255,255,255,.6)",
          // Avoids a flash of the un-animated base style during the start delay.
          opacity: 0,
          animation: "ftv4-shoot 9s ease-in 2.4s infinite",
        }}
      />
    </div>
  );
}

function FreeTrialCard({
  credits,
  ctaLabel,
  onCta,
  entered,
  compact,
}: {
  credits: number;
  ctaLabel: string;
  onCta: () => void;
  entered: boolean;
  compact: boolean;
}) {
  return (
    <section
      className={cn(
        "relative overflow-hidden border border-t-0 text-center text-white dark:text-white",
        compact
          ? "w-full rounded-t-[24px]"
          : cn(
              "w-[min(92vw,540px)] rounded-[24px] shadow-[0_40px_100px_-30px_rgba(0,0,0,.75)] transition-[opacity,transform] duration-500 ease-out",
              entered ? "opacity-100 translate-y-0 scale-100" : "opacity-0 translate-y-3 scale-[.98]",
            ),
      )}
      style={{
        borderColor: "rgba(255,255,255,.14)",
        background: "linear-gradient(to bottom,#070b14 0%,#0b1524 40%,#1b3247 74%,#41627e 100%)",
      }}
    >
      <CardBackdrop />

      <DialogPrimitive.Close
        aria-label="Close"
        className="absolute top-4 right-4 z-[3] inline-flex size-8 items-center justify-center rounded-full text-white/70 outline-none transition-colors hover:text-white focus-visible:text-white dark:text-white/70 dark:hover:text-white dark:focus-visible:text-white"
      >
        <X size={15} />
      </DialogPrimitive.Close>

      <div
        className={cn(
          "relative z-[2] flex flex-col items-center",
          compact ? "px-6 pt-10 pb-8" : "px-10 pt-14 pb-12",
        )}
      >
        {/* aria-hidden: the dialog title already announces this wording. */}
        <span
          aria-hidden
          className="inline-flex items-center rounded-full px-3.5 py-1.5 font-mono text-[11px] font-semibold leading-none tracking-[.16em] text-white/80 dark:text-white/80"
          style={{
            background: "rgba(255,255,255,.08)",
            border: "1px solid rgba(255,255,255,.14)",
          }}
        >
          Free credits on us
        </span>

        <BalanceReveal credits={credits} compact={compact} />

        <div className={cn("flex w-full flex-col items-center", compact ? "mt-8" : "mt-11")}>
          <button
            type="button"
            onClick={onCta}
            className="inline-flex h-[50px] items-center gap-2 rounded-md bg-indigo-600 px-7 font-semibold text-[15px] text-white shadow-[0_10px_30px_-8px_rgba(0,0,0,.55)] transition-all hover:-translate-y-px hover:bg-indigo-500 dark:bg-indigo-600 dark:hover:bg-indigo-500 dark:text-white"
          >
            {ctaLabel}
            <ArrowRight size={17} />
          </button>
          <p className="mt-5 text-[12.5px] font-medium text-white/60 dark:text-white/60">
            Yours to use across every agent. No card needed.
          </p>
        </div>
      </div>
    </section>
  );
}

// Inline: Tailwind's arbitrary-value parser splits on the commas in these.
function popStyle(popped: boolean): CSSProperties {
  return {
    animation: popped ? "ftv4-pop .6s cubic-bezier(.22,1.4,.4,1) both" : undefined,
    textShadow: popped
      ? "0 0 24px color-mix(in srgb, var(--primary) 70%, transparent), 0 0 48px color-mix(in srgb, var(--primary) 40%, transparent)"
      : "0 0 0 transparent",
    transition: "text-shadow .5s ease",
  };
}

/** Rolls through random values before settling on the real amount. Snaps
 *  straight to target under prefers-reduced-motion. */
function BalanceReveal({ credits, compact = false }: { credits: number; compact?: boolean }) {
  const reduceMotion = usePrefersReducedMotion();
  const [display, setDisplay] = useState(() =>
    reduceMotion ? formatCredits(credits) : formatCredits(0),
  );
  const [popped, setPopped] = useState(false);

  useEffect(() => {
    if (reduceMotion) {
      setDisplay(formatCredits(credits));
      return;
    }
    let rollTimer: ReturnType<typeof setInterval> | undefined;
    let popTimer: ReturnType<typeof setTimeout> | undefined;
    const revealTimer = setTimeout(() => {
      const startedAt = performance.now();
      const step = () => {
        if (performance.now() - startedAt >= BALANCE_ROLL_MS) {
          if (rollTimer) clearInterval(rollTimer);
          setDisplay(formatCredits(credits));
          setPopped(true);
          popTimer = setTimeout(() => setPopped(false), BALANCE_POP_MS);
          return;
        }
        setDisplay(formatCredits(Math.random() * Math.max(2, credits)));
      };
      step();
      rollTimer = setInterval(step, BALANCE_ROLL_TICK_MS);
    }, BALANCE_REVEAL_DELAY_MS);
    return () => {
      clearTimeout(revealTimer);
      if (rollTimer) clearInterval(rollTimer);
      if (popTimer) clearTimeout(popTimer);
    };
    // credits is fixed for one modal open; re-running this on every render
    // would restart the roll-up mid-flight.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reduceMotion]);

  return (
    <span
      className={cn(
        "block w-full whitespace-nowrap text-center font-mono font-medium leading-none tabular-nums",
        // Smaller on mobile so a 3-digit grant doesn't clip in the sheet.
        compact ? "text-[44px]" : "text-[66px]",
        compact ? "mt-8" : "mt-11",
      )}
      style={popStyle(popped)}
    >
      {display}
    </span>
  );
}
