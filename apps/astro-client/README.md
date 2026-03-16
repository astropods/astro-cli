# Astro Client

Web frontend for the Astro platform.

## Quick Start

### Local Backend Development

If you're running the astro-server locally:

```bash
bun install
bun run dev
```

The app will be available at `http://localhost:5173`.

### Remote Backend Development

To develop the frontend against a deployed backend (e.g., `https://astropods.ai`), you need to set up local HTTPS with a same-site domain. This is required for authentication cookies to work properly.

**One-time setup (macOS):**

```bash
bun run setup
```

This interactive script will:
1. Add `local.astropods.ai` to your `/etc/hosts` (requires sudo)
2. Install [mkcert](https://github.com/FiloSottile/mkcert) for local certificate generation
3. Generate trusted HTTPS certificates
4. Configure `.env` for the remote backend

**After setup:**

```bash
bun run dev
```

Open `https://local.astropods.ai:5173` in your browser.

### Manual Setup (Linux/Other)

If the setup script doesn't work for your OS:

1. **Add hosts entry:**
   ```bash
   echo "127.0.0.1 local.astropods.ai" | sudo tee -a /etc/hosts
   ```

2. **Install mkcert:**
   - Linux: `sudo apt install mkcert` or see [mkcert docs](https://github.com/FiloSottile/mkcert#installation)
   - Then run: `mkcert -install`

3. **Generate certificates:**
   ```bash
   mkdir -p .certs
   cd .certs
   mkcert local.astropods.ai
   ```

4. **Configure environment:**
   ```bash
   echo "VITE_API_URL=https://astropods.ai" > .env
   ```

5. **Start the dev server:**
   ```bash
   bun run dev
   ```

## Architecture

### Authentication Flow

When developing against a remote backend, authentication uses a same-site subdomain approach:

1. Your local frontend runs at `https://local.astropods.ai:5173`
2. The backend runs at `https://astropods.ai`
3. Both share the same registrable domain (`astropods.ai`)
4. Session cookies set by the backend work on your local domain

This avoids `SameSite` cookie restrictions without compromising security.

### API Communication

- **Auth endpoints** (`/auth/*`): Called directly to the backend URL for proper cookie handling
- **API endpoints** (`/api/*`): Proxied through Vite dev server to the backend

## Environment Variables

| Variable       | Description     | Default                 |
| -------------- | --------------- | ----------------------- |
| `VITE_API_URL` | Backend API URL | `http://localhost:8080` |

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
| `bun dev`               | Start development server                     |
| `bun build`             | Build for production                         |
| `bun preview`           | Preview production build                     |
| `bun lint`              | Run ESLint                                   |
| `bun run test`          | Run tests once                               |
| `bun run test:watch`    | Run tests in watch mode                      |
| `bun run test:coverage` | Run tests with coverage report               |
| `bun run test:e2e`      | Run Playwright browser E2E tests             |
| `bun setup`             | Set up local development environment (macOS) |

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

## Troubleshooting

### "State parameter mismatch" error

This happens when cookies aren't being shared properly. Make sure:
- You're accessing the app via `https://local.astropods.ai:5173` (not `localhost`)
- The setup script completed successfully
- Your browser trusts the local certificate (check for HTTPS errors)

### Certificate not trusted

Run `mkcert -install` to install the local CA in your system trust store.

### "No session found" after login

Ensure:
1. The backend has `http://localhost:5173` and `https://local.astropods.ai:5173` in `ALLOWED_ORIGINS`
2. The backend has `AUTH_COOKIE_DOMAIN=.astropods.ai` set
3. You're using the HTTPS local domain URL
