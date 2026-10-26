import { ApiError, fetchJson } from "../../shared/api";
import type {
  BasedOnStatus,
  CreateCommentMessagePayload,
  CreateEmailCommentPayload,
  CreateEmailPayload,
  CreateSlotPayload,
  CreateSlotResponse,
  DuplicateEmailPayload,
  EmailActivityItem,
  EmailAnalysis,
  EmailAreaApproval,
  EmailComment,
  EmailReviewStatus,
  EmailCommentMessage,
  EmailDetail,
  EmailHTMLInspection,
  EmailListItem,
  EmailReviewArea,
  EmailVersionDetail,
  RobotUpload,
  EmailVersionListItem,
  LayoutItem,
  ReferenceDifference,
  RenderedEmail,
  TranslationStatus,
  TranslationSuggestions,
  UpdateEditableFieldsPayload,
  UpdateEmailAreaApprovalPayload,
  UpdateEmailPlanningFieldsPayload,
  UpdateEmailPlanningFieldsResponse,
  UpdateEmailReviewStatusPayload,
  UpdateEmailReviewStatusResponse,
} from "./types";

export function fetchEmailActivity(emailId: string): Promise<EmailActivityItem[]> {
  return fetchJson<EmailActivityItem[]>(
    `/api/emails/${encodeURIComponent(emailId)}/activity`
  );
}

export function fetchEmails(boardKey?: string): Promise<EmailListItem[]> {
  const query = boardKey ? `?board=${encodeURIComponent(boardKey)}` : "";
  return fetchJson<EmailListItem[]>(`/api/emails${query}`);
}

// The board's history: archived emails, newest archive first.
export function fetchArchivedEmails(boardKey: string): Promise<EmailListItem[]> {
  return fetchJson<EmailListItem[]>(`/api/emails?board=${encodeURIComponent(boardKey)}&archived=only`);
}

export async function downloadEmailsExport(
  ids: string[],
  format: "html" | "md",
  markdownOptions?: { sections: string[]; openCommentsOnly: boolean }
): Promise<{ blob: Blob; fileName: string }> {
  const response = await fetch("/api/emails/export", {
    body: JSON.stringify({
      format,
      ids,
      ...(markdownOptions && {
        open_comments_only: markdownOptions.openCommentsOnly,
        sections: markdownOptions.sections,
      }),
    }),
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    method: "POST",
  });

  if (!response.ok) {
    const message = await response.text();
    throw new ApiError(response.status, message || `Request failed: ${response.status}`);
  }

  const disposition = response.headers.get("Content-Disposition") ?? "";
  const fileName =
    /filename="?([^";]+)"?/.exec(disposition)?.[1] ?? `emails-${format}.zip`;

  return { blob: await response.blob(), fileName };
}

export function markEmailLive(
  emailId: string,
  payload: { live: boolean; note?: string }
): Promise<{ live_marked_at: string | null; live_marked_by: string | null; live_note: string | null }> {
  return fetchJson(`/api/emails/${encodeURIComponent(emailId)}/live`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

// version is the version that was copied; the API refuses the mark if the
// email was saved again since. robots are the robots it went into or out
// of; none means every robot of the email's region.
export function markEmailUploaded(
  emailId: string,
  uploaded: boolean,
  version?: number,
  robots?: string[]
): Promise<{ uploaded_version: number | null; uploaded_at: string | null; upload_robots: RobotUpload[] }> {
  return fetchJson(`/api/emails/${encodeURIComponent(emailId)}/uploaded`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ uploaded, version, robots }),
  });
}

export function takeEmail(emailId: string, take: boolean): Promise<{ owner_email: string | null }> {
  return fetchJson(`/api/emails/${encodeURIComponent(emailId)}/take`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ take }),
  });
}

export function fetchTranslationStatus(emailId: string): Promise<TranslationStatus | undefined> {
  return fetchJson<TranslationStatus | undefined>(`/api/emails/${encodeURIComponent(emailId)}/translation`);
}

export function confirmTranslation(emailId: string): Promise<TranslationStatus | undefined> {
  return fetchJson<TranslationStatus | undefined>(
    `/api/emails/${encodeURIComponent(emailId)}/translation/confirm`,
    { method: "POST" }
  );
}

export function suggestTranslation(emailId: string): Promise<TranslationSuggestions> {
  return fetchJson<TranslationSuggestions>(
    `/api/emails/${encodeURIComponent(emailId)}/translation/suggestions`,
    { method: "POST" }
  );
}

export function fetchBasedOnStatus(emailId: string): Promise<BasedOnStatus | undefined> {
  return fetchJson<BasedOnStatus | undefined>(`/api/emails/${encodeURIComponent(emailId)}/based-on`);
}

export function confirmBasedOn(emailId: string): Promise<BasedOnStatus | undefined> {
  return fetchJson<BasedOnStatus | undefined>(`/api/emails/${encodeURIComponent(emailId)}/based-on/confirm`, {
    method: "POST",
  });
}

// setBasedOnLink sets which reference email a region's email adapts; null
// makes it the region's own email.
export function setBasedOnLink(emailId: string, referenceId: string | null): Promise<void> {
  return fetchJson<void>(`/api/emails/${encodeURIComponent(emailId)}/based-on`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reference_id: referenceId }),
  });
}

// fetchReferenceDifferences sums up, per email id, how the board's region
// emails differ from the reference emails they adapt.
export async function fetchReferenceDifferences(boardKey: string): Promise<Record<string, ReferenceDifference>> {
  return (
    await fetchJson<{ emails: Record<string, ReferenceDifference> }>(
      `/api/reference-differences?board=${encodeURIComponent(boardKey)}`
    )
  ).emails;
}

export function placeEmail(
  emailId: string,
  payload: { sequence: string; stage: string; sort_order?: number }
): Promise<{ id: string; sequence: string; stage: string; sort_order: number; translation_of: string | null }> {
  return fetchJson(`/api/emails/${encodeURIComponent(emailId)}/place`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export type CreatedTranslation = {
  email: EmailDetail;
  model: string;
  glossary: string[];
  // Fields left in English because the model's text was unusable.
  kept_english: { key: string; problem: string }[];
};

// createTranslationFromEN creates the missing translation of an EN email in
// its slot, as a draft; translate asks the AI for the text first.
export function createTranslationFromEN(
  masterId: string,
  payload: { language: string; translate: boolean }
): Promise<CreatedTranslation> {
  return fetchJson<CreatedTranslation>(`/api/emails/${encodeURIComponent(masterId)}/translations`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function setTranslationLink(emailId: string, masterId: string | null): Promise<void> {
  return fetchJson<void>(`/api/emails/${encodeURIComponent(emailId)}/translation-link`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ master_id: masterId }),
  });
}

export function fetchEmailDetail(emailId: string): Promise<EmailDetail> {
  return fetchJson<EmailDetail>(`/api/emails/${encodeURIComponent(emailId)}`);
}

export function createEmail(payload: CreateEmailPayload): Promise<EmailDetail> {
  return fetchJson<EmailDetail>("/api/emails", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

export function createSlot(payload: CreateSlotPayload): Promise<CreateSlotResponse> {
  return fetchJson<CreateSlotResponse>("/api/slots", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

export type SlotRef = { sequence: string; stage: string; sort_order: number };
export type SlotPlace = { stage: string; sort_order: number };

// updateSlot edits every language of a slot at once: name, place, timing and
// condition. Fields left out stay as they are; "" clears timing or condition.
export function updateSlot(
  payload: SlotRef & {
    title?: string;
    to_stage?: string;
    to_sort_order?: number;
    send_timing?: string;
    send_condition?: string;
  }
): Promise<SlotPlace> {
  return fetchJson<SlotPlace>("/api/slots", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

// moveSlot puts a slot right before another slot of a stage, or at its end.
export function moveSlot(
  payload: SlotRef & { to_stage: string; before_sort_order: number | null }
): Promise<SlotPlace> {
  return fetchJson<SlotPlace>("/api/slots/move", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export type BulkEmailAction =
  | { action: "owner"; owner_email: string | null }
  | { action: "status"; review_status: EmailReviewStatus }
  | { action: "slot"; stage: string; sort_order?: number }
  | { action: "timing"; send_timing?: string; send_condition?: string }
  | { action: "archive" };

export type BulkEmailResult = {
  updated: number;
  skipped: { id: string; title: string; language: string; reason: string }[];
  slots?: number;
  stage?: string;
  sort_order?: number;
};

// bulkUpdateEmails applies one action to emails selected on a board, all or
// nothing; only "ready" skips the emails its gates hold back.
export function bulkUpdateEmails(
  payload: BulkEmailAction & { sequence: string; email_ids: string[] }
): Promise<BulkEmailResult> {
  return fetchJson<BulkEmailResult>("/api/emails/bulk", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function inspectEmailHTML(originalHTML: string): Promise<EmailHTMLInspection> {
  return fetchJson<EmailHTMLInspection>("/api/emails/inspect-html", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ original_html: originalHTML }),
  });
}

export function fetchRenderedEmail(emailId: string): Promise<RenderedEmail> {
  return fetchJson<RenderedEmail>(
    `/api/emails/${encodeURIComponent(emailId)}/rendered`
  );
}

export function fetchEmailVersions(
  emailId: string
): Promise<EmailVersionListItem[]> {
  return fetchJson<EmailVersionListItem[]>(
    `/api/emails/${encodeURIComponent(emailId)}/versions`
  );
}

export function fetchEmailVersion(
  emailId: string,
  versionId: string
): Promise<EmailVersionDetail> {
  return fetchJson<EmailVersionDetail>(
    `/api/emails/${encodeURIComponent(emailId)}/versions/${encodeURIComponent(versionId)}`
  );
}

export function restoreEmailVersion(
  emailId: string,
  versionId: string
): Promise<void> {
  return fetchJson<void>(
    `/api/emails/${encodeURIComponent(emailId)}/versions/${encodeURIComponent(versionId)}/restore`,
    {
      method: "POST",
    }
  );
}

// Sets the blocks an EN email is built from; its translations follow.
export function updateEmailLayout(emailId: string, layout: LayoutItem[]): Promise<void> {
  return fetchJson<void>(`/api/emails/${encodeURIComponent(emailId)}/layout`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ layout }),
  });
}

export function updateEmailEditableFields(
  emailId: string,
  payload: UpdateEditableFieldsPayload
): Promise<void> {
  return fetchJson<void>(
    `/api/emails/${encodeURIComponent(emailId)}/editable-fields`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function updateEmailReviewStatus(
  emailId: string,
  payload: UpdateEmailReviewStatusPayload
): Promise<UpdateEmailReviewStatusResponse> {
  return fetchJson<UpdateEmailReviewStatusResponse>(
    `/api/emails/${encodeURIComponent(emailId)}/review-status`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function listEmailAreaApprovals(
  emailId: string
): Promise<EmailAreaApproval[]> {
  return fetchJson<EmailAreaApproval[]>(
    `/api/emails/${encodeURIComponent(emailId)}/area-approvals`
  );
}

export function updateEmailAreaApproval(
  emailId: string,
  area: EmailReviewArea,
  payload: UpdateEmailAreaApprovalPayload
): Promise<EmailAreaApproval> {
  return fetchJson<EmailAreaApproval>(
    `/api/emails/${encodeURIComponent(emailId)}/area-approvals/${encodeURIComponent(area)}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function updateEmailPlanningFields(
  emailId: string,
  payload: UpdateEmailPlanningFieldsPayload
): Promise<UpdateEmailPlanningFieldsResponse> {
  return fetchJson<UpdateEmailPlanningFieldsResponse>(
    `/api/emails/${encodeURIComponent(emailId)}/planning-fields`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function duplicateEmail(
  emailId: string,
  payload: DuplicateEmailPayload
): Promise<EmailDetail> {
  return fetchJson<EmailDetail>(
    `/api/emails/${encodeURIComponent(emailId)}/duplicate-as`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function archiveEmail(emailId: string): Promise<void> {
  return fetchJson<void>(`/api/emails/${encodeURIComponent(emailId)}/archive`, {
    method: "PATCH",
  });
}

// Puts an archived email back in its place on the board; refused (409) when
// an active email took that place meanwhile.
export function restoreEmail(emailId: string): Promise<void> {
  return fetchJson<void>(`/api/emails/${encodeURIComponent(emailId)}/restore`, {
    method: "PATCH",
  });
}

export function analyzeEmail(emailId: string): Promise<EmailAnalysis> {
  return fetchJson<EmailAnalysis>(
    `/api/emails/${encodeURIComponent(emailId)}/ai-analysis`,
    {
      method: "POST",
    }
  );
}

export async function fetchSharedEmailAnalysis(
  emailId: string
): Promise<EmailAnalysis | null> {
  const response = await fetch(`/api/emails/${encodeURIComponent(emailId)}/ai-analysis`, {
    credentials: "include",
  });

  if (response.status === 204) {
    return null;
  }

  if (!response.ok) {
    throw new Error(`Request failed with ${response.status}`);
  }

  return (await response.json()) as EmailAnalysis;
}

export function analyzeEmailStream(
  emailId: string,
  handlers: {
    onDelta: (text: string) => void;
    onResult?: (analysis: EmailAnalysis) => void;
    onDone?: () => void;
    onError?: (error: string) => void;
  },
  options: {
    refresh?: boolean;
  } = {}
) {
  const searchParams = new URLSearchParams();
  if (options.refresh) {
    searchParams.set("refresh", "true");
  }
  const query = searchParams.toString();
  const url = `/api/emails/${encodeURIComponent(emailId)}/ai-analysis-stream${query ? `?${query}` : ""}`;

  const source = new EventSource(url, { withCredentials: true });

  source.addEventListener("delta", (event) => {
    try {
      const text = JSON.parse((event as MessageEvent).data);
      handlers.onDelta(text);
    } catch {
      handlers.onDelta((event as MessageEvent).data);
    }
  });

  source.addEventListener("result", (event) => {
    try {
      const analysis = JSON.parse((event as MessageEvent).data) as EmailAnalysis;
      handlers.onResult?.(analysis);
    } catch {
      handlers.onError?.("failed to parse stream result");
      source.close();
    }
  });

  source.addEventListener("done", () => {
    handlers.onDone?.();
    source.close();
  });

  source.addEventListener("error", (event) => {
    if (event instanceof MessageEvent && typeof event.data === "string" && event.data) {
      try {
        handlers.onError?.(JSON.parse(event.data) as string);
      } catch {
        handlers.onError?.(event.data);
      }
    } else {
      handlers.onError?.("stream error");
    }
    source.close();
  });

  return () => {
    source.close();
  };
}

export function fetchEmailComments(emailId: string): Promise<EmailComment[]> {
  return fetchJson<EmailComment[]>(
    `/api/emails/${encodeURIComponent(emailId)}/comments`
  );
}

export function createEmailComment(
  emailId: string,
  payload: CreateEmailCommentPayload
): Promise<EmailComment> {
  return fetchJson<EmailComment>(
    `/api/emails/${encodeURIComponent(emailId)}/comments`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function createCommentMessage(
  commentId: string,
  payload: CreateCommentMessagePayload
): Promise<EmailCommentMessage> {
  return fetchJson<EmailCommentMessage>(
    `/api/comments/${encodeURIComponent(commentId)}/messages`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function resolveComment(commentId: string): Promise<EmailComment> {
  return fetchJson<EmailComment>(
    `/api/comments/${encodeURIComponent(commentId)}/resolve`,
    {
      method: "PATCH",
    }
  );
}

// updateEmailRegion puts an email in a region of its board; "" leaves it
// without one.
export function updateEmailRegion(emailId: string, regionKey: string): Promise<void> {
  return fetchJson<void>(`/api/emails/${encodeURIComponent(emailId)}/region`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ region_key: regionKey }),
  });
}
