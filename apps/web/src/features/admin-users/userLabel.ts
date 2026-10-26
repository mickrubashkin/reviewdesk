import type { UserAdminItem } from "../emails/types";

// userLabel is how the admin pages name a person: their name, else the email.
export function userLabel(user: UserAdminItem) {
  return user.name || user.email;
}
