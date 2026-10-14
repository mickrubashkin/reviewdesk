import type {
  EmailAnalysisCheckStatus,
  EmailAnalysisVerdict,
  EmailRecommendation,
} from "./types";
import { streamCheckOrder } from "./analysisMeta";

export type StreamPreview = {
  summary: string | null;
  score: number | null;
  verdict: EmailAnalysisVerdict | null;
  checks: Partial<Record<(typeof streamCheckOrder)[number], EmailAnalysisCheckStatus>>;
  recommendations: Partial<EmailRecommendation>[];
};

export function buildStreamPreview(text: string): StreamPreview {
  const checks: StreamPreview["checks"] = {};
  setPreviewCheck(checks, "subject", extractEnumField(text, "subject", ["good", "weak", "bad"]));
  setPreviewCheck(checks, "preheader", extractEnumField(text, "preheader", ["good", "weak", "bad"]));
  setPreviewCheck(checks, "focus", extractEnumField(text, "focus", ["good", "weak", "bad"]));
  setPreviewCheck(checks, "cta", extractEnumField(text, "cta", ["good", "weak", "bad"]));
  setPreviewCheck(
    checks,
    "stage_alignment",
    extractEnumField(text, "stage_alignment", ["good", "weak", "bad"])
  );
  setPreviewCheck(
    checks,
    "readability",
    extractEnumField(text, "readability", ["good", "weak", "bad"])
  );

  return {
    summary: extractStringField(text, "summary"),
    score: extractNumberField(text, "score"),
    verdict: extractEnumField(text, "verdict", [
      "ready",
      "minor_fixes",
      "needs_work",
    ]),
    checks,
    recommendations: extractRecommendationPreviews(text),
  };
}

function setPreviewCheck(
  checks: StreamPreview["checks"],
  check: (typeof streamCheckOrder)[number],
  status: EmailAnalysisCheckStatus | null
) {
  if (status) {
    checks[check] = status;
  }
}

function extractStringField(text: string, field: string) {
  const pattern = new RegExp(`"${field}"\\s*:\\s*"((?:\\\\.|[^"\\\\])*)`);
  const match = text.match(pattern);
  if (!match) {
    return null;
  }

  return decodeJSONFragment(match[1]);
}

function extractNumberField(text: string, field: string) {
  const pattern = new RegExp(`"${field}"\\s*:\\s*(\\d+)`);
  const match = text.match(pattern);
  if (!match) {
    return null;
  }

  return Number(match[1]);
}

function extractEnumField<T extends string>(
  text: string,
  field: string,
  values: readonly T[]
) {
  const value = extractStringField(text, field);
  if (!value) {
    return null;
  }

  return values.includes(value as T) ? (value as T) : null;
}

function extractRecommendationPreviews(text: string) {
  const recommendations: Partial<EmailRecommendation>[] = [];
  const itemPattern = /\{[^{}]*"priority"\s*:\s*"(high|medium|low)"[^{}]*/g;
  const matches = text.matchAll(itemPattern);

  for (const match of matches) {
    const chunk = match[0];
    recommendations.push({
      priority: match[1] as EmailRecommendation["priority"],
      title: extractStringField(chunk, "title") ?? undefined,
      details: extractStringField(chunk, "details") ?? undefined,
    });
  }

  return recommendations.slice(0, 3);
}

function decodeJSONFragment(value: string) {
  try {
    return JSON.parse(`"${value}"`) as string;
  } catch {
    return value.replace(/\\"/g, '"');
  }
}
