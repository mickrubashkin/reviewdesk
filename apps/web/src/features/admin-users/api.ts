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
