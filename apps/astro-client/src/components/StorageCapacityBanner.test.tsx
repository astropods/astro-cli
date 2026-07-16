import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { DeploymentStorageUsage } from "@/lib/api";
import { StorageCapacityBanner } from "./StorageCapacityBanner";

const mockUsage = vi.fn();
vi.mock("@/api/queries/files", () => ({
  useDeploymentStorageUsage: () => mockUsage(),
}));

function usage(partial: Partial<DeploymentStorageUsage>): DeploymentStorageUsage {
  return {
    available: true,
    total_bytes: 1_000_000_000,
    used_bytes: 0,
    available_bytes: 1_000_000_000,
    percent_used: 0,
    ...partial,
  };
}

function renderBanner() {
  return render(<StorageCapacityBanner deploymentId="d1" />);
}

describe("StorageCapacityBanner", () => {
  it("renders nothing below the warning threshold", () => {
    mockUsage.mockReturnValue({ data: usage({ percent_used: 50 }) });
    const { container } = renderBanner();
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when usage is unavailable (e.g. S3 / no statfs)", () => {
    mockUsage.mockReturnValue({ data: usage({ available: false, percent_used: 99 }) });
    const { container } = renderBanner();
    expect(container).toBeEmptyDOMElement();
  });

  it("warns between the warning and critical thresholds", () => {
    mockUsage.mockReturnValue({ data: usage({ percent_used: 88 }) });
    renderBanner();
    expect(screen.getByText(/Storage 88% full/)).toBeInTheDocument();
    expect(screen.queryByText(/almost full/)).not.toBeInTheDocument();
  });

  it("escalates to an 'almost full' message at/above the critical threshold", () => {
    mockUsage.mockReturnValue({ data: usage({ percent_used: 96 }) });
    renderBanner();
    expect(screen.getByText(/Storage almost full \(96%\)/)).toBeInTheDocument();
  });

  it("renders nothing while the reading is still loading", () => {
    mockUsage.mockReturnValue({ data: undefined });
    const { container } = renderBanner();
    expect(container).toBeEmptyDOMElement();
  });
});
