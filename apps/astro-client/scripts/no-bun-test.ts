/**
 * This preload script runs when `bun test` is invoked directly.
 * We use Vitest, not Bun's native test runner.
 *
 * Run `bun run test` instead — it invokes Vitest via the package.json script.
 */
console.error(`
╔══════════════════════════════════════════════════════════════╗
║  ERROR: Do not use \`bun test\` in astro-client.             ║
║                                                              ║
║  Astro Client uses Vitest, not Bun's native test runner.     ║
║  Please run this instead:                                    ║
║                                                              ║
║    bun run test                                              ║
╚══════════════════════════════════════════════════════════════╝
`);
process.exit(1);
