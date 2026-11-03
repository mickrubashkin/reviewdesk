import { describe, expect, it } from "vitest";

import { helpRole } from "./helpRole";

describe("helpRole", () => {
  it("names admins by their stored role, whatever else is set", () => {
    expect(helpRole({ role: "super_admin", languages: ["en"], managed_boards: null })).toBe(
      "CRM operator"
    );
    expect(helpRole({ role: "admin", languages: ["de"], managed_boards: ["onboarding"] })).toBe(
      "Board owner"
    );
  });

  it("makes a reviewer who manages a board a region manager", () => {
    expect(helpRole({ role: "reviewer", languages: ["de"], managed_boards: ["onboarding"] })).toBe(
      "Region manager"
    );
  });

  it("makes a reviewer with only languages a translator", () => {
    expect(helpRole({ role: "reviewer", languages: ["pl"], managed_boards: [] })).toBe(
      "Translator"
    );
  });

  it("leaves a reviewer with neither a viewer", () => {
    expect(helpRole({ role: "reviewer", languages: null, managed_boards: null })).toBe("Viewer");
  });
});
