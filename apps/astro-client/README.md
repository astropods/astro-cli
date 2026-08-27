# Astro Client

Web frontend for the Astro platform.

## Quick Start

The client runs against a locally running astro-server. Run the client on its own:

```bash
bun install
bun run dev          # or: moon run astro-client:dev
```

The app is available at `http://localhost:5173` and talks to the backend at `VITE_API_URL` (default `http://localhost:8080`). To bring up the whole stack (client + server + Traefik) behind `http://localhost`, see the [repo README](../../README.md).

## API Communication

The Vite dev server proxies `/api`, `/download`, `/install`, `/auth`, and `/webhooks` to `VITE_API_URL`. Auth calls (`/auth/*`) are additionally issued directly to the backend URL so session cookies are set on the right origin.

## Environment Variables

| Variable       | Description     | Default                 |
| -------------- | --------------- | ----------------------- |
| `VITE_API_URL` | Backend API URL (Vite proxy target) | `http://localhost:8080` |
| `VITE_ASSETS_URL` | CDN base for static assets (icons, etc.) | `""` (served locally by Vite) |
| `VITE_AMPLITUDE_API_KEY` | Amplitude analytics key (omit to disable telemetry) | unset |

## Project Structure

```
src/
├── api/queries/    # TanStack Query hooks and tests
├── components/     # React components
├── contexts/       # React contexts (auth, etc.)
├── lib/           # Utilities and API client
├── pages/         # Page components
├── test/          # Test setup, utilities, and MSW mocks
└── main.tsx       # Entry point
```

## Scripts

| Script                  | Description                                  |
| ----------------------- | -------------------------------------------- |
| `bun run dev`           | Start the dev server (`react-router dev`)    |
| `bun run build`         | Build for production                         |
| `bun run start`         | Serve the production build (`server.ts`)     |
| `bun run typecheck`     | Type-check (`react-router typegen` + `tsc`)  |
| `bun run lint`          | Run ESLint                                   |
| `bun run test`          | Run tests once                               |
| `bun run test:watch`    | Run tests in watch mode                      |
| `bun run test:coverage` | Run tests with coverage report               |
| `bun run test:e2e`      | Run Playwright browser E2E tests             |
| `bun run storybook`     | Run Storybook (port 6006)                    |
| `bun run build-storybook` | Build the static Storybook                 |

## Testing

Tests use [Vitest](https://vitest.dev/) with [React Testing Library](https://testing-library.com/docs/react-testing-library/intro/) and [MSW](https://mswjs.io/) for API mocking.

```bash
bun run test            # single run
bun run test:watch      # re-run on file changes
bun run test:coverage   # single run with coverage report
```

Tests live next to the code they cover (e.g. `src/api/queries/agents.test.tsx`). Shared test infrastructure is in `src/test/`:

| File                   | Purpose                                                                                                  |
| ---------------------- | -------------------------------------------------------------------------------------------------------- |
| `test/setup.ts`        | Global setup — starts MSW server, resets handlers between tests                                          |
| `test/test-utils.tsx`  | `renderWithProviders` and `createHookWrapper` — wraps components/hooks with QueryClient and MemoryRouter |
| `test/msw/handlers.ts` | Default MSW request handlers and fixture data                                                            |
| `test/msw/server.ts`   | MSW server instance                                                                                      |

To override a handler for a specific test, use `server.use()` inside the test — it resets automatically after each test via the setup file.

### Client E2E tests (Playwright)

Run these from the repository root with Moon:

```bash
moon run astro-client:e2e.setup   # one-time browser install (or after Playwright upgrades)
moon run astro-client:e2e         # run all client browser E2E tests
```

You can also run from `apps/astro-client` directly:

```bash
bun run test:e2e
```
