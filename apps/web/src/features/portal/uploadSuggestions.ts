import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { robotsBehind } from "../emails/reviewStatus";
import type { EmailListItem } from "../emails/types";
import { fetchUploadSuggestions, type UploadSuggestion } from "./api";

export const suggestionsKey = ["portal-upload-suggestions"];

type SuggestedEmail = Pick<EmailListItem, "id" | "latest_version" | "uploaded_version" | "review_status" | "upload_robots">;

// useUploadSuggestions maps email id → suggestion. Super admins only.
export function useUploadSuggestions(enabled: boolean) {
  const query = useQuery({ queryKey: suggestionsKey, queryFn: fetchUploadSuggestions, enabled, staleTime: 60_000 });
  return useMemo(() => new Map((query.data ?? []).map((s) => [s.email_id, s])), [query.data]);
}

// seenRobotsBehind are the robots without the version whose paired portal
// user was seen sending it.
export function seenRobotsBehind(suggestion: UploadSuggestion, email: SuggestedEmail) {
  const seen = new Set((suggestion.robots ?? []).map((robot) => robot.robot));
  return robotsBehind(email)
    .map((robot) => robot.robot)
    .filter((robot) => seen.has(robot));
}

// suggestionFor drops a suggestion the email has moved past: marked since,
// changed again after the portal saw it, or sent only by robots that
// already have the version.
export function suggestionFor(suggestions: Map<string, UploadSuggestion>, email: SuggestedEmail) {
  const suggestion = suggestions.get(email.id);
  if (!suggestion || suggestion.version !== email.latest_version || email.uploaded_version === suggestion.version) {
    return null;
  }
  if ((suggestion.unknown_sends ?? suggestion.sends) === 0 && seenRobotsBehind(suggestion, email).length === 0) {
    return null;
  }
  return suggestion;
}

function times(sends: number) {
  return sends === 1 ? "once" : `${sends} times`;
}

export function describeSuggestion(suggestion: UploadSuggestion) {
  const robots = suggestion.robots ?? [];
  const from = robots.length
    ? ` (${[
        ...robots.map((robot) => `${robot.robot}'s robot ${times(robot.sends)}`),
        ...(suggestion.unknown_sends ? [`unknown robots ${times(suggestion.unknown_sends)}`] : []),
      ].join(", ")})`
    : "";
  return `The portal sent version ${suggestion.version} ${times(suggestion.sends)}${from}, last on ${new Date(suggestion.last_sent_at).toLocaleDateString()}.`;
}
