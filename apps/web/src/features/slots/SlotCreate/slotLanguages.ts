import type { EmailHTMLInspection } from "../../emails/types";

export const slotLanguages = ["en", "de", "es", "pl", "br"] as const;
export type SlotLanguage = (typeof slotLanguages)[number];

export type LanguageDraft = {
  html: string;
  subject: string | null; // null = use the subject detected in the HTML
  preheader: string | null;
};

export const emptyDraft: LanguageDraft = { html: "", subject: null, preheader: null };

export function effectiveSubject(draft: LanguageDraft, inspection?: EmailHTMLInspection) {
  return draft.subject ?? inspection?.detection.detected_subject ?? "";
}

export function effectivePreheader(draft: LanguageDraft, inspection?: EmailHTMLInspection) {
  return draft.preheader ?? inspection?.detection.detected_preheader ?? "";
}

// Warnings specific to a slot: language mismatch and field keys that differ
// from the EN master, with the closest EN key as a hint for typos.
export function slotWarnings(
  language: SlotLanguage,
  inspection: EmailHTMLInspection | undefined,
  masterInspection: EmailHTMLInspection | undefined
): string[] {
  if (!inspection) {
    return [];
  }
  const warnings: string[] = [];
  const detected = inspection.detection.detected_language;
  if (detected && detected !== language) {
    warnings.push(
      `The HTML declares lang "${detected.toUpperCase()}", but it is in the ${language.toUpperCase()} tab.`
    );
  }

  if (language !== "en" && masterInspection) {
    const masterKeys = new Set(masterInspection.editable_fields.map((f) => f.key));
    const ownKeys = new Set(inspection.editable_fields.map((f) => f.key));
    for (const key of ownKeys) {
      if (!masterKeys.has(key)) {
        const hint = closestKey(key, [...masterKeys]);
        warnings.push(
          `Field "${key}" is not in EN${hint ? ` — did you mean "${hint}"?` : ""}`
        );
      }
    }
    const missing = [...masterKeys].filter((key) => !ownKeys.has(key));
    if (missing.length > 0) {
      warnings.push(`Fields from EN missing here: ${missing.join(", ")}`);
    }
  }
  return warnings;
}

function closestKey(key: string, candidates: string[]) {
  let best: string | null = null;
  let bestDistance = 3; // only suggest near-misses such as typos
  for (const candidate of candidates) {
    const distance = levenshtein(key, candidate);
    if (distance < bestDistance) {
      best = candidate;
      bestDistance = distance;
    }
  }
  return best;
}

function levenshtein(a: string, b: string) {
  const row = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i++) {
    let previous = row[0];
    row[0] = i;
    for (let j = 1; j <= b.length; j++) {
      const current = row[j];
      row[j] = Math.min(row[j] + 1, row[j - 1] + 1, previous + (a[i - 1] === b[j - 1] ? 0 : 1));
      previous = current;
    }
  }
  return row[b.length];
}
