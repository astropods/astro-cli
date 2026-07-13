import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { DeploymentStatusBadge } from "./DeploymentTile";

afterEach(cleanup);

// In-progress phases spin; settled states show no motion. Deploying keeps the
// amber warning treatment; the build phase (Preparing/Building) is styled blue
// on its own card so the two read as distinct when stacked.
describe("DeploymentStatusBadge motion", () => {
  it("spins for the deploying phase", () => {
    const { container } = render(<DeploymentStatusBadge status="deploying" />);
    expect(screen.getByText("Deploying")).toBeInTheDocument();
    expect(container.querySelector(".animate-spin")).toBeTruthy();
  });

  it("spins for undeploying", () => {
    const { container } = render(<DeploymentStatusBadge status="undeploying" />);
    expect(screen.getByText("Undeploying")).toBeInTheDocument();
    expect(container.querySelector(".animate-spin")).toBeTruthy();
  });

  it("shows no motion icon once active", () => {
    const { container } = render(<DeploymentStatusBadge status="active" />);
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(container.querySelector(".animate-spin")).toBeFalsy();
  });
});
