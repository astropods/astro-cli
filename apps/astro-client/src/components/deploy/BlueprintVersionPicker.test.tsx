import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BlueprintVersion } from "@/lib/api";
import { BlueprintVersionPicker } from "./BlueprintVersionPicker";

afterEach(cleanup);

const versions: BlueprintVersion[] = [
  {
    build_id: "latest-build-2222",
    published_at: "2026-07-13T12:00:00Z",
    commit_message: "Fix stale configuration inputs",
    commit_sha: "2222222abcdef",
    repo_full_name: "astropods/example-agent",
    spec: {},
  },
  {
    build_id: "current-build-1111",
    published_at: "2026-07-12T12:00:00Z",
    commit_message: "Full stack rewrite",
    commit_sha: "1111111abcdef",
    repo_full_name: "astropods/example-agent",
    spec: {},
  },
  {
    build_id: "older-build-0000",
    published_at: "2026-07-11T12:00:00Z",
    commit_message: "Initial stable release",
    spec: {},
  },
];

describe("BlueprintVersionPicker", () => {
  it("marks the latest and currently deployed builds", async () => {
    const user = userEvent.setup();
    const onBuildChange = vi.fn();
    render(
      <BlueprintVersionPicker
        versions={versions}
        selectedBuildId="current-build-1111"
        currentBuildId="current-build-1111"
        latestBuildId="latest-build-2222"
        onBuildChange={onBuildChange}
      />,
    );

    expect(screen.getByText("Blueprint version")).toBeInTheDocument();
    expect(screen.getByText("Select the published build this agent should run.")).toBeInTheDocument();
    expect(screen.getByText("New build available")).toBeInTheDocument();
    expect(screen.queryByText("Update available")).not.toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Blueprint version" })).toHaveTextContent(
      "Full stack rewrite",
    );
    expect(screen.getByRole("combobox", { name: "Blueprint version" })).toHaveTextContent(
      "1111111",
    );
    expect(screen.getByRole("combobox", { name: "Blueprint version" })).toHaveTextContent(
      /Pushed .* ago/,
    );

    await user.click(screen.getByRole("combobox", { name: "Blueprint version" }));

    expect(screen.getByRole("option", { name: /fix stale configuration inputs.*2222222.*latest.*pushed/i })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /full stack rewrite.*current/i })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("option", { name: /initial stable release/i })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /initial stable release/i })).toHaveTextContent(
      "older",
    );
    expect(screen.getByText("Latest")).toHaveClass("text-mono-sm");
    expect(screen.getByText("Latest")).toHaveClass("text-indigo-700");
    expect(screen.getByText("Latest")).toHaveClass("rounded-full");
    expect(screen.getByText("Latest")).not.toHaveClass("text-[10px]");
    await user.click(screen.getByRole("option", { name: /initial stable release/i }));
    expect(onBuildChange).toHaveBeenCalledWith("older-build-0000");
  });

  it("keeps the deployed build selectable when it is absent from readable versions", async () => {
    const user = userEvent.setup();
    render(
      <BlueprintVersionPicker
        versions={versions.slice(0, 1)}
        selectedBuildId="private-current-build"
        currentBuildId="private-current-build"
        latestBuildId="latest-build-2222"
        onBuildChange={() => {}}
      />,
    );

    const trigger = screen.getByRole("combobox", { name: "Blueprint version" });
    expect(trigger).toHaveTextContent("Build private");
    expect(trigger.textContent?.match(/private/g)).toHaveLength(1);
    await user.click(screen.getByRole("combobox", { name: "Blueprint version" }));
    expect(screen.getByRole("option", { name: /private.*current/i })).toHaveAttribute("aria-selected", "true");
  });

});
