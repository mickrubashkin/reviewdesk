import type { UserAdminItem } from "../emails/types";
import { helpRole } from "./helpRole";

type SearchFields = Pick<UserAdminItem, "email" | "name" | "role" | "languages" | "managed_boards" | "aliases">;

// matchesUserSearch tells whether a user fits what was typed in the Users
// search: every word must appear in their name, an address (aliases too), the
// role of the help, a language they translate, or a board they manage (by key
// or by name from boardNames).
export function matchesUserSearch(user: SearchFields, query: string, boardNames: Map<string, string> = new Map()) {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return true;
  const haystack = [
    user.name,
    user.email,
    ...(user.aliases ?? []),
    helpRole(user),
    ...(user.languages ?? []),
    ...(user.managed_boards ?? []).flatMap((key) => [key, boardNames.get(key) ?? ""]),
  ]
    .join(" ")
    .toLowerCase();
  return words.every((word) => haystack.includes(word));
}
