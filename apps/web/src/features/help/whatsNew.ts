import type { AuthUser } from "../emails/types";
import whatsNewMarkdown from "../../../../../docs/help/en/whats-new.md?raw";

// latestNewsDate is the date of the newest entry of "What's new": its first
// "## YYYY-MM-DD" heading, or "" when there is none.
export function latestNewsDate(markdown: string): string {
  return markdown.match(/^##\s+(\d{4}-\d{2}-\d{2})\s*$/m)?.[1] ?? "";
}

export const latestNews = latestNewsDate(whatsNewMarkdown);

// hasUnreadNews: the newest entry is newer than the one this person last
// opened. ISO dates compare as text.
export function hasUnreadNews(user: Pick<AuthUser, "whats_new_seen">, latest = latestNews) {
  return latest > (user.whats_new_seen ?? "");
}
