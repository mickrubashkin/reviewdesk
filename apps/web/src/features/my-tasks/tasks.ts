import { useQuery } from "@tanstack/react-query";

import { useNotifications } from "../comments/notifications";
import { fetchEmails } from "../emails/api";
import { needsBitrixUpload } from "../emails/reviewStatus";
import type { AuthUser, EmailListItem } from "../emails/types";
import type { UploadSuggestion } from "../portal/api";
import { suggestionFor, useUploadSuggestions } from "../portal/uploadSuggestions";

export type TaskSectionKey = "outdated" | "comments" | "drafts" | "seen" | "upload";

export type TaskSection = {
  key: TaskSectionKey;
  title: string;
  hint: string;
  emails: EmailListItem[];
};

export function isMine(email: EmailListItem, user: AuthUser) {
  return (email.owner_email ?? "").toLowerCase() === user.email.toLowerCase();
}

// isTakenByOther: someone else claimed it, so it is not waiting for me.
export function isTakenByOther(email: EmailListItem, user: AuthUser) {
  return Boolean(email.owner_email) && !isMine(email, user);
}

// Mine first, then unclaimed, then taken by someone else.
function byClaim(user: AuthUser) {
  const rank = (email: EmailListItem) => (isMine(email, user) ? 0 : isTakenByOther(email, user) ? 2 : 1);
  return (a: EmailListItem, b: EmailListItem) => rank(a) - rank(b);
}

// buildTaskSections picks the emails that need something from this person:
// translations in their languages that fell behind EN, comments and drafts
// on emails they own, and for super admins the Bitrix24 upload queue.
export function buildTaskSections(
  emails: EmailListItem[],
  user: AuthUser,
  suggestions: Map<string, UploadSuggestion> = new Map()
): TaskSection[] {
  const languages = new Set((user.languages ?? []).map((language) => language.toLowerCase()));
  const sections: TaskSection[] = [
    {
      key: "outdated",
      title: "Translations to update",
      hint: "EN changed after these were translated. Take one so others translating the same language know you are on it.",
      emails: emails
        .filter((email) => email.translation_stale && (languages.has(email.language.toLowerCase()) || isMine(email, user)))
        .sort(byClaim(user)),
    },
    {
      key: "comments",
      title: "Open comments on my emails",
      hint: "Reply or resolve, then mark the email ready.",
      emails: emails.filter((email) => isMine(email, user) && email.open_comment_count > 0),
    },
    {
      key: "drafts",
      title: "My drafts",
      hint: "Emails I own that are not ready yet.",
      emails: emails.filter((email) => isMine(email, user) && email.review_status === "draft"),
    },
  ];
  if (user.role === "super_admin") {
    const toUpload = emails.filter(needsBitrixUpload);
    const seen = toUpload.filter((email) => suggestionFor(suggestions, email) !== null);
    sections.push(
      {
        key: "seen",
        title: "Already sent by the portal",
        hint: "The portal sends the current version, but it is not marked uploaded. Check and mark it.",
        emails: seen,
      },
      {
        key: "upload",
        title: "To upload to Bitrix24",
        hint: "Ready emails that the robot does not have yet, or has an older version of.",
        emails: toUpload.filter((email) => !seen.includes(email)),
      }
    );
  }
  return sections;
}

// countTasks counts distinct emails across sections, so an email that is
// both a draft and has comments counts once. Translations someone else took
// do not count.
export function countTasks(sections: TaskSection[], user: AuthUser) {
  return new Set(
    sections.flatMap((section) =>
      section.emails
        .filter((email) => section.key !== "outdated" || !isTakenByOther(email, user))
        .map((email) => email.id)
    )
  ).size;
}

export function useMyTasks(user: AuthUser) {
  const emailsQuery = useQuery({
    queryKey: ["emails", "all-boards"],
    queryFn: () => fetchEmails(),
    staleTime: 30_000,
  });
  const suggestions = useUploadSuggestions(user.role === "super_admin");
  const notificationsQuery = useNotifications();
  const sections = emailsQuery.data ? buildTaskSections(emailsQuery.data, user, suggestions) : [];
  const mentions = notificationsQuery.data ?? [];
  return {
    emailsQuery,
    mentions,
    sections,
    suggestions,
    // Each unread mention counts on its own: it is a message to read, not an email.
    count: countTasks(sections, user) + mentions.length,
  };
}
