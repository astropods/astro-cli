import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { AgentTabBar } from "./AgentTabBar";

function LocationProbe() {
  return <output aria-label="Current path">{useLocation().pathname}</output>;
}

afterEach(cleanup);

describe("AgentTabBar compact navigation", () => {
  it("uses the design-system select and navigates to the chosen tab", async () => {
    render(
      <MemoryRouter initialEntries={["/acme/agents/dep-1/traces"]}>
        <Routes>
          <Route
            path="/:account/agents/:deploymentId/:tab"
            element={
              <>
                <AgentTabBar />
                <LocationProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>,
    );

    const user = userEvent.setup();
    const trigger = screen.getByRole("combobox", { name: "Select agent tab" });
    expect(trigger).toHaveTextContent("Traces");
    expect(document.querySelector("select")).toBeNull();

    await user.click(trigger);
    await user.click(screen.getByRole("option", { name: /Configure/i }));

    expect(screen.getByRole("status", { name: "Current path" })).toHaveTextContent(
      "/acme/agents/dep-1/configure",
    );
  });
});
