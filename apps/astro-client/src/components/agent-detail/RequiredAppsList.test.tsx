import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { RequiredAppsList } from "./RequiredAppsList";

describe("RequiredAppsList", () => {
  it("formats integration labels and preserves GitHub casing", () => {
    renderWithProviders(
      <RequiredAppsList integrations={["Slack", "Google Drive", "github", "custom-app"]} />,
    );

    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.getByText("Google drive")).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("Custom app")).toBeInTheDocument();
  });
});
