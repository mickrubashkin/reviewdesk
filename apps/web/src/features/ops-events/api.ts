import { fetchJson } from "../../shared/api";
import type {
  OperationalEventFilters,
  OperationalEventItem,
} from "../emails/types";

export function fetchOperationalEvents(
  filters: OperationalEventFilters
): Promise<OperationalEventItem[]> {
  const params = new URLSearchParams();

  Object.entries(filters).forEach(([key, value]) => {
    const trimmedValue = value?.trim();
    if (trimmedValue) {
      params.set(key, trimmedValue);
    }
  });

  const query = params.toString();
  return fetchJson<OperationalEventItem[]>(
    `/api/admin/operational-events${query ? `?${query}` : ""}`
  );
}
