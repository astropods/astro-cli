import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { SidebarAuthor } from "./SidebarAuthor";

describe("SidebarAuthor", () => {
  it("renders the Authors section title", () => {
    renderWithProviders(
      <SidebarAuthor authors={[]} ownerName="Acme Corp" ownerHandle="acme" />,
    );
    expect(screen.getByText("Authors")).toBeInTheDocument();
  });

  it("falls back to owner when authors array is empty", () => {
    renderWithProviders(
      <SidebarAuthor authors={[]} ownerName="Acme Corp" ownerHandle="acme" />,
    );
    expect(screen.getByText("Acme Corp")).toBeInTheDocument();
    expect(screen.getByText("@acme")).toBeInTheDocument();
  });

  it("renders a single author with name and handle", () => {
    renderWithProviders(
      <SidebarAuthor
        authors={[{ name: "Jane Smith", account: "janesmith" }]}
        ownerName="Jane Smith"
        ownerHandle="janesmith"
      />,
    );
    expect(screen.getByText("Jane Smith")).toBeInTheDocument();
    expect(screen.getByText("@janesmith")).toBeInTheDocument();
  });

  it("links to the author profile when account is present", () => {
    renderWithProviders(
      <SidebarAuthor
        authors={[{ name: "Jane Smith", account: "janesmith" }]}
        ownerName="Jane Smith"
        ownerHandle="janesmith"
      />,
    );
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/janesmith");
  });

  it("renders an author without account as plain text (no link)", () => {
    renderWithProviders(
      <SidebarAuthor
        authors={[{ name: "Anonymous Author" }]}
        ownerName="Acme Corp"
        ownerHandle="acme"
      />,
    );
    expect(screen.getByText("Anonymous Author")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("renders up to three authors as full cards", () => {
    renderWithProviders(
      <SidebarAuthor
        authors={[
          { name: "Alice", account: "alice" },
          { name: "Bob", account: "bob" },
          { name: "Carol", account: "carol" },
        ]}
        ownerName="Acme Corp"
        ownerHandle="acme"
      />,
    );
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("Carol")).toBeInTheDocument();
    expect(screen.getAllByRole("link")).toHaveLength(3);
  });

  it("renders four or more authors in compact avatar-only mode", () => {
    renderWithProviders(
      <SidebarAuthor
        authors={[
          { name: "Alice", account: "alice" },
          { name: "Bob", account: "bob" },
          { name: "Carol", account: "carol" },
          { name: "Dave", account: "dave" },
        ]}
        ownerName="Acme Corp"
        ownerHandle="acme"
      />,
    );
    // Full name text should not appear as visible labels in compact mode
    expect(screen.queryByText("@alice")).not.toBeInTheDocument();
    expect(screen.queryByText("@bob")).not.toBeInTheDocument();
    // All four links still rendered (compact avatars are still linked)
    expect(screen.getAllByRole("link")).toHaveLength(4);
  });

  it("handles a long author name without crashing", () => {
    const longName = "A".repeat(80);
    renderWithProviders(
      <SidebarAuthor
        authors={[{ name: longName, account: "longname" }]}
        ownerName="Acme Corp"
        ownerHandle="acme"
      />,
    );
    expect(screen.getByText(longName)).toBeInTheDocument();
  });

  it("handles names with symbols", () => {
    renderWithProviders(
      <SidebarAuthor
        authors={[{ name: "O'Brien & Co. <dev>", account: "obrien" }]}
        ownerName="Acme Corp"
        ownerHandle="acme"
      />,
    );
    expect(screen.getByText("O'Brien & Co. <dev>")).toBeInTheDocument();
  });

  it("handles owner handle with symbols in fallback state", () => {
    renderWithProviders(
      <SidebarAuthor
        authors={[]}
        ownerName="Dr. Ångström-Müller"
        ownerHandle="dr_angstrom"
      />,
    );
    expect(screen.getByText("Dr. Ångström-Müller")).toBeInTheDocument();
    expect(screen.getByText("@dr_angstrom")).toBeInTheDocument();
  });
});
