import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { setTheme, useTheme } from "./theme";

function resetTheme() {
  setTheme("light");
  localStorage.clear();
  document.documentElement.classList.remove("dark");
}

afterEach(() => {
  cleanup();
  resetTheme();
});

beforeEach(() => {
  resetTheme();
});

function ThemeProbe({ id }: { id: string }) {
  const { theme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid={`${id}-theme`}>{theme}</span>
      <button data-testid={`${id}-light`} onClick={() => setTheme("light")}>light</button>
      <button data-testid={`${id}-dark`} onClick={() => setTheme("dark")}>dark</button>
    </div>
  );
}

describe("useTheme", () => {
  it("setting theme from one consumer updates a sibling consumer", () => {
    render(
      <>
        <ThemeProbe id="a" />
        <ThemeProbe id="b" />
      </>,
    );

    expect(screen.getByTestId("a-theme")).toHaveTextContent("light");
    expect(screen.getByTestId("b-theme")).toHaveTextContent("light");

    fireEvent.click(screen.getByTestId("a-dark"));

    expect(screen.getByTestId("a-theme")).toHaveTextContent("dark");
    expect(screen.getByTestId("b-theme")).toHaveTextContent("dark");
  });

  it("toggles the `dark` class on documentElement", () => {
    render(<ThemeProbe id="a" />);

    fireEvent.click(screen.getByTestId("a-dark"));
    expect(document.documentElement.classList.contains("dark")).toBe(true);

    fireEvent.click(screen.getByTestId("a-light"));
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("persists across remount via localStorage", () => {
    const first = render(<ThemeProbe id="a" />);
    fireEvent.click(screen.getByTestId("a-dark"));
    first.unmount();

    render(<ThemeProbe id="b" />);
    expect(screen.getByTestId("b-theme")).toHaveTextContent("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });
});
