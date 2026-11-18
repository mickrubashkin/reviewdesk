import { describe, expect, it } from "vitest";

import { lastPlaceToRestore, placeWorthKeeping } from "./lastPlace";

describe("placeWorthKeeping", () => {
  it("keeps boards and emails", () => {
    expect(placeWorthKeeping("/boards/onboarding")).toBe(true);
    expect(placeWorthKeeping("/emails/42/review")).toBe(true);
  });

  it("skips the root and new-item forms", () => {
    expect(placeWorthKeeping("/")).toBe(false);
    expect(placeWorthKeeping("/emails/new")).toBe(false);
    expect(placeWorthKeeping("/slots/new")).toBe(false);
  });
});

describe("lastPlaceToRestore", () => {
  it("restores the stored place with its query on the first page of a load", () => {
    expect(lastPlaceToRestore("/boards/sales?view=matrix", true)).toBe("/boards/sales?view=matrix");
  });

  it("does nothing later in a visit or without a stored place", () => {
    expect(lastPlaceToRestore("/boards/sales", false)).toBeNull();
    expect(lastPlaceToRestore(null, true)).toBeNull();
  });

  it("ignores stored values that are not app paths", () => {
    expect(lastPlaceToRestore("https://example.com/", true)).toBeNull();
    expect(lastPlaceToRestore("//example.com/", true)).toBeNull();
    expect(lastPlaceToRestore("/?view=matrix", true)).toBeNull();
    expect(lastPlaceToRestore("/emails/new?stage=x", true)).toBeNull();
  });
});
