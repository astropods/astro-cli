import { useState } from "react";
import { CheckIcon, ClipboardIcon, DownloadIcon, LogInIcon, PlusIcon, PlayIcon, UploadIcon, ArrowUpCircleIcon, TerminalIcon } from "lucide-react";

function getCLIPrefix(): string {
  if (typeof window !== "undefined" && window.location.hostname.includes("astropod.ai")) {
    return "ast-preview";
  }
  return "ast";
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      onClick={copy}
      className="shrink-0 p-1.5 rounded-md text-stone-400 hover:text-stone-600 hover:bg-stone-200/60 transition-colors cursor-pointer"
      aria-label="Copy to clipboard"
    >
      {copied ? <CheckIcon className="size-4 text-green-600" /> : <ClipboardIcon className="size-4" />}
    </button>
  );
}

function CodeBlock({ children }: { children: string }) {
  return (
    <div className="flex items-center gap-2 bg-stone-900 text-stone-100 px-4 py-3 rounded-lg font-mono text-sm overflow-x-auto">
      <span className="text-stone-500 select-none shrink-0">$</span>
      <code className="flex-1 min-w-0">{children}</code>
      <CopyButton text={children} />
    </div>
  );
}

const STEPS = [
  {
    icon: DownloadIcon,
    title: "Install the CLI",
    description: "Run this in your terminal:",
    commandFn: (origin: string) => `curl -fsSL ${origin}/install | sh`,
    note: "macOS only. You may need to allow the binary in Settings → Privacy & Security.",
  },
  {
    icon: LogInIcon,
    title: "Log in",
    description: "Authenticate with your Astro account:",
    commandFn: (_: string, prefix: string) => `${prefix} login`,
  },
  {
    icon: PlusIcon,
    title: "Create an agent",
    description: "Scaffold a new project:",
    commandFn: (_: string, prefix: string) => `${prefix} create hello-astro`,
  },
  {
    icon: PlayIcon,
    title: "Run locally",
    description: "Start a local dev environment with hot-reload:",
    commandFn: (_: string, prefix: string) => `cd hello-astro && ${prefix} dev`,
  },
  {
    icon: UploadIcon,
    title: "Publish",
    description: "Push your agent to the Astro registry:",
    commandFn: (_: string, prefix: string) => `${prefix} publish`,
  },
];

export default function Docs() {
  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const prefix = getCLIPrefix();

  return (
    <div className="max-w-2xl mx-auto px-6 py-10 md:py-14">
      {/* Header */}
      <div className="mb-10">
        <div className="inline-flex items-center gap-2 text-xs font-medium text-teal-700 bg-teal-50 border border-teal-200 px-2.5 py-1 rounded-full mb-4">
          <TerminalIcon className="size-3" />
          Getting Started
        </div>
        <h1 className="text-3xl font-semibold tracking-tight mb-2">
          Deploy your first agent
        </h1>
        <p className="text-stone-500 text-base">
          Install the Astro CLI and go from zero to a running agent in five steps.
        </p>
      </div>

      {/* Steps */}
      <div className="relative">
        {/* Vertical line connecting steps */}
        <div className="absolute left-[19px] top-10 bottom-10 w-px bg-stone-200" />

        <div className="space-y-8">
          {STEPS.map((step, i) => {
            const Icon = step.icon;
            return (
              <div key={i} className="relative flex gap-4">
                {/* Step number circle */}
                <div className="relative z-10 flex items-center justify-center size-10 shrink-0 rounded-full bg-white border border-stone-300 text-stone-600">
                  <Icon className="size-4" />
                </div>

                {/* Content */}
                <div className="flex-1 min-w-0 pt-1.5">
                  <h2 className="text-sm font-semibold text-stone-900 mb-1">
                    <span className="text-stone-400 mr-1.5">{i + 1}.</span>
                    {step.title}
                  </h2>
                  <p className="text-sm text-stone-500 mb-3">{step.description}</p>
                  <CodeBlock>{step.commandFn(origin, prefix)}</CodeBlock>
                  {step.note && (
                    <p className="text-xs text-stone-400 mt-2">{step.note}</p>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Footer section */}
      <div className="mt-12 pt-8 border-t border-stone-200 space-y-6">
        <div className="flex items-start gap-4">
          <div className="flex items-center justify-center size-10 shrink-0 rounded-full bg-stone-50 border border-stone-200 text-stone-500">
            <ArrowUpCircleIcon className="size-4" />
          </div>
          <div className="pt-1">
            <h3 className="text-sm font-semibold text-stone-900 mb-1">Keeping up to date</h3>
            <p className="text-sm text-stone-500 mb-3">
              Upgrade to the latest version at any time:
            </p>
            <CodeBlock>{`${prefix} upgrade`}</CodeBlock>
          </div>
        </div>

        <p className="text-sm text-stone-400 pl-14">
          Run <code className="font-mono text-stone-500 bg-stone-100 border border-stone-200 px-1.5 py-0.5 rounded text-xs">{prefix} --help</code> to see all available commands.
        </p>
      </div>
    </div>
  );
}
