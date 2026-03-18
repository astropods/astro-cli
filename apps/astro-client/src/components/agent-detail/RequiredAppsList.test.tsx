import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { RequiredAppsList } from "./RequiredAppsList";

describe("RequiredAppsList", () => {
  it("renders integration labels as provided by data", () => {
    renderWithProviders(
      <RequiredAppsList integrations={[
        { id: "slack", name: "Slack", known: true },
        { id: "google-drive", name: "Google Drive", known: true },
        { id: "github", name: "github", known: true },
        { id: "custom-app", name: "custom-app", known: true },
      ]} />,
    );

    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.getByText("Google Drive")).toBeInTheDocument();
    expect(screen.getByText("github")).toBeInTheDocument();
    expect(screen.getByText("custom-app")).toBeInTheDocument();
  });
});
