import { describe, expect, it } from "vitest";
import {
  classifyUserId,
  UNATTRIBUTED_USER_KEY,
  UNIDENTIFIED_USER_KEY,
} from "./user-classification";

describe("classifyUserId", () => {
  const members = new Set(["u_alice", "u_bob"]);

  it("returns UNATTRIBUTED_USER_KEY for null", () => {
    expect(classifyUserId(null, members)).toBe(UNATTRIBUTED_USER_KEY);
  });

  it("returns UNATTRIBUTED_USER_KEY for undefined", () => {
    expect(classifyUserId(undefined, members)).toBe(UNATTRIBUTED_USER_KEY);
  });

  it("returns UNATTRIBUTED_USER_KEY for empty string", () => {
    expect(classifyUserId("", members)).toBe(UNATTRIBUTED_USER_KEY);
  });

  it("returns the original id when the user is a member", () => {
    expect(classifyUserId("u_alice", members)).toBe("u_alice");
  });

  it("returns UNIDENTIFIED_USER_KEY when the user is not a member", () => {
    expect(classifyUserId("u_outside", members)).toBe(UNIDENTIFIED_USER_KEY);
  });

  it("treats every id as unidentified when there are no members", () => {
    expect(classifyUserId("u_anyone", new Set())).toBe(UNIDENTIFIED_USER_KEY);
  });
});
