import { beforeEach, describe, expect, it } from "vitest";
import { clearDraft, loadDraft, saveDraft } from "./chat-draft";

describe("chat-draft", () => {
  beforeEach(() => sessionStorage.clear());

  it("round-trips a draft, isolated per deployment and conversation", () => {
    saveDraft("dep-1", "conv-a", "hello");
    expect(loadDraft("dep-1", "conv-a")).toBe("hello");
    expect(loadDraft("dep-1", "conv-b")).toBe("");
    expect(loadDraft("dep-2", "conv-a")).toBe("");
  });

  it("gives the new-chat composer (null conversation) its own slot", () => {
    saveDraft("dep-1", null, "new chat draft");
    expect(loadDraft("dep-1", null)).toBe("new chat draft");
    expect(loadDraft("dep-1", "conv-a")).toBe("");
  });

  it("removes the slot when saving empty so a sent draft can't resurrect", () => {
    saveDraft("dep-1", "conv-a", "temp");
    saveDraft("dep-1", "conv-a", "");
    expect(loadDraft("dep-1", "conv-a")).toBe("");
    expect(sessionStorage.getItem("astro:chat-draft:dep-1:conv-a")).toBeNull();
  });

  it("clearDraft drops only the target conversation's slot", () => {
    saveDraft("dep-1", "conv-a", "keep this");
    saveDraft("dep-1", "conv-b", "delete this");

    clearDraft("dep-1", "conv-b");

    expect(loadDraft("dep-1", "conv-b")).toBe("");
    expect(sessionStorage.getItem("astro:chat-draft:dep-1:conv-b")).toBeNull();
    // A sibling conversation's draft is untouched.
    expect(loadDraft("dep-1", "conv-a")).toBe("keep this");
  });
});
