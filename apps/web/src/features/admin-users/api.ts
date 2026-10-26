import { fetchJson } from "../../shared/api";
import type {
  CreateAdminUserPayload,
  UserAdminItem,
  UserRole,
} from "../emails/types";

export function fetchAdminUsers(): Promise<UserAdminItem[]> {
  return fetchJson<UserAdminItem[]>("/api/admin/users");
}

export function createAdminUser(
  payload: CreateAdminUserPayload
): Promise<UserAdminItem> {
  return fetchJson<UserAdminItem>("/api/admin/users", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

export function updateAdminUserRole(
  userId: string,
  role: UserRole
): Promise<UserAdminItem> {
  return fetchJson<UserAdminItem>(
    `/api/admin/users/${encodeURIComponent(userId)}/role`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ role }),
    }
  );
}

export function updateAdminUserLanguages(
  userId: string,
  languages: string[]
): Promise<UserAdminItem> {
  return fetchJson<UserAdminItem>(
    `/api/admin/users/${encodeURIComponent(userId)}/languages`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ languages }),
    }
  );
}

export function updateAdminUserBoards(
  userId: string,
  boards: string[]
): Promise<UserAdminItem> {
  return fetchJson<UserAdminItem>(`/api/admin/users/${encodeURIComponent(userId)}/boards`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ boards }),
  });
}

export function updateAdminUserName(userId: string, name: string): Promise<UserAdminItem> {
  return fetchJson<UserAdminItem>(`/api/admin/users/${encodeURIComponent(userId)}/name`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
}

export function deleteAdminUser(userId: string): Promise<void> {
  return fetchJson<void>(`/api/admin/users/${encodeURIComponent(userId)}`, { method: "DELETE" });
}

// mergeAdminUsers folds fromUserId into intoUserId; the merged user's address
// becomes an alias.
export function mergeAdminUsers(intoUserId: string, fromUserId: string): Promise<UserAdminItem> {
  return fetchJson<UserAdminItem>(`/api/admin/users/${encodeURIComponent(intoUserId)}/merge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ from_user_id: fromUserId }),
  });
}

export function addAdminUserAlias(userId: string, email: string): Promise<{ aliases: string[] }> {
  return fetchJson(`/api/admin/users/${encodeURIComponent(userId)}/aliases`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  });
}

export function removeAdminUserAlias(userId: string, email: string): Promise<{ aliases: string[] }> {
  return fetchJson(
    `/api/admin/users/${encodeURIComponent(userId)}/aliases/${encodeURIComponent(email)}`,
    { method: "DELETE" }
  );
}
