import type { Meta, StoryObj } from '@storybook/react-vite'

const hues = [
  'stone',
  'neutral',
  'slate',
  'teal',
  'coral',
  'indigo',
  'red',
  'amber',
  'yellow',
  'green',
  'blue',
  'purple',
  'pink',
] as const

const shades = [25, 50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const

// Pre-computed class map so Tailwind can detect every utility at build time.
const bgClass: Record<string, Record<number, string>> = {
  indigo: {
    25: 'bg-indigo-25',
    50: 'bg-indigo-50',
    100: 'bg-indigo-100',
    200: 'bg-indigo-200',
    300: 'bg-indigo-300',
    400: 'bg-indigo-400',
    500: 'bg-indigo-500',
    600: 'bg-indigo-600',
    700: 'bg-indigo-700',
    800: 'bg-indigo-800',
    900: 'bg-indigo-900',
    950: 'bg-indigo-950',
  },
  'neutral': {
    25: 'bg-neutral-25',
    50: 'bg-neutral-50',
    100: 'bg-neutral-100',
    200: 'bg-neutral-200',
    300: 'bg-neutral-300',
    400: 'bg-neutral-400',
    500: 'bg-neutral-500',
    600: 'bg-neutral-600',
    700: 'bg-neutral-700',
    800: 'bg-neutral-800',
    900: 'bg-neutral-900',
    950: 'bg-neutral-950',
  },
  'slate': {
    25: 'bg-slate-25',
    50: 'bg-slate-50',
    100: 'bg-slate-100',
    200: 'bg-slate-200',
    300: 'bg-slate-300',
    400: 'bg-slate-400',
    500: 'bg-slate-500',
    600: 'bg-slate-600',
    700: 'bg-slate-700',
    800: 'bg-slate-800',
    900: 'bg-slate-900',
    950: 'bg-slate-950',
  },
  'stone': {
    25: 'bg-stone-25',
    50: 'bg-stone-50',
    100: 'bg-stone-100',
    200: 'bg-stone-200',
    300: 'bg-stone-300',
    400: 'bg-stone-400',
    500: 'bg-stone-500',
    600: 'bg-stone-600',
    700: 'bg-stone-700',
    800: 'bg-stone-800',
    900: 'bg-stone-900',
    950: 'bg-stone-950',
  },
  red: {
    25: 'bg-red-25',
    50: 'bg-red-50',
    100: 'bg-red-100',
    200: 'bg-red-200',
    300: 'bg-red-300',
    400: 'bg-red-400',
    500: 'bg-red-500',
    600: 'bg-red-600',
    700: 'bg-red-700',
    800: 'bg-red-800',
    900: 'bg-red-900',
    950: 'bg-red-950',
  },
  amber: {
    25: 'bg-amber-25',
    50: 'bg-amber-50',
    100: 'bg-amber-100',
    200: 'bg-amber-200',
    300: 'bg-amber-300',
    400: 'bg-amber-400',
    500: 'bg-amber-500',
    600: 'bg-amber-600',
    700: 'bg-amber-700',
    800: 'bg-amber-800',
    900: 'bg-amber-900',
    950: 'bg-amber-950',
  },
  yellow: {
    25: 'bg-yellow-25',
    50: 'bg-yellow-50',
    100: 'bg-yellow-100',
    200: 'bg-yellow-200',
    300: 'bg-yellow-300',
    400: 'bg-yellow-400',
    500: 'bg-yellow-500',
    600: 'bg-yellow-600',
    700: 'bg-yellow-700',
    800: 'bg-yellow-800',
    900: 'bg-yellow-900',
    950: 'bg-yellow-950',
  },
  green: {
    25: 'bg-green-25',
    50: 'bg-green-50',
    100: 'bg-green-100',
    200: 'bg-green-200',
    300: 'bg-green-300',
    400: 'bg-green-400',
    500: 'bg-green-500',
    600: 'bg-green-600',
    700: 'bg-green-700',
    800: 'bg-green-800',
    900: 'bg-green-900',
    950: 'bg-green-950',
  },
  teal: {
    25: 'bg-teal-25',
    50: 'bg-teal-50',
    100: 'bg-teal-100',
    200: 'bg-teal-200',
    300: 'bg-teal-300',
    400: 'bg-teal-400',
    500: 'bg-teal-500',
    600: 'bg-teal-600',
    700: 'bg-teal-700',
    800: 'bg-teal-800',
    900: 'bg-teal-900',
    950: 'bg-teal-950',
  },
  coral: {
    25: 'bg-coral-25',
    50: 'bg-coral-50',
    100: 'bg-coral-100',
    200: 'bg-coral-200',
    300: 'bg-coral-300',
    400: 'bg-coral-400',
    500: 'bg-coral-500',
    600: 'bg-coral-600',
    700: 'bg-coral-700',
    800: 'bg-coral-800',
    900: 'bg-coral-900',
    950: 'bg-coral-950',
  },
  blue: {
    25: 'bg-blue-25',
    50: 'bg-blue-50',
    100: 'bg-blue-100',
    200: 'bg-blue-200',
    300: 'bg-blue-300',
    400: 'bg-blue-400',
    500: 'bg-blue-500',
    600: 'bg-blue-600',
    700: 'bg-blue-700',
    800: 'bg-blue-800',
    900: 'bg-blue-900',
    950: 'bg-blue-950',
  },
  purple: {
    25: 'bg-purple-25',
    50: 'bg-purple-50',
    100: 'bg-purple-100',
    200: 'bg-purple-200',
    300: 'bg-purple-300',
    400: 'bg-purple-400',
    500: 'bg-purple-500',
    600: 'bg-purple-600',
    700: 'bg-purple-700',
    800: 'bg-purple-800',
    900: 'bg-purple-900',
    950: 'bg-purple-950',
  },
  pink: {
    25: 'bg-pink-25',
    50: 'bg-pink-50',
    100: 'bg-pink-100',
    200: 'bg-pink-200',
    300: 'bg-pink-300',
    400: 'bg-pink-400',
    500: 'bg-pink-500',
    600: 'bg-pink-600',
    700: 'bg-pink-700',
    800: 'bg-pink-800',
    900: 'bg-pink-900',
    950: 'bg-pink-950',
  },
}

function ColorGrid() {
  return (
    <div className="p-6 font-sans">
      <h2 className="mb-6 text-2xl font-semibold text-foreground">
        Color Palette
      </h2>
      <div
        className="inline-grid gap-1"
        style={{
          gridTemplateColumns: `100px repeat(${shades.length}, auto)`,
        }}
      >
        {/* Header row: empty cell + shade numbers */}
        <div />
        {shades.map((shade) => (
          <div
            key={shade}
            className="pb-2 text-center text-xs font-semibold text-foreground"
          >
            {shade}
          </div>
        ))}

        {/* Color rows */}
        {hues.map((hue) => (
          <>
            {/* Row label */}
            <div
              key={`${hue}-label`}
              className="flex items-center text-sm font-medium capitalize text-foreground"
            >
              {hue}
            </div>
            {/* Swatches */}
            {shades.map((shade) => (
              <div
                key={`${hue}-${shade}`}
                className={`size-20 rounded-md border border-black/8 ${bgClass[hue][shade]}`}
                title={`${hue}-${shade}`}
              />
            ))}
          </>
        ))}
      </div>
    </div>
  )
}

const meta = {
  title: 'Design System/Theme/Colors',
  component: ColorGrid,
  parameters: {
    layout: 'fullscreen',
  },
} satisfies Meta<typeof ColorGrid>

export default meta
type Story = StoryObj<typeof meta>

export const Palette: Story = {}

const semanticTokens = [
  {
    group: "Foreground",
    desc: "Text colors for content hierarchy.",
    tokens: [
      { name: "foreground", bg: "bg-foreground", text: "text-background", desc: "Default body text", light: "slate-950", dark: "slate-100" },
      { name: "muted-foreground", bg: "bg-muted-foreground", text: "text-white", desc: "Subdued text", light: "slate-600", dark: "slate-300" },
      { name: "faint-foreground", bg: "bg-faint-foreground", text: "text-white", desc: "Tertiary / disabled text", light: "slate-500", dark: "slate-400" },
      { name: "primary-foreground", bg: "bg-primary-foreground", text: "text-foreground-accent", desc: "Text on primary", light: "#fff", dark: "#fff" },
      { name: "foreground-accent", bg: "bg-foreground-accent", text: "text-white", desc: "Links / brand foreground", light: "indigo-600", dark: "indigo-400" },
      { name: "secondary-foreground", bg: "bg-secondary-foreground", text: "text-white", desc: "Text on secondary", light: "slate-800", dark: "slate-100" },
      { name: "accent-foreground", bg: "bg-accent-foreground", text: "text-white", desc: "Text on accent", light: "slate-800", dark: "slate-100" },
      { name: "popover-foreground", bg: "bg-popover-foreground", text: "text-white", desc: "Text in popovers", light: "foreground", dark: "slate-100" },
      { name: "destructive", bg: "bg-destructive", text: "text-white", desc: "Error / danger text", light: "red-700", dark: "red-400" },
      { name: "success", bg: "bg-success", text: "text-white", desc: "Success / positive text", light: "green-600", dark: "green-400" },
    ],
  },
  {
    group: "Surface & Background",
    desc: "Background fills for pages, panels, and overlays.",
    tokens: [
      { name: "background", bg: "bg-background", text: "text-foreground", desc: "Page background", light: "slate-50", dark: "slate-950" },
      { name: "surface", bg: "bg-surface", text: "text-foreground", desc: "Application shell", light: "slate-100", dark: "slate-950/900 mix 70/30" },
      { name: "card", bg: "bg-card", text: "text-foreground", desc: "Card / panel", light: "white", dark: "slate-950/900 mix 60/40" },
      { name: "muted", bg: "bg-muted", text: "text-muted-foreground", desc: "Subtle filled content", light: "slate-200", dark: "card alias" },
      { name: "popover", bg: "bg-popover", text: "text-foreground", desc: "Popover / dropdown overlay", light: "white", dark: "slate-950/900 35/65" },
      { name: "primary", bg: "bg-primary", text: "text-primary-foreground", desc: "Primary action fill", light: "indigo-700", dark: "indigo-600" },
      { name: "secondary", bg: "bg-secondary", text: "text-secondary-foreground", desc: "Secondary action fill", light: "slate-200", dark: "slate-800" },
      { name: "accent", bg: "bg-accent", text: "text-accent-foreground", desc: "Accent / hover fill", light: "white/slate-200 mix 45%", dark: "900/800 mix 40%" },
    ],
  },
  {
    group: "Border & Input",
    desc: "Stroke and input chrome tokens.",
    tokens: [
      { name: "border", bg: "bg-border", text: "text-foreground", desc: "Default border", light: "slate-300", dark: "slate-300 @ 15%" },
      { name: "border-strong", bg: "bg-border-strong", text: "text-white", desc: "Emphasized border", light: "slate-400", dark: "slate-300 @ 25%" },
      { name: "input", bg: "bg-input", text: "text-foreground", desc: "Input border", light: "slate-300", dark: "slate-300 @ 20%" },
      { name: "input-background", bg: "bg-[var(--input-background)]", text: "text-foreground", desc: "Input fill", light: "white", dark: "slate-950" },
      { name: "ring", bg: "bg-ring", text: "text-white", desc: "Focus ring", light: "slate-500", dark: "slate-500" },
      { name: "input-focus-ring", bg: "bg-[var(--input-focus-ring)]", text: "text-foreground", desc: "Input focus ring", light: "slate-600 @ 20%", dark: "slate-400 @ 15%" },
    ],
  },
  {
    group: "Misc",
    desc: "Specialty tokens.",
    tokens: [
      { name: "code-text", bg: "bg-code-text", text: "text-black", desc: "Inline code text color", light: "oklch(82.90% 0.0224 182.6)", dark: "teal-300" },
    ],
  },
]

function SemanticTokens() {
  return (
    <div className="flex flex-col gap-10 p-6 font-sans">
      {semanticTokens.map((group) => (
        <div key={group.group}>
          <h3 className="text-heading-3 text-foreground">{group.group}</h3>
          <p className="text-body-sm text-muted-foreground mt-1 mb-4">{group.desc}</p>
          <div className="grid grid-cols-[auto_1fr_1fr_1fr] gap-x-6 gap-y-3 items-center">
            <div className="text-label font-mono uppercase text-faint-foreground">Swatch</div>
            <div className="text-label font-mono uppercase text-faint-foreground">Token</div>
            <div className="text-label font-mono uppercase text-faint-foreground">Purpose</div>
            <div className="text-label font-mono uppercase text-faint-foreground">Value (light / dark)</div>
            {group.tokens.map((t) => (
              <>
                <div
                  key={`${t.name}-swatch`}
                  className={`h-9 w-20 rounded-md border border-black/8 ${t.bg}`}
                />
                <code key={`${t.name}-name`} className="text-mono-sm font-mono text-foreground">{t.name}</code>
                <span key={`${t.name}-desc`} className="text-body-sm text-muted-foreground">{t.desc}</span>
                <span key={`${t.name}-value`} className="text-mono-sm font-mono text-faint-foreground">{t.light} / {t.dark}</span>
              </>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

export const SemanticTokensStory: StoryObj = {
  name: "Semantic Tokens",
  render: () => <SemanticTokens />,
}
