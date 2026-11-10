import { describe, expect, it } from "vitest";

import { matchesUserSearch } from "./userSearch";

const sam = {
  email: "sam@example.com",
  name: "Sam Lee",
  role: "reviewer" as const,
  languages: ["es"],
  managed_boards: ["onboarding"],
  aliases: ["sam@example.org"],
};

describe("matchesUserSearch", () => {
  it("matches everyone on an empty query", () => {
    expect(matchesUserSearch(sam, "  ")).toBe(true);
  });

  it("finds by name, address, alias, role, language and board", () => {
    for (const query of ["lee", "SAM@EXAMPLE.COM", "example.org", "region manager", "es", "onboarding", "Partners"]) {
      expect(matchesUserSearch(sam, query, new Map([["onboarding", "Partners board"]]))).toBe(true);
    }
  });

  it("needs every word", () => {
    expect(matchesUserSearch(sam, "sam de")).toBe(false);
    expect(matchesUserSearch(sam, "sam onboarding")).toBe(true);
  });
});
