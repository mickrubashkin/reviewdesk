import { fetchJson } from "../../shared/api";
import type {
  AuthEventFilters,
  AuthEventItem,
} from "../emails/types";

export function fetchAuthEvents(
  filters: AuthEventFilters
): Promise<AuthEventItem[]> {
  const params = new URLSearchParams();

  Object.entries(filters).forEach(([key, value]) => {
    const trimmedValue = value?.trim();
    if (trimmedValue) {
      params.set(key, trimmedValue);
    }
  });

  const query = params.toString();
  return fetchJson<AuthEventItem[]>(
    `/api/auth/events${query ? `?${query}` : ""}`
  );
}
