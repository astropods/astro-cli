import { useState, useEffect } from "react";
import { paletteNames } from "../src/colors";
import { lightTheme, darkTheme } from "../src/semantic";

const steps = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const;

const semanticGroups: { label: string; tokens: string[] }[] = [
  {
    label: "Core",
    tokens: ["background", "foreground", "border", "input", "ring"],
  },
  {
    label: "Primary",
    tokens: ["primary", "primary-foreground"],
  },
  {
    label: "Secondary",
    tokens: ["secondary", "secondary-foreground"],
  },
  {
    label: "Muted",
    tokens: ["muted", "muted-foreground", "tertiary-foreground"],
  },
  {
    label: "Accent",
    tokens: ["accent", "accent-foreground"],
  },
  {
    label: "Destructive",
    tokens: ["destructive"],
  },
  {
    label: "Card",
    tokens: ["card", "card-hover", "card-foreground"],
  },
  {
    label: "Popover",
    tokens: ["popover", "popover-foreground"],
  },
  {
    label: "Sidebar",
    tokens: [
      "sidebar",
      "sidebar-foreground",
      "sidebar-primary",
      "sidebar-primary-foreground",
      "sidebar-accent",
      "sidebar-accent-foreground",
      "sidebar-border",
      "sidebar-ring",
    ],
  },
];

function isForegroundToken(name: string) {
  return name.includes("foreground");
}

export function App() {
  const [dark, setDark] = useState(false);

  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);

  return (
    <div className="min-h-screen p-8 max-w-7xl mx-auto">
      <header className="flex items-center justify-between mb-10">
        <h1 className="text-2xl font-bold">astro-theme</h1>
        <button
          onClick={() => setDark(!dark)}
          className="px-4 py-2 rounded-md bg-card border border-border text-foreground text-sm font-medium hover:bg-card-hover transition-colors"
        >
          {dark ? "Light" : "Dark"}
        </button>
      </header>

      {/* Palette grids */}
      <section className="mb-12">
        <h2 className="text-lg font-semibold mb-4">Color Palettes</h2>
        <div className="space-y-3">
          {paletteNames.map((name) => (
            <div key={name} className="flex items-center gap-2">
              <span className="w-16 text-sm font-mono text-muted-foreground shrink-0">
                {name}
              </span>
              <div className="flex gap-1 flex-1">
                {steps.map((step) => (
                  <div key={step} className="flex-1 flex flex-col items-center gap-1">
                    <div
                      className="w-full aspect-square rounded-md border border-border/40"
                      style={{ backgroundColor: `var(--color-${name}-${step})` }}
                    />
                    <span className="text-[10px] text-muted-foreground font-mono">
                      {step}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Semantic tokens */}
      <section>
        <h2 className="text-lg font-semibold mb-4">Semantic Tokens</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {semanticGroups.map((group) => (
            <div key={group.label} className="border border-border rounded-lg p-4">
              <h3 className="text-sm font-semibold mb-3">{group.label}</h3>
              <div className="space-y-2">
                {group.tokens.map((token) => {
                  const lightVal = lightTheme[token as keyof typeof lightTheme];
                  const darkVal = darkTheme[token as keyof typeof darkTheme];
                  const isFg = isForegroundToken(token);
                  return (
                    <div key={token} className="flex items-center gap-3">
                      {isFg ? (
                        <div
                          className="w-8 h-8 rounded border border-border/40 flex items-center justify-center"
                          style={{ color: `var(--${token})` }}
                        >
                          <span className="text-sm font-bold">Aa</span>
                        </div>
                      ) : (
                        <div
                          className="w-8 h-8 rounded border border-border/40"
                          style={{ backgroundColor: `var(--${token})` }}
                        />
                      )}
                      <div className="flex-1 min-w-0">
                        <div className="text-xs font-mono">{token}</div>
                        <div className="text-[10px] font-mono text-muted-foreground truncate">
                          {dark ? darkVal : lightVal}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
