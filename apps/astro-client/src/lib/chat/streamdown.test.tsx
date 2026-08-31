import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { Streamdown } from "streamdown";
import { deploymentChatRemarkPlugins } from "./streamdown";

// The chat renders through Streamdown, and passing remarkPlugins replaces its
// own list instead of extending it. These assertions cover both halves: emoji
// arrive, and the defaults they were spread onto are still there.
function markdown(text: string) {
  return render(
    <Streamdown mode="static" remarkPlugins={deploymentChatRemarkPlugins}>
      {text}
    </Streamdown>,
  );
}

describe("deploymentChatRemarkPlugins", () => {
  it("renders an emoji shortcode as emoji", () => {
    const { container } = markdown("shipped it :tada:");
    expect(container.textContent).toBe("shipped it 🎉");
  });

  it("renders several shortcodes in one line", () => {
    const { container } = markdown(":rocket: then :fire:");
    expect(container.textContent).toBe("🚀 then 🔥");
  });

  it("leaves a shortcode inside a code span alone", () => {
    const { container } = markdown("use `:tada:` to celebrate");
    expect(container.textContent).toBe("use :tada: to celebrate");
  });

  it("leaves a fenced block alone", () => {
    const { container } = markdown("```\nprint(':tada:')\n```");
    expect(container.textContent).toContain(":tada:");
  });

  it("leaves a clock time alone", () => {
    const { container } = markdown("deploy at 10:30:45 today");
    expect(container.textContent).toBe("deploy at 10:30:45 today");
  });

  it("leaves an unknown name as typed, the way a Slack custom emoji arrives", () => {
    const { container } = markdown("nice :partyparrot: work");
    expect(container.textContent).toBe("nice :partyparrot: work");
  });

  it("keeps GFM strikethrough, which the default plugins provide", () => {
    markdown("~~dropped~~");
    expect(screen.getByText("dropped").tagName.toLowerCase()).toBe("del");
  });

  it("keeps GFM tables, which the default plugins provide", () => {
    markdown("| a | b |\n| - | - |\n| 1 | 2 |");
    expect(screen.getByRole("table")).toBeInTheDocument();
  });
});
