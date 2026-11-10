import { describe, expect, it } from "vitest";

import { ownerOptions } from "./ownerOptions";
import type { UserAdminItem, UserRole } from "./types";

const user = (email: string, role: UserRole, languages: string[] = [], boards: string[] = []): UserAdminItem => ({
  id: email,
  email,
  name: "",
  role,
  created_at: "",
  updated_at: "",
  last_seen_at: null,
  languages,
  managed_boards: boards,
});

const users = [
  user("admin@example.com", "admin"),
  user("manager@example.com", "reviewer", [], ["onboarding"]),
  user("es@example.com", "reviewer", ["es"]),
  user("de@example.com", "reviewer", ["de"]),
  user("en@example.com", "reviewer", ["en", "es"]),
  user("other-board@example.com", "reviewer", [], ["regional"]),
];

describe("ownerOptions", () => {
  it("offers the board's managers and the translators of the email's language", () => {
    expect(ownerOptions(users, [{ language: "es", sequence: "onboarding" }])).toEqual([
      { label: "admin@example.com (admin)", value: "admin@example.com" },
      { label: "manager@example.com (reviewer)", value: "manager@example.com" },
      { label: "es@example.com (translator · ES)", value: "es@example.com" },
      { label: "en@example.com (translator · ES)", value: "en@example.com" },
    ]);
  });

  it("treats EN like any other language", () => {
    const values = ownerOptions(users, [{ language: "en", sequence: "onboarding" }]).map((option) => option.value);
    expect(values).toEqual(["admin@example.com", "manager@example.com", "en@example.com"]);
  });

  it("needs a translator to cover every selected language", () => {
    const values = ownerOptions(users, [
      { language: "es", sequence: "onboarding" },
      { language: "en", sequence: "onboarding" },
    ]).map((option) => option.value);
    expect(values).toEqual(["admin@example.com", "manager@example.com", "en@example.com"]);
  });

  it("keeps the current owner on the list", () => {
    const options = ownerOptions(users, [{ language: "es", sequence: "onboarding" }], "de@example.com");
    expect(options[0]).toEqual({ label: "de@example.com", value: "de@example.com" });
  });
});
