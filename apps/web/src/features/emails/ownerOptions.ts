import { canManageBoard } from "./permissions";
import type { UserAdminItem, UserRole } from "./types";

type OwnedEmail = { language: string; sequence: string };

export type OwnerOption = { label: string; value: string };

// ownerOptions lists who can own all the emails, as the API checks it: the
// people who manage their boards, then the translators whose languages cover
// every one of them, labeled with those languages. currentOwner stays on the
// list even when it would not be offered any more.
export function ownerOptions(users: UserAdminItem[], emails: OwnedEmail[], currentOwner = ""): OwnerOption[] {
  const languages = [...new Set(emails.map((email) => email.language.toLowerCase()))];
  const managers: OwnerOption[] = [];
  const translators: OwnerOption[] = [];
  for (const user of users) {
    const name = `${user.name ? `${user.name} · ` : ""}${user.email}`;
    if (emails.every((email) => canManageBoard(user, email.sequence))) {
      managers.push({ label: `${name} (${formatUserRole(user.role)})`, value: user.email });
    } else if (languages.every((language) => (user.languages ?? []).includes(language))) {
      translators.push({
        label: `${name} (translator · ${languages.map((language) => language.toUpperCase()).join(", ")})`,
        value: user.email,
      });
    }
  }
  const options = [...managers, ...translators];
  const current = currentOwner.trim();
  if (current && !options.some((option) => option.value === current)) {
    options.unshift({ label: current, value: current });
  }
  return options;
}

function formatUserRole(role: UserRole) {
  return role.replaceAll("_", " ");
}
