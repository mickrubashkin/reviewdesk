import type { UserAdminItem } from "../emails/types";

// The roles of the help (docs/help/en/README.md), ranked from least to most
// access. The stored role alone does not say which one a reviewer is: Manages
// makes them a region manager, Translates alone a translator.
export const helpRoles = [
  "Viewer",
  "Translator",
  "Region manager",
  "Board owner",
  "CRM operator",
] as const;

export type HelpRole = (typeof helpRoles)[number];

type RoleFields = Pick<UserAdminItem, "role" | "languages" | "managed_boards">;

export function helpRole(user: RoleFields): HelpRole {
  if (user.role === "super_admin") {
    return "CRM operator";
  }
  if (user.role === "admin") {
    return "Board owner";
  }
  if ((user.managed_boards ?? []).length > 0) {
    return "Region manager";
  }
  if ((user.languages ?? []).length > 0) {
    return "Translator";
  }
  return "Viewer";
}
