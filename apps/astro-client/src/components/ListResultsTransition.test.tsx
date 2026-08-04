import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ListResultsTransition } from "./ListResultsTransition";

describe("ListResultsTransition", () => {
  it("restarts the compositor-only entrance when the resolved list key changes", () => {
    const { container, rerender } = render(
      <ListResultsTransition transitionKey="personal:ready">
        <span>First page</span>
      </ListResultsTransition>,
    );
    const firstContainer = container.firstElementChild;

    rerender(
      <ListResultsTransition transitionKey="organization:ready">
        <span>Filtered page</span>
      </ListResultsTransition>,
    );

    expect(container.firstElementChild).not.toBe(firstContainer);
    expect(container.firstElementChild).toHaveClass(
      "animate-in",
      "fade-in-0",
      "slide-in-from-bottom-1",
      "motion-reduce:animate-none",
    );
  });
});
