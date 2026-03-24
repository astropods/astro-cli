import { startTransition, StrictMode } from "react";
import { hydrateRoot } from "react-dom/client";
import { HydratedRouter } from "react-router/dom";
import "./index.css";

async function bootstrap() {
  if (import.meta.env.VITE_MOCK_API === "true") {
    const { startMockWorker } = await import("./mocks/browser");
    await startMockWorker();
  }

  startTransition(() => {
    hydrateRoot(
      document,
      <StrictMode>
        <HydratedRouter />
      </StrictMode>,
    );
  });
}

bootstrap();
