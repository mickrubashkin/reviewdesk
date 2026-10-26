import type { AuthUser } from "./types";

type PermissionUser = {
  role: AuthUser["role"];
  languages?: string[] | null;
  managed_boards?: string[] | null;
};

export function isAdminRole(role: AuthUser["role"]) {
  return role === "admin" || role === "super_admin";
}

// canManageBoard mirrors the API rule: admins manage every board, others the
// boards listed in Users → Manages; on other boards they only read.
export function canManageBoard(user: PermissionUser, boardKey: string | null | undefined) {
  if (isAdminRole(user.role)) {
    return true;
  }
  return boardKey != null && (user.managed_boards ?? []).includes(boardKey);
}

// managesAnyBoard gates pages that pick the board themselves (New slot, New email).
export function managesAnyBoard(user: PermissionUser) {
  return isAdminRole(user.role) || (user.managed_boards ?? []).length > 0;
}

// canEditEmailContent mirrors the API rule: board managers edit the emails of
// their boards, and translators edit emails in a language they translate
// (Users → Translates).
export function canEditEmailContent(user: PermissionUser, email: { language: string; sequence: string }) {
  if (canManageBoard(user, email.sequence)) {
    return true;
  }
  return (user.languages ?? []).includes(email.language.toLowerCase());
}
