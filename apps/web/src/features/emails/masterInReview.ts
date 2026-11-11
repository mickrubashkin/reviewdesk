import type { EmailListItem } from "./types";

type MasterFields = Pick<EmailListItem, "id" | "review_status" | "open_comment_count">;

// masterInReview returns a translation's EN master while EN is still being
// reviewed: a draft with open comments, so its text is likely to change
// again and the translation waits for it. A draft nobody comments on does
// not hold translations back: EN can sit in draft for weeks with nobody on it.
export function masterInReview<T extends MasterFields>(email: { translation_of: string | null }, emails: T[]): T | null {
  if (!email.translation_of) {
    return null;
  }
  const master = emails.find((candidate) => candidate.id === email.translation_of);
  return master && master.review_status === "draft" && master.open_comment_count > 0 ? master : null;
}
