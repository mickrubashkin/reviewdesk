import type { EmailListItem } from "./types";

const dayMs = 24 * 60 * 60 * 1000;

// timingDays reads how long a robot waits from the send timing as people
// write it ("immediately", "2 days after previous email", "+3 days");
// null when it does not say.
export function timingDays(timing: string | null): number | null {
  const text = (timing ?? "").trim().toLowerCase();
  if (!text) return null;
  if (text === "immediately") return 0;
  const days = text.match(/(\d+)\s*days?\b/);
  if (days) return Number(days[1]);
  const hours = text.match(/(\d+)\s*(?:hours?|h)\b/);
  if (hours) return Number(hours[1]) / 24;
  return null;
}

export type FirstSendEstimate = {
  // The first day the portal can show a send.
  from: string;
  // The robots' delay counted from a deal entering the stage.
  days: number;
};

// firstSendEstimate is for an email pasted into a robot that the portal has
// not seen sent. A new robot acts only on deals that enter the stage after
// it is set up, and each email of the stage waits for the one before it, so
// the first send comes no sooner than the upload plus the delays of the
// stage's emails up to this one; the nightly portal sync shows it a day
// later. null when the email is not waiting or a delay in the chain is not
// written down.
export function firstSendEstimate(email: EmailListItem, boardEmails: EmailListItem[]): FirstSendEstimate | null {
  if (email.production_status !== "not_seen" || !email.uploaded_at || email.archived_at || !email.stage) {
    return null;
  }
  // One timing per slot of the stage up to this one; a slot's languages
  // share it, this email's language wins if they differ.
  const timings = new Map<number, string | null>();
  for (const other of boardEmails) {
    if (other.archived_at || other.sequence !== email.sequence || other.stage !== email.stage || other.sort_order > email.sort_order) {
      continue;
    }
    if (!timings.has(other.sort_order) || other.language === email.language) {
      timings.set(other.sort_order, other.send_timing);
    }
  }
  timings.set(email.sort_order, email.send_timing);

  let days = 0;
  for (const timing of timings.values()) {
    const delay = timingDays(timing);
    if (delay === null) return null;
    days += delay;
  }
  const from = new Date(new Date(email.uploaded_at).getTime() + (days + 1) * dayMs);
  return { from: from.toISOString(), days };
}

// withFirstSendEstimates gives each email waiting for its first send, with
// production_status already set, the estimate of when it can show.
export function withFirstSendEstimates<T extends EmailListItem>(emails: T[]): T[] {
  return emails.map((email) => {
    const estimate = firstSendEstimate(email, emails);
    return estimate ? { ...email, first_send: estimate } : email;
  });
}
