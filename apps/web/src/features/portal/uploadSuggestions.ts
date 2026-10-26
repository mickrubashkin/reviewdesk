import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import type { EmailListItem } from "../emails/types";
import { fetchUploadSuggestions, type UploadSuggestion } from "./api";

export const suggestionsKey = ["portal-upload-suggestions"];

// useUploadSuggestions maps email id → suggestion. Super admins only.
export function useUploadSuggestions(enabled: boolean) {
  const query = useQuery({ queryKey: suggestionsKey, queryFn: fetchUploadSuggestions, enabled, staleTime: 60_000 });
  return useMemo(() => new Map((query.data ?? []).map((s) => [s.email_id, s])), [query.data]);
}

// suggestionFor drops a suggestion the email has moved past: marked since,
// or changed again after the portal saw it.
export function suggestionFor(
  suggestions: Map<string, UploadSuggestion>,
  email: Pick<EmailListItem, "id" | "latest_version" | "uploaded_version">
) {
  const suggestion = suggestions.get(email.id);
  if (!suggestion || suggestion.version !== email.latest_version || email.uploaded_version === suggestion.version) {
    return null;
  }
  return suggestion;
}

export function describeSuggestion(suggestion: UploadSuggestion) {
  const sends = suggestion.sends === 1 ? "once" : `${suggestion.sends} times`;
  return `The portal sent version ${suggestion.version} ${sends}, last on ${new Date(suggestion.last_sent_at).toLocaleDateString()}.`;
}
