import { Link } from "react-router";
import { Gauge, Star, UserCircle, Zap } from "lucide-react";

const DIAGRAM_SVG = `
<svg viewBox="0 0 720 340" fill="none" xmlns="http://www.w3.org/2000/svg" class="w-full">
  <!-- Background grid -->
  <defs>
    <pattern id="grid" width="20" height="20" patternUnits="userSpaceOnUse">
      <path d="M 20 0 L 0 0 0 20" fill="none" stroke="currentColor" stroke-opacity="0.04" stroke-width="0.5"/>
    </pattern>
    <marker id="arrow" viewBox="0 0 10 7" refX="10" refY="3.5" markerWidth="8" markerHeight="6" orient="auto-start-reverse">
      <polygon points="0 0, 10 3.5, 0 7" fill="#d97706"/>
    </marker>
    <linearGradient id="boxGrad" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#fef3c7" stop-opacity="0.6"/>
      <stop offset="100%" stop-color="#fde68a" stop-opacity="0.15"/>
    </linearGradient>
  </defs>
  <rect width="720" height="340" fill="url(#grid)"/>

  <!-- Events box -->
  <rect x="30" y="130" width="120" height="64" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="90" y="155" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Events</text>
  <text x="90" y="172" text-anchor="middle" fill="#a16207" font-size="8">CloudEvents ingested</text>
  <text x="90" y="183" text-anchor="middle" fill="#a16207" font-size="8">by your application</text>

  <!-- Arrow: Events -> Meters -->
  <line x1="150" y1="162" x2="218" y2="162" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="184" y="154" text-anchor="middle" fill="#a16207" font-size="7">ingest</text>

  <!-- Meters box -->
  <rect x="220" y="120" width="130" height="84" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="285" y="146" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Meters</text>
  <text x="285" y="161" text-anchor="middle" fill="#a16207" font-size="8">Aggregate events into</text>
  <text x="285" y="172" text-anchor="middle" fill="#a16207" font-size="8">measurable usage.</text>
  <text x="285" y="192" text-anchor="middle" fill="#a16207" font-size="7" font-style="italic">SUM, COUNT, AVG...</text>

  <!-- Arrow: Meters -> Features -->
  <line x1="350" y1="162" x2="418" y2="162" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="384" y="154" text-anchor="middle" fill="#a16207" font-size="7">powers</text>

  <!-- Features box -->
  <rect x="420" y="130" width="120" height="64" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="480" y="155" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Features</text>
  <text x="480" y="172" text-anchor="middle" fill="#a16207" font-size="8">Named capabilities</text>
  <text x="480" y="183" text-anchor="middle" fill="#a16207" font-size="8">backed by a meter</text>

  <!-- Arrow: Features -> Entitlements -->
  <line x1="540" y1="162" x2="568" y2="162" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="554" y="154" text-anchor="middle" fill="#a16207" font-size="7">grants</text>

  <!-- Entitlements box -->
  <rect x="570" y="110" width="130" height="84" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="635" y="136" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Entitlements</text>
  <text x="635" y="151" text-anchor="middle" fill="#a16207" font-size="8">Feature access rules</text>
  <text x="635" y="162" text-anchor="middle" fill="#a16207" font-size="8">per customer. Types:</text>
  <text x="635" y="182" text-anchor="middle" fill="#a16207" font-size="7" font-style="italic">metered | static | boolean</text>

  <!-- Customers box -->
  <rect x="570" y="230" width="130" height="64" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="635" y="255" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Customers</text>
  <text x="635" y="272" text-anchor="middle" fill="#a16207" font-size="8">End users who receive</text>
  <text x="635" y="283" text-anchor="middle" fill="#a16207" font-size="8">entitlements & grants</text>

  <!-- Arrow: Customers -> Entitlements -->
  <line x1="635" y1="230" x2="635" y2="196" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="648" y="215" text-anchor="start" fill="#a16207" font-size="7">has</text>

  <!-- Grants label on Entitlements -->
  <rect x="570" y="42" width="130" height="52" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5" stroke-dasharray="4 2"/>
  <text x="635" y="64" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Grants</text>
  <text x="635" y="79" text-anchor="middle" fill="#a16207" font-size="8">Usage credits added</text>
  <text x="635" y="90" text-anchor="middle" fill="#a16207" font-size="8">to an entitlement</text>

  <!-- Arrow: Grants -> Entitlements -->
  <line x1="635" y1="94" x2="635" y2="108" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
</svg>
`;

const STEPS = [
  {
    num: 1,
    title: "Create a Meter",
    desc: "Define how to aggregate events. Pick an event type, aggregation (SUM, COUNT, etc.), and optionally a value property and group-by dimensions.",
    link: "/openmeter/meters",
    icon: Gauge,
  },
  {
    num: 2,
    title: "Create a Feature",
    desc: "Name a capability and link it to a meter. Features are the building blocks of what you sell or gate.",
    link: "/openmeter/features",
    icon: Star,
  },
  {
    num: 3,
    title: "Add Customers",
    desc: "Register end-users or organizations. Each customer can receive entitlements for features.",
    link: "/openmeter/customers",
    icon: UserCircle,
  },
  {
    num: 4,
    title: "Ingest Events",
    desc: "Send CloudEvents from your app. Events flow into meters, which update usage balances in real time.",
    link: "/openmeter/events",
    icon: Zap,
  },
];

export function OpenMeterHomePage() {
  return (
    <div className="space-y-6 max-w-4xl">
      <div>
        <h2 className="text-xl font-semibold">OpenMeter</h2>
        <p className="text-[11px] text-muted-foreground mt-1">
          Usage-based metering, entitlements, and billing infrastructure. Track events, meter usage, gate features, and manage grants per customer.
        </p>
      </div>

      <div
        className="rounded-lg glass p-4"
        dangerouslySetInnerHTML={{ __html: DIAGRAM_SVG }}
      />

      <div>
        <h3 className="text-sm font-medium mb-3">Getting started</h3>
        <div className="grid grid-cols-2 gap-3">
          {STEPS.map((s) => (
            <Link
              key={s.num}
              to={s.link}
              className="group rounded-lg glass px-3 py-2.5 hover:bg-glass-light transition-colors"
            >
              <div className="flex items-center gap-2 mb-1">
                <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-pollen/80 text-[10px] font-bold text-honey-dark">
                  {s.num}
                </span>
                <s.icon className="size-3 text-amber" />
                <span className="text-[11px] font-semibold group-hover:text-amber transition-colors">{s.title}</span>
              </div>
              <p className="text-[10px] text-muted-foreground leading-relaxed">{s.desc}</p>
            </Link>
          ))}
        </div>
      </div>

      <div className="rounded-lg glass-subtle px-3 py-2">
        <h4 className="text-[11px] font-medium mb-1">How it all connects</h4>
        <ul className="text-[10px] text-muted-foreground space-y-0.5 list-disc list-inside">
          <li><strong className="text-foreground">Events</strong> are raw data points your app sends (CloudEvents format).</li>
          <li><strong className="text-foreground">Meters</strong> aggregate events into numeric usage values (e.g., total API calls).</li>
          <li><strong className="text-foreground">Features</strong> name what you're selling or gating, backed by a meter.</li>
          <li><strong className="text-foreground">Entitlements</strong> grant a customer access to a feature (metered, static, or boolean).</li>
          <li><strong className="text-foreground">Grants</strong> add usage credits to metered entitlements (e.g., 10,000 tokens/month).</li>
          <li><strong className="text-foreground">Customers</strong> are end-users who receive entitlements and consume grants.</li>
        </ul>
      </div>
    </div>
  );
}
