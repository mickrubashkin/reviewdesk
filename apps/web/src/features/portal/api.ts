import { fetchJson } from "../../shared/api";

export type PortalCategory = { id: number; name: string };

export type PortalSyncRun = {
  id: string;
  category_id: number;
  days: number;
  status: "running" | "done" | "failed";
  progress: string;
  activities_seen: number;
  emails_stored: number;
  groups_count: number;
  error: string | null;
  started_by_email: string;
  started_at: string;
  finished_at: string | null;
};

export type PortalStage = { id: string; name: string; sort: number };

export type MatchStatus = "matched" | "differs" | "unmatched";
export type GroupDecision = "confirmed" | "ignored" | "created";

export type PortalGroup = {
  id: string;
  language: string;
  subject: string;
  excerpt: string;
  send_count: number;
  first_sent_at: string;
  last_sent_at: string;
  stages: Record<string, number>;
  match_email_id: string | null;
  match_email_title: string | null;
  match_email_stage: string | null;
  match_score: number;
  match_status: MatchStatus;
  decision: GroupDecision | null;
  decision_email_id: string | null;
  decided_by_email: string | null;
  decided_at: string | null;
};

export type UnseenEmail = {
  id: string;
  title: string;
  stage: string;
  sort_order: number;
  language: string;
  variant: string;
  adaptation_label: string;
  send_timing: string | null;
  send_condition: string | null;
};

export type ManualLiveEmail = UnseenEmail & {
  live_marked_at: string;
  live_marked_by: string;
  live_note: string | null;
};

export type PortalOverview = {
  latest_run: PortalSyncRun | null;
  stages: PortalStage[];
  groups: PortalGroup[];
  unseen: UnseenEmail[];
  manual_live: ManualLiveEmail[];
};

export type PortalGroupDetail = PortalGroup & {
  sample_html: string;
  sample_text: string;
  match: {
    id: string;
    title: string;
    language: string;
    stage: string;
    html: string;
    text: string;
  } | null;
};

export function fetchPortalCategories(): Promise<PortalCategory[]> {
  return fetchJson<PortalCategory[]>("/api/portal/categories");
}

export function fetchPortalOverview(boardKey: string): Promise<PortalOverview> {
  return fetchJson<PortalOverview>(`/api/boards/${encodeURIComponent(boardKey)}/portal`);
}

export function startPortalSync(boardKey: string, payload: { category_id: number; days: number }) {
  return fetchJson<{ run_id: string }>(`/api/boards/${encodeURIComponent(boardKey)}/portal/sync`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export function fetchPortalGroup(id: string, emailID?: string): Promise<PortalGroupDetail> {
  const query = emailID ? `?email_id=${encodeURIComponent(emailID)}` : "";
  return fetchJson<PortalGroupDetail>(`/api/portal/groups/${encodeURIComponent(id)}${query}`);
}

export function decidePortalGroup(
  id: string,
  payload: { decision: GroupDecision | null; email_id?: string }
): Promise<void> {
  return fetchJson<void>(`/api/portal/groups/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export type PortalReconstruction = {
  html: string;
  filled_fields: string[];
  missing_fields: string[];
  from_template: boolean;
  warnings: string[];
  subject: string;
  language: string;
  template_email_id: string;
  template_email_title: string;
  suggested_stage: string;
};

export function reconstructPortalGroup(id: string): Promise<PortalReconstruction> {
  return fetchJson<PortalReconstruction>(`/api/portal/groups/${encodeURIComponent(id)}/reconstruct`);
}

export type PortalImportResult = {
  board_key: string;
  board_name: string;
  created: number;
  linked: number;
  without_fields: number;
  skipped: Array<{ group_id: string; subject: string; reason: string }>;
};

export function importMissingFromPortal(boardKey: string): Promise<PortalImportResult> {
  return fetchJson<PortalImportResult>(`/api/boards/${encodeURIComponent(boardKey)}/portal/import-missing`, {
    method: "POST",
  });
}

// Mirrors the server: unmatched, undecided, a known language, sent at least
// twice, and not a test or a manager's reply or forward.
export function isImportCandidate(group: PortalGroup) {
  return (
    group.decision === null &&
    group.match_status === "unmatched" &&
    group.language !== "" &&
    group.send_count >= 2 &&
    !/^\s*(test|teste|тест)(?:[^\p{L}]|$)/iu.test(group.subject) &&
    !/^\s*(re|fw|fwd|aw)\s*:/i.test(group.subject)
  );
}
