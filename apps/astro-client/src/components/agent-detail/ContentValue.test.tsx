import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ContentValue } from "./ContentValue";

afterEach(cleanup);

describe("ContentValue", () => {
  it("renders Markdown content in pretty mode", () => {
    const { container } = render(<ContentValue content="hello **world**" />);

    expect(container).toHaveTextContent("hello world");
    expect(container.querySelector("strong")).toHaveTextContent("world");
  });

  it("renders literal text in raw mode", () => {
    const { container } = render(
      <ContentValue content="hello **world**" mode="raw" />,
    );

    const pre = container.querySelector("pre");
    expect(pre).toHaveTextContent("hello **world**");
  });

  it("renders JSON content with syntax highlighting", () => {
    const { container } = render(<ContentValue content={{ answer: 42 }} />);

    expect(container.querySelector("pre")).toHaveTextContent('"answer"');
    expect(container).toHaveTextContent("42");
  });

  it("uses the provided empty fallback", () => {
    render(
      <ContentValue
        content=""
        emptyFallback={<span>No content available.</span>}
      />,
    );

    expect(screen.getByText("No content available.")).toBeInTheDocument();
  });
});
