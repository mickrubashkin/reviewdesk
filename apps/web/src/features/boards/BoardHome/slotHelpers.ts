import { liveSendsLabel } from "../../emails/statusSummary";
import type { EmailListItem, StageColumn } from "../../emails/types";

// pickPrimary is the email shown for a slot and language: the live one, then
// the newest version of the default adaptation, then adaptations and old.
export function pickPrimary(emails: EmailListItem[]): EmailListItem | undefined {
  return [...emails].sort((a, b) => primaryRank(b) - primaryRank(a))[0];
}

function primaryRank(email: EmailListItem) {
  let rank = 0;
  // What robots actually send matters most.
  if (email.production_status === "live" || email.production_status === "live_manual") rank += 1000;
  if (email.variant !== "old") rank += 100;
  if (email.adaptation_key === "default") rank += 10;
  const versionNumber = /^v(\d+)$/i.exec(email.variant);
  if (versionNumber) rank += Math.min(Number(versionNumber[1]), 9);
  else if (email.variant === "new") rank += 1;
  return rank;
}

const preferredLanguages = ["en", "de", "es", "pl", "br"];

// matrixLanguages lists the board's languages, the usual five first.
export function matrixLanguages(columns: StageColumn[]) {
  const present = new Set(columns.flatMap((c) => c.emailGroups.flatMap((g) => g.versions.map((v) => v.language))));
  return [
    ...preferredLanguages.filter((l) => present.has(l)),
    ...[...present].filter((l) => !preferredLanguages.includes(l)).sort(),
  ];
}

// Languages the AI translates into; the API has the same list.
export const aiTranslationLanguages = ["de", "es", "pl", "br"];

type CellVersionFields = Pick<
  EmailListItem,
  | "id"
  | "variant"
  | "adaptation_key"
  | "adaptation_label"
  | "production_status"
  | "portal_send_count"
  | "portal_sends_30d"
  | "portal_last_sent_at"
  | "portal_quiet_days"
>;

export type CellVersion = { id: string; label: string; sends: string; live: boolean };

// cellVersions sums up the sends of every version in a matrix cell, the shown
// one first, so the cell can tell when an old text still goes out next to it.
export function cellVersions(cellEmails: CellVersionFields[], primaryId: string): CellVersion[] {
  const ordered = [...cellEmails].sort((a, b) => Number(b.id === primaryId) - Number(a.id === primaryId));
  return ordered.map((email) => {
    const label = `${email.variant}${email.adaptation_key !== "default" ? ` · ${email.adaptation_label}` : ""}`;
    const last = email.portal_last_sent_at
      ? `, last ${new Date(email.portal_last_sent_at).toLocaleDateString(undefined, { day: "numeric", month: "short" })}`
      : "";
    switch (email.production_status) {
      case "live":
        return { id: email.id, label, sends: `live · ${liveSendsLabel(email)}${last}`, live: true };
      case "live_manual":
        return { id: email.id, label, sends: "live (manual)", live: true };
      default:
        return { id: email.id, label, sends: email.portal_send_count > 0 ? `not sent lately${last}` : "not sent", live: false };
    }
  });
}
