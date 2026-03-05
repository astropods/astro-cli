import { Link } from "react-router";
import { Gauge, Star, UserCircle, Zap, ClipboardList } from "lucide-react";

const DIAGRAM_SVG = `
<svg viewBox="0 0 820 400" fill="none" xmlns="http://www.w3.org/2000/svg" class="w-full">
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
    <linearGradient id="planGrad" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="#dbeafe" stop-opacity="0.6"/>
      <stop offset="100%" stop-color="#bfdbfe" stop-opacity="0.15"/>
    </linearGradient>
  </defs>
  <rect width="820" height="400" fill="url(#grid)"/>

  <!-- ===== TOP ROW: Manual / Data flow ===== -->

  <!-- Events box -->
  <rect x="30" y="50" width="120" height="64" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="90" y="75" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Events</text>
  <text x="90" y="92" text-anchor="middle" fill="#a16207" font-size="8">CloudEvents ingested</text>
  <text x="90" y="103" text-anchor="middle" fill="#a16207" font-size="8">by your application</text>

  <!-- Arrow: Events -> Meters -->
  <line x1="150" y1="82" x2="218" y2="82" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="184" y="74" text-anchor="middle" fill="#a16207" font-size="7">ingest</text>

  <!-- Meters box -->
  <rect x="220" y="40" width="130" height="84" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="285" y="66" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Meters</text>
  <text x="285" y="81" text-anchor="middle" fill="#a16207" font-size="8">Aggregate events into</text>
  <text x="285" y="92" text-anchor="middle" fill="#a16207" font-size="8">measurable usage.</text>
  <text x="285" y="112" text-anchor="middle" fill="#a16207" font-size="7" font-style="italic">SUM, COUNT, AVG...</text>

  <!-- Arrow: Meters -> Features -->
  <line x1="350" y1="82" x2="418" y2="82" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="384" y="74" text-anchor="middle" fill="#a16207" font-size="7">powers</text>

  <!-- Features box -->
  <rect x="420" y="50" width="120" height="64" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="480" y="75" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Features</text>
  <text x="480" y="92" text-anchor="middle" fill="#a16207" font-size="8">Named capabilities</text>
  <text x="480" y="103" text-anchor="middle" fill="#a16207" font-size="8">backed by a meter</text>

  <!-- Arrow: Features -> Entitlements -->
  <line x1="540" y1="82" x2="588" y2="82" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>

  <!-- Entitlements box -->
  <rect x="590" y="40" width="130" height="84" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="655" y="66" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Entitlements</text>
  <text x="655" y="81" text-anchor="middle" fill="#a16207" font-size="8">Feature access rules</text>
  <text x="655" y="92" text-anchor="middle" fill="#a16207" font-size="8">per customer. Types:</text>
  <text x="655" y="112" text-anchor="middle" fill="#a16207" font-size="7" font-style="italic">metered | static | boolean</text>

  <!-- Grants box (dashed, above entitlements) -->
  <rect x="735" y="30" width="70" height="52" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5" stroke-dasharray="4 2"/>
  <text x="770" y="52" text-anchor="middle" fill="#92400e" font-size="10" font-weight="600">Grants</text>
  <text x="770" y="66" text-anchor="middle" fill="#a16207" font-size="7">Usage credits</text>
  <text x="770" y="75" text-anchor="middle" fill="#a16207" font-size="7">on entitlements</text>

  <!-- Arrow: Grants -> Entitlements -->
  <line x1="735" y1="60" x2="722" y2="66" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>

  <!-- ===== BOTTOM ROW: Automation flow ===== -->

  <!-- Divider label -->
  <text x="410" y="170" text-anchor="middle" fill="#a16207" font-size="9" font-weight="600" opacity="0.6">— Automation layer —</text>

  <!-- Plans box (blue tint) -->
  <rect x="120" y="200" width="160" height="84" rx="8" fill="url(#planGrad)" stroke="#3b82f6" stroke-width="1.5"/>
  <text x="200" y="226" text-anchor="middle" fill="#1e40af" font-size="11" font-weight="600">Plans</text>
  <text x="200" y="241" text-anchor="middle" fill="#2563eb" font-size="8">Reusable pricing templates</text>
  <text x="200" y="252" text-anchor="middle" fill="#2563eb" font-size="8">with phases & rate cards.</text>
  <text x="200" y="272" text-anchor="middle" fill="#2563eb" font-size="7" font-style="italic">Define features, limits, billing</text>

  <!-- Arrow: Plans -> Subscriptions -->
  <line x1="280" y1="242" x2="348" y2="242" stroke="#3b82f6" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="314" y="234" text-anchor="middle" fill="#2563eb" font-size="7">assigned via</text>

  <!-- Subscriptions box (blue tint) -->
  <rect x="350" y="200" width="160" height="84" rx="8" fill="url(#planGrad)" stroke="#3b82f6" stroke-width="1.5"/>
  <text x="430" y="226" text-anchor="middle" fill="#1e40af" font-size="11" font-weight="600">Subscriptions</text>
  <text x="430" y="241" text-anchor="middle" fill="#2563eb" font-size="8">Assign a plan to a customer.</text>
  <text x="430" y="252" text-anchor="middle" fill="#2563eb" font-size="8">Auto-provisions entitlements</text>
  <text x="430" y="263" text-anchor="middle" fill="#2563eb" font-size="8">and grants from rate cards.</text>

  <!-- Arrow: Subscriptions -> Entitlements (up) -->
  <path d="M 510 230 L 560 230 L 620 126" stroke="#3b82f6" stroke-width="1.5" fill="none" marker-end="url(#arrow)"/>
  <text x="580" y="186" text-anchor="middle" fill="#2563eb" font-size="7">auto-creates</text>

  <!-- Customers box -->
  <rect x="570" y="210" width="130" height="64" rx="8" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1.5"/>
  <text x="635" y="235" text-anchor="middle" fill="#92400e" font-size="11" font-weight="600">Customers</text>
  <text x="635" y="252" text-anchor="middle" fill="#a16207" font-size="8">End users who subscribe</text>
  <text x="635" y="263" text-anchor="middle" fill="#a16207" font-size="8">to plans or get entitlements</text>

  <!-- Arrow: Customers -> Subscriptions -->
  <line x1="570" y1="246" x2="512" y2="246" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="541" y="258" text-anchor="middle" fill="#a16207" font-size="7">subscribes</text>

  <!-- Arrow: Customers -> Entitlements (up) -->
  <line x1="655" y1="210" x2="655" y2="126" stroke="#d97706" stroke-width="1.5" marker-end="url(#arrow)"/>
  <text x="668" y="170" text-anchor="start" fill="#a16207" font-size="7">has</text>

  <!-- Legend -->
  <rect x="30" y="330" width="12" height="8" rx="2" fill="url(#boxGrad)" stroke="#d97706" stroke-width="1"/>
  <text x="48" y="337" fill="#a16207" font-size="7">Core resources</text>
  <rect x="130" y="330" width="12" height="8" rx="2" fill="url(#planGrad)" stroke="#3b82f6" stroke-width="1"/>
  <text x="148" y="337" fill="#2563eb" font-size="7">Automation (Plans & Subscriptions)</text>
</svg>
`;

const STEPS = [
  {
    num: 1,
    title: "Create Meters",
    desc: "Define how to aggregate events. Pick an event type, aggregation (SUM, COUNT, etc.), and optionally a value property and group-by dimensions.",
    link: "/openmeter/meters",
    icon: Gauge,
  },
  {
    num: 2,
    title: "Create Features",
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
    title: "Create a Plan",
    desc: "Bundle features into a reusable pricing template with phases and rate cards. Define entitlement types, usage limits, and billing cadence.",
    link: "/openmeter/plans",
    icon: ClipboardList,
  },
  {
    num: 5,
    title: "Subscribe Customers",
    desc: "Open a customer and subscribe them to a plan. Entitlements and grants are auto-provisioned from the plan's rate cards.",
    link: "/openmeter/customers",
    icon: UserCircle,
  },
  {
    num: 6,
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
          Usage-based metering, entitlements, and billing infrastructure. Track events, meter usage, gate features, and manage grants per customer — manually or via plans and subscriptions.
        </p>
      </div>

      <div
        className="rounded-lg glass p-4"
        dangerouslySetInnerHTML={{ __html: DIAGRAM_SVG }}
      />

      <div>
        <h3 className="text-sm font-medium mb-3">Getting started</h3>
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
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
          <li><strong className="text-foreground">Plans</strong> bundle features into reusable pricing templates with phases and rate cards.</li>
          <li><strong className="text-foreground">Subscriptions</strong> assign a plan to a customer, auto-provisioning entitlements and grants.</li>
          <li><strong className="text-foreground">Entitlements</strong> grant a customer access to a feature (metered, static, or boolean).</li>
          <li><strong className="text-foreground">Grants</strong> add usage credits to metered entitlements (e.g., 10,000 tokens/month).</li>
          <li><strong className="text-foreground">Customers</strong> are end-users who subscribe to plans and consume usage.</li>
        </ul>
      </div>

      <div className="rounded-lg glass-subtle px-3 py-2">
        <h4 className="text-[11px] font-medium mb-1">Two ways to set up access</h4>
        <div className="grid grid-cols-2 gap-4 mt-1.5">
          <div>
            <p className="text-[10px] font-medium text-blue-500 mb-0.5">Automated (recommended)</p>
            <p className="text-[10px] text-muted-foreground leading-relaxed">
              Create a Plan with rate cards, then subscribe customers. Entitlements, grants, and billing are managed automatically.
            </p>
          </div>
          <div>
            <p className="text-[10px] font-medium text-amber mb-0.5">Manual</p>
            <p className="text-[10px] text-muted-foreground leading-relaxed">
              Create entitlements and grants directly on a customer. Useful for one-off overrides or custom deals.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
