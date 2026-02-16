function getCLIPrefix(): string {
  if (typeof window !== "undefined" && window.location.hostname.includes("astropod.ai")) {
    return "ast-preview";
  }
  return "ast";
}

export function Dev() {
  // Domain the page is hosted on (e.g. https://astro.example.com) so curl uses the same host
  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const prefix = getCLIPrefix();

  const DOWNLOADS = [
    { name: `${prefix}-darwin-amd64`, label: "macOS (Intel)" },
    { name: `${prefix}-darwin-arm64`, label: "macOS (Apple Silicon)" },
  ];

  const CURL_INSTALLS = [
    { label: "macOS (Intel)", name: `${prefix}-darwin-amd64` },
    { label: "macOS (Apple Silicon)", name: `${prefix}-darwin-arm64` },
  ];

  return (
    <div className="max-w-2xl space-y-8 p-6 md:p-8">
      <div>
        <h1 className="text-2xl font-semibold mb-1">Developer</h1>
        <p className="text-muted-foreground">
          Download the Astro CLI, install it, and run your first agent.
        </p>
      </div>

      <section>
        <h2 className="text-lg font-medium mb-2">Download</h2>
        <p className="text-sm text-muted-foreground mb-3">
          Get the Astro CLI <code className="bg-muted px-1 rounded">{prefix}</code> for your platform (served from this host):
        </p>
        <ul className="list-disc list-inside space-y-1 text-sm">
          {DOWNLOADS.map(({ name, label }) => (
            <li key={name}>
              <a
                href={`${origin}/download/${name}`}
                className="text-primary hover:underline"
                download={name}
              >
                {label}
              </a>{" "}
              <span className="text-muted-foreground">({name})</span>
            </li>
          ))}
        </ul>
      </section>

      <section>
        <h2 className="text-lg font-medium mb-2">Install</h2>
        <p className="text-sm text-muted-foreground mb-3">
          Run one of these to download and install <code className="bg-muted px-1 rounded">{prefix}</code> into <code className="bg-muted px-1 rounded">/usr/local/bin</code>:
        </p>
        <div className="space-y-4">
          {CURL_INSTALLS.map(({ label, name }) => (
            <div key={name}>
              <p className="text-xs font-medium text-muted-foreground mb-1">{label}</p>
              <pre className="bg-muted p-3 rounded-md text-sm overflow-x-auto">
                <code>{`curl -fsSL ${origin}/download/${name} -o ${prefix} && chmod +x ${prefix}`}</code>
              </pre>
            </div>
          ))}
        </div>
        <p className="text-xs text-muted-foreground mt-3">
          On Mac, if the binary is blocked, open <strong>Settings → Privacy &amp; Security</strong> and click “Allow” for the downloaded file so it can run.
        </p>
      </section>

      <section>
        <h2 className="text-lg font-medium mb-2">Quick start</h2>
        <p className="text-sm text-muted-foreground mb-2">
          Create a new agent and follow the instructions to run it locally:
        </p>
        <pre className="bg-muted p-4 rounded-md text-sm overflow-x-auto">
          <code>{`ast create hello-astro`}</code>
        </pre>
        <p className="text-sm text-muted-foreground mt-2">
          Then open <code className="bg-muted px-1 rounded">ast docs help</code> or{" "}
          <code className="bg-muted px-1 rounded">ast --help</code> for more.
        </p>
      </section>
    </div>
  );
}
