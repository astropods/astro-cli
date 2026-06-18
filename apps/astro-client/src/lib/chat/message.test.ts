import { describe, expect, it } from "vitest";
import { serverTurnInFlight, inFlightAssistantMessageId } from "./message";

describe("serverTurnInFlight", () => {
  it("returns false when messages is null", () => {
    expect(
      serverTurnInFlight({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        messages: null as unknown as [],
      }),
    ).toBe(false);
  });

  it("returns true when trailing message is user and stream is active", () => {
    expect(
      serverTurnInFlight({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        assistant_streaming: true,
        messages: [{ id: "u1", role: "user", content: "hi" }],
      }),
    ).toBe(true);
  });

  it("returns false when server cleared assistant_streaming despite user tail", () => {
    expect(
      serverTurnInFlight({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        assistant_streaming: false,
        messages: [
          { id: "a1", role: "assistant", content: "done" },
          { id: "u1", role: "user", content: "stuck" },
        ],
      }),
    ).toBe(false);
  });
});

describe("inFlightAssistantMessageId", () => {
  it("returns null while awaiting the first assistant chunk", () => {
    expect(
      inFlightAssistantMessageId({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        assistant_streaming: true,
        messages: [{ id: "u1", role: "user", content: "hi" }],
      }),
    ).toBeNull();
  });

  it("returns the tail assistant id while streaming", () => {
    expect(
      inFlightAssistantMessageId({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        assistant_streaming: true,
        messages: [
          { id: "u1", role: "user", content: "hi" },
          { id: "a1", role: "assistant", content: "hello" },
        ],
      }),
    ).toBe("a1");
  });
});
