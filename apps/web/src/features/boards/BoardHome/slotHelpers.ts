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
