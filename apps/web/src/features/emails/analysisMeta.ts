import type {
  EmailAnalysisCheckStatus,
  EmailAnalysisVerdict,
} from "./types";

export const verdictMeta = {
  ready: { color: "green", label: "Ready" },
  minor_fixes: { color: "yellow", label: "Minor fixes" },
  needs_work: { color: "red", label: "Needs work" },
} as const satisfies Record<EmailAnalysisVerdict, { color: string; label: string }>;

export const checkStatusMeta = {
  good: { color: "green", label: "Good" },
  weak: { color: "yellow", label: "Weak" },
  bad: { color: "red", label: "Bad" },
} as const satisfies Record<EmailAnalysisCheckStatus, { color: string; label: string }>;

export const priorityMeta = {
  high: { color: "red", label: "High" },
  medium: { color: "yellow", label: "Medium" },
  low: { color: "gray", label: "Low" },
} as const;

export const analysisCheckLabels = {
  subject: "Subject",
  preheader: "Preheader",
  focus: "Focus",
  cta: "CTA",
  stage_alignment: "Stage",
  readability: "Readability",
} as const;

export const streamCheckOrder = [
  "subject",
  "preheader",
  "focus",
  "cta",
  "stage_alignment",
  "readability",
] as const;
