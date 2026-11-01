import { timingDays } from "../../../emails/firstSend";
import { needsBitrixUpload } from "../../../emails/reviewStatus";
import type { BoardRegion, EmailListItem, EmailVersionGroup, StageColumn } from "../../../emails/types";
import { pickPrimary } from "../slotHelpers";

export type TimelineStep = {
  key: string;
  email: EmailListItem;
  // The other emails of the slot and language, to tell which version is live.
  versions: EmailListItem[];
  // The wait after the previous email of the branch; null when the timing
  // does not say.
  delay: number | null;
  // Days from a deal entering the stage; null from the first unknown delay on.
  day: number | null;
  // The wait of the reference branch at this slot, when this branch is not
  // the reference and the two differ (both known).
  referenceDelay?: number;
};

// A branch is one chain of emails off a stage node: a language, or a region.
// Its cells follow the stage's slots; null is a slot the branch has no email
// for (only kept when branches are compared side by side).
export type TimelineBranch = {
  key: string;
  name: string;
  language: string;
  color: string;
  reference: boolean;
  managers: string[];
  cells: Array<TimelineStep | null>;
};

export type TimelineStage = {
  stage: string;
  title: string;
  branches: TimelineBranch[];
  // Slots with no email in the language shown (by language only).
  missing: number;
};

export type TimelineMode = { kind: "language"; language: string } | { kind: "region"; regions: BoardRegion[] };

const regionColors = ["blue", "teal", "grape", "orange", "pink", "cyan", "lime", "indigo"];

// buildTimeline lays each stage out as branches of emails in send order with
// the days between them: one branch of a language, or one per region of the
// board, side by side slot by slot. The delays add up per branch, the same
// way the first-send estimate counts them.
export function buildTimeline(columns: StageColumn[], mode: TimelineMode): TimelineStage[] {
  return columns.map((column) => {
    if (mode.kind === "language") {
      const cells = branchCells(column.emailGroups, (email) => email.language === mode.language, false);
      const steps = cells.filter((cell): cell is TimelineStep => cell !== null);
      const branch: TimelineBranch = {
        key: mode.language,
        name: mode.language.toUpperCase(),
        language: mode.language,
        color: "blue",
        reference: false,
        managers: [],
        cells: steps,
      };
      return {
        stage: column.stage,
        title: column.title,
        branches: steps.length > 0 ? [branch] : [],
        missing: column.emailGroups.length - steps.length,
      };
    }

    const branches = regionBranches(column.emailGroups, mode.regions);
    return { stage: column.stage, title: column.title, branches, missing: 0 };
  });
}

// regionBranches: the reference first, the other regions after it, and the
// emails without a region in a branch of their own while there are any.
function regionBranches(groups: EmailVersionGroup[], regions: BoardRegion[]): TimelineBranch[] {
  const ordered = [...regions].sort((a, b) => Number(b.reference) - Number(a.reference));
  const known = new Set(regions.map((r) => r.key));
  const defs = ordered.map((region, index) => ({
    key: region.key,
    name: region.name,
    language: region.language,
    reference: region.reference,
    managers: region.managers,
    color: region.reference ? "blue" : regionColors[1 + (index % (regionColors.length - 1))],
    belongs: (email: EmailListItem) => email.region_key === region.key,
  }));
  if (groups.some((g) => g.versions.some((v) => !v.region_key || !known.has(v.region_key)))) {
    defs.push({
      key: "",
      name: "No region",
      language: "en",
      reference: false,
      managers: [],
      color: "gray",
      belongs: (email) => !email.region_key || !known.has(email.region_key),
    });
  }

  const branches = defs.map((def) => ({
    key: def.key,
    name: def.name,
    language: def.language,
    color: def.color,
    reference: def.reference,
    managers: def.managers,
    cells: branchCells(groups, def.belongs, true),
  }));

  // A slot nobody has an email for would be an empty column.
  const used = groups.map((_, i) => branches.some((b) => b.cells[i] !== null));
  const trimmed = branches.map((b) => ({ ...b, cells: b.cells.filter((_, i) => used[i]) }));
  const withEmails = trimmed.filter((b) => b.cells.some((c) => c !== null));

  // Mark the waits that differ from the reference's at the same slot.
  const reference = withEmails.find((b) => b.reference);
  if (reference) {
    for (const branch of withEmails) {
      if (branch === reference) continue;
      branch.cells.forEach((cell, i) => {
        const refDelay = reference.cells[i]?.delay;
        if (cell && cell.delay !== null && refDelay !== undefined && refDelay !== null && refDelay !== cell.delay) {
          cell.referenceDelay = refDelay;
        }
      });
    }
  }
  return withEmails;
}

// branchCells picks the email of each slot the branch owns and counts the
// days along it. keepGaps leaves a null where the branch has no email.
function branchCells(groups: EmailVersionGroup[], belongs: (email: EmailListItem) => boolean, keepGaps: boolean) {
  const cells: Array<TimelineStep | null> = [];
  let day: number | null = 0;
  for (const group of groups) {
    const versions = group.versions.filter(belongs);
    const email = pickPrimary(versions);
    if (!email) {
      if (keepGaps) cells.push(null);
      continue;
    }
    const delay = timingDays(email.send_timing);
    day = day === null || delay === null ? null : day + delay;
    cells.push({ key: group.key, email, versions, delay, day });
  }
  return cells;
}

// formatDelay is the label of the line between two emails.
export function formatDelay(delay: number | null): string {
  if (delay === null) return "?";
  if (delay === 0) return "right away";
  if (delay < 1) return `+${Math.round(delay * 24)} h`;
  return `+${Number.isInteger(delay) ? delay : delay.toFixed(1)} d`;
}

export type StageSummary = {
  emails: number;
  live: number;
  toUpload: number;
  openComments: number;
  // The day the last email goes out; null when a timing on the way is unknown.
  lastDay: number | null;
  branches: number;
};

// summarizeStage is what the stage's node shows when its branches are folded.
export function summarizeStage(stage: TimelineStage): StageSummary {
  const steps = stage.branches.flatMap((branch) => branch.cells.filter((cell): cell is TimelineStep => cell !== null));
  const emails = steps.map((step) => step.email);
  const lastDays = stage.branches.map((branch) => {
    const last = [...branch.cells].reverse().find((cell) => cell !== null);
    return last ? last.day : 0;
  });
  const known = lastDays.filter((day): day is number => day !== null);
  return {
    emails: emails.length,
    live: emails.filter((e) => e.production_status === "live" || e.production_status === "live_manual").length,
    toUpload: emails.filter((e) => needsBitrixUpload(e)).length,
    openComments: emails.reduce((sum, e) => sum + e.open_comment_count, 0),
    lastDay: known.length > 0 ? Math.max(...known) : null,
    branches: stage.branches.length,
  };
}

export function formatDay(day: number | null): string {
  if (day === null) return "?";
  return Number.isInteger(day) ? String(day) : day.toFixed(1);
}

// What to light up on the timeline, so the emails that need someone stand out
// from the rest.
export const highlightOptions = [
  { value: "none", label: "Nothing" },
  { value: "to_upload", label: "To upload" },
  { value: "comments", label: "Open comments" },
  { value: "not_live", label: "Not live" },
  { value: "outdated", label: "Behind" },
] as const;

export type Highlight = (typeof highlightOptions)[number]["value"];

export function isHighlight(value: string | null): value is Highlight {
  return highlightOptions.some((option) => option.value === value);
}

// matchesHighlight: to_upload is a ready email a robot lacks; outdated is one
// whose reference or master changed since it was made.
export function matchesHighlight(email: EmailListItem, highlight: Highlight): boolean {
  switch (highlight) {
    case "none":
      return true;
    case "to_upload":
      return needsBitrixUpload(email);
    case "comments":
      return email.open_comment_count > 0;
    case "not_live":
      return email.production_status !== "live" && email.production_status !== "live_manual";
    case "outdated":
      return Boolean(email.based_on_stale) || email.translation_stale;
  }
}

// countMatches counts the emails of the stages a highlight lights up.
export function countMatches(stages: TimelineStage[], highlight: Highlight): { matching: number; total: number } {
  let matching = 0;
  let total = 0;
  for (const stage of stages) {
    for (const branch of stage.branches) {
      for (const cell of branch.cells) {
        if (!cell) continue;
        total += 1;
        if (matchesHighlight(cell.email, highlight)) matching += 1;
      }
    }
  }
  return { matching, total };
}
