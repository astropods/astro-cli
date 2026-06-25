import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { setTheme, useTheme, parseCookieTheme } from "./theme";

function getCookie(name: string): string | undefined {
  const match = document.cookie.match(new RegExp(`(?:^|;\\s*)${name}=([^;]*)`));
  return match?.[1];
}

function clearCookie(name: string) {
  document.cookie = `${name}=;path=/;max-age=0`;
}

function resetTheme() {
  setTheme("dark");
  localStorage.clear();
  clearCookie("astro-theme");
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

    expect(screen.getByTestId("a-theme")).toHaveTextContent("dark");
    expect(screen.getByTestId("b-theme")).toHaveTextContent("dark");

    fireEvent.click(screen.getByTestId("a-light"));

    expect(screen.getByTestId("a-theme")).toHaveTextContent("light");
    expect(screen.getByTestId("b-theme")).toHaveTextContent("light");
  });

  it("toggles the `dark` class on documentElement", () => {
    render(<ThemeProbe id="a" />);

    expect(document.documentElement.classList.contains("dark")).toBe(true);

    fireEvent.click(screen.getByTestId("a-light"));
    expect(document.documentElement.classList.contains("dark")).toBe(false);

    fireEvent.click(screen.getByTestId("a-dark"));
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("persists across remount via localStorage", () => {
    const first = render(<ThemeProbe id="a" />);
    fireEvent.click(screen.getByTestId("a-light"));
    first.unmount();

    render(<ThemeProbe id="b" />);
    expect(screen.getByTestId("b-theme")).toHaveTextContent("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("writes a cookie when setTheme is called", () => {
    render(<ThemeProbe id="a" />);

    fireEvent.click(screen.getByTestId("a-light"));
    expect(getCookie("astro-theme")).toBe("light");

    fireEvent.click(screen.getByTestId("a-dark"));
    expect(getCookie("astro-theme")).toBe("dark");
  });
});

describe("parseCookieTheme", () => {
  it("returns the theme from a valid cookie header", () => {
    expect(parseCookieTheme("astro-theme=dark")).toBe("dark");
    expect(parseCookieTheme("astro-theme=light")).toBe("light");
  });

  it("returns the theme when among multiple cookies", () => {
    expect(parseCookieTheme("session=abc; astro-theme=dark; other=xyz")).toBe("dark");
  });

  it("returns dark for null or missing cookie", () => {
    expect(parseCookieTheme(null)).toBe("dark");
    expect(parseCookieTheme("session=abc")).toBe("dark");
  });

  it("returns dark for invalid cookie values", () => {
    expect(parseCookieTheme("astro-theme=invalid")).toBe("dark");
    expect(parseCookieTheme("astro-theme=auto")).toBe("dark");
  });
});
