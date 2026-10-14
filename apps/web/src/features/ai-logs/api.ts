import { fetchJson } from "../../shared/api";
import type {
  AIAnalysisLogFilters,
  AIAnalysisLogItem,
} from "../emails/types";

export function fetchAIAnalysisLogs(
  filters: AIAnalysisLogFilters
): Promise<AIAnalysisLogItem[]> {
  const params = new URLSearchParams();

  Object.entries(filters).forEach(([key, value]) => {
    const trimmedValue = value?.trim();
    if (trimmedValue) {
      params.set(key, trimmedValue);
    }
  });

  const query = params.toString();
  return fetchJson<AIAnalysisLogItem[]>(
    `/api/ai-analysis-logs${query ? `?${query}` : ""}`
  );
}
