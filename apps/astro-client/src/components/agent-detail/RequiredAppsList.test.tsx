import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { RequiredAppsList } from "./RequiredAppsList";

describe("RequiredAppsList", () => {
  it("renders integration labels as provided by data", () => {
    renderWithProviders(
      <RequiredAppsList integrations={["Slack", "Google Drive", "github", "custom-app"]} />,
    );

    expect(screen.getByText("Slack")).toBeInTheDocument();
    expect(screen.getByText("Google Drive")).toBeInTheDocument();
    expect(screen.getByText("github")).toBeInTheDocument();
    expect(screen.getByText("custom-app")).toBeInTheDocument();
  });
});
