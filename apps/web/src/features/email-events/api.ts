import { fetchJson } from "../../shared/api";
import type {
  EmailEventFilters,
  EmailEventItem,
} from "../emails/types";

export function fetchEmailEvents(
  filters: EmailEventFilters
): Promise<EmailEventItem[]> {
  const params = new URLSearchParams();

  Object.entries(filters).forEach(([key, value]) => {
    const trimmedValue = value?.trim();
    if (trimmedValue) {
      params.set(key, trimmedValue);
    }
  });

  const query = params.toString();
  return fetchJson<EmailEventItem[]>(
    `/api/admin/email-events${query ? `?${query}` : ""}`
  );
}
