import { changedFieldNames } from "./changeSummary";
import type { EmailActivityItem } from "./types";

// "Changed since your visit". A visit lasts while someone keeps coming back
// within 30 minutes; the API keeps when they last opened a page and when the
// visit before began (email_visits.go, user_visits.go).
const visitGapMs = 30 * 60 * 1000;

// visitSince is when the visit before the current one began, as the API will
// keep it once this open is recorded: a page opened after a 30-minute gap
// starts a new visit, so the last open is the one before. Null means no
// visit before.
export function visitSince(seenAt?: string | null, previousSeenAt?: string | null, now = new Date()) {
  if (!seenAt) {
    return null;
  }
  return now.getTime() - new Date(seenAt).getTime() > visitGapMs ? seenAt : (previousSeenAt ?? null);
}

// isAfter compares two API times, which may come in different offsets.
export function isAfter(time?: string | null, since?: string | null) {
  return Boolean(time && since) && new Date(time as string).getTime() > new Date(since as string).getTime();
}

export type ChangesSinceVisit = {
  since: string;
  fields: string[];
  blocks: string[];
  people: string[];
};

// changesSinceVisit sums up the edits of the text and fields that others made
// after since, from the email's activity; null when there are none.
export function changesSinceVisit(
  activities: EmailActivityItem[],
  since: string | null | undefined,
  myEmail: string
): ChangesSinceVisit | null {
  if (!since) {
    return null;
  }
  const sinceTime = new Date(since).getTime();
  const edits = activities.filter(
    (activity) =>
      activity.type === "email_updated" &&
      new Date(activity.created_at).getTime() > sinceTime &&
      (activity.actor_email ?? "").toLowerCase() !== myEmail.toLowerCase()
  );
  if (edits.length === 0) {
    return null;
  }
  const unique = (values: string[]) => [...new Set(values)];
  return {
    since,
    fields: unique(edits.flatMap((edit) => changedFieldNames(edit.changes))),
    blocks: unique(
      edits.flatMap((edit) => {
        const blocks = edit.metadata.changed_review_blocks;
        return Array.isArray(blocks) ? blocks.filter((block): block is string => typeof block === "string") : [];
      })
    ),
    people: unique(edits.map((edit) => edit.actor_email ?? "").filter(Boolean)),
  };
}
