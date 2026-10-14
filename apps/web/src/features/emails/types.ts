export type EmailListItem = {
  id: string;
  sequence: string;
  title: string;
  subject: string | null;
  preheader: string | null;
  send_timing: string | null;
  send_condition: string | null;
  stage: string | null;
  sort_order: number;
  language: string;
  variant: EmailVariant;
  adaptation_key: string;
  adaptation_label: string;
  review_status: EmailReviewStatus;
  owner_email: string | null;
  reviewer_email: string | null;
  due_date: string | null;
  implementation_notes: string | null;
  open_comment_count: number;
  open_blocking_comment_count: number;
  portal_send_count: number;
  portal_last_sent_at: string | null;
  // The EN master this email translates, and whether its text changed since.
  translation_of: string | null;
  translation_stale: boolean;
  live_marked_at?: string | null;
  // Derived on the board from the fields above and the board's last sync.
  production_status?: ProductionStatus;
};

// live: the portal sync saw robots send it; live_manual: marked by hand;
// not_seen: a sync ran but never saw it; unknown: no sync yet.
export type TranslationStatus = {
  master_id: string;
  master_title: string;
  master_language: string;
  translated_from_version: number;
  master_latest_version: number;
  stale: boolean;
  changes: Array<{ key: string; before: string; after: string }>;
  checked_at: string | null;
  checked_by: string | null;
};

export type ProductionStatus = "live" | "live_manual" | "not_seen" | "unknown";

export type EmailDetail = EmailListItem & {
  live_marked_at?: string | null;
  live_marked_by?: string | null;
  live_note?: string | null;
  slug: string;
  original_html: string;
  review_html: string | null;
  template_html: string;
  template_hash: string | null;
  template_version: string | null;
  editable_fields: EditableFields;
};

export type EmailVariant = string;

export type EmailReviewStatus =
  | "draft"
  | "in_review"
  | "changes_requested"
  | "approved"
  | "production_approved";

export type EmailCommentSeverity = "suggestion" | "issue" | "blocking";

export type Board = {
  id: string;
  key: string;
  name: string;
  stages: string[];
  created_at: string;
  updated_at: string;
  portal_synced_at: string | null;
};

export type CreateBoardPayload = {
  name: string;
  key?: string;
  source_board_key?: string;
};

export type CreateBoardStagePayload = {
  name: string;
  key?: string;
};

export type UpdateBoardStagePayload = {
  name: string;
};

export type ReorderBoardStagesPayload = {
  stages: string[];
};

export type BoardApprovalArea = {
  id: string;
  key: string;
  name: string;
  required: boolean;
  sort_order: number;
  archived_at: string | null;
};

export type CreateBoardApprovalAreaPayload = {
  name: string;
  key?: string;
  required?: boolean;
};

export type UpdateBoardApprovalAreaPayload = {
  name?: string;
  required?: boolean;
  archived?: boolean;
};

export type ReorderBoardApprovalAreasPayload = {
  areas: string[];
};

export type CreateEmailPayload = {
  sequence?: string;
  title: string;
  subject?: string;
  preheader?: string;
  send_timing?: string;
  stage: string;
  sort_order: number;
  language: string;
  variant: EmailVariant;
  adaptation_label?: string;
  original_html: string;
};

export type EmailHTMLInspection = {
  review_block_count: number;
  original_review_block_count: number;
  editable_field_count: number;
  editable_fields: Array<{
    key: string;
    type: EditableFieldType;
    value_preview: string;
  }>;
  warnings: string[];
  review_html: string;
  detection: {
    detected_language: string;
    detected_subject: string;
    detected_preheader: string;
    has_preheader_slot: boolean;
    nested_markup_fields: Array<{ key: string; tags: string[] }>;
    bitrix_expressions: string[];
  };
};

export type CreateSlotPayload = {
  sequence: string;
  stage: string;
  title: string;
  sort_order?: number;
  send_timing?: string;
  send_condition?: string;
  emails: Array<{
    language: string;
    subject?: string;
    preheader?: string;
    original_html: string;
  }>;
};

export type CreateSlotResponse = {
  sort_order: number;
  emails: EmailDetail[];
};

export type DuplicateEmailPayload = {
  sequence?: string;
  stage?: string;
  sort_order?: number;
  language?: string;
  variant?: EmailVariant;
  adaptation_label?: string;
  title?: string;
  subject?: string;
  preheader?: string;
};

export type EditableFieldType = "text" | "url" | "image" | "number";

export type EditableField = {
  type: EditableFieldType;
  value: string | number;
  order?: number;
};

export type EditableFields = Record<string, EditableField>;

export type UpdateEditableFieldsPayload = {
  title: string;
  subject: string;
  preheader: string;
  editable_fields: EditableFields;
  original_html?: string;
};

export type UpdateEmailReviewStatusPayload = {
  review_status: EmailReviewStatus;
};

export type UpdateEmailReviewStatusResponse = {
  review_status: EmailReviewStatus;
};

export type EmailReviewArea = string;

export type EmailAreaApprovalStatus =
  | "pending"
  | "approved"
  | "changes_requested"
  | "stale";

export type EmailAreaApproval = {
  board_approval_area_id: string;
  area: EmailReviewArea;
  name: string;
  required: boolean;
  status: EmailAreaApprovalStatus;
  decision_note: string | null;
  decided_by_email: string | null;
  content_snapshot_hash: string | null;
  decided_at: string | null;
};

export type UpdateEmailAreaApprovalPayload = {
  status: "approved" | "changes_requested";
  decision_note: string | null;
};

export type UpdateEmailPlanningFieldsPayload = {
  owner_email: string | null;
  reviewer_email: string | null;
  due_date: string | null;
  implementation_notes: string | null;
  send_timing: string | null;
  send_condition: string | null;
  adaptation_label: string;
};

export type UpdateEmailPlanningFieldsResponse =
  UpdateEmailPlanningFieldsPayload;

export type RenderedEmail = {
  html: string;
};

export type EmailVersionSource = "initial" | "manual" | "restore";

export type EmailVersionListItem = {
  id: string;
  email_id: string;
  version_number: number;
  created_at: string;
  created_by_user_id: string | null;
  created_by_email: string;
  source: EmailVersionSource;
  restored_from_version_id: string | null;
  restored_from_version_number: number | null;
  changed_field_count: number;
  changed_metadata_count: number;
  html_changed: boolean;
};

export type EmailVersionDetail = EmailVersionListItem & {
  title: string;
  subject: string | null;
  preheader: string | null;
  original_html: string;
  template_html: string;
  editable_fields: EditableFields;
  review_html: string;
};

export type EmailAnalysis = {
  summary: string;
  score: number;
  verdict: EmailAnalysisVerdict;
  checks: EmailAnalysisChecks;
  recommendations: EmailRecommendation[];
};

export type EmailAnalysisVerdict = "ready" | "minor_fixes" | "needs_work";

export type EmailAnalysisCheckStatus = "good" | "weak" | "bad";

export type EmailAnalysisChecks = {
  subject: EmailAnalysisCheckStatus;
  preheader: EmailAnalysisCheckStatus;
  focus: EmailAnalysisCheckStatus;
  cta: EmailAnalysisCheckStatus;
  stage_alignment: EmailAnalysisCheckStatus;
  readability: EmailAnalysisCheckStatus;
};

export type EmailAnalysisStreamEvent =
  | {
    type: "delta";
    text: string;
  }
  | {
    type: "done";
  }
  | {
    type: "error";
    message: string;
  };

export type EmailAnalysisStreamState = {
  status: "idle" | "streaming" | "done" | "error";
  text: string;
  error: string | null;
};

export type EmailRecommendation = {
  priority: "high" | "medium" | "low";
  title: string;
  details: string;
};

export type AIAnalysisLogItem = {
  id: string;
  email_id: string | null;
  email_title: string | null;
  email_slug: string | null;
  language: string | null;
  variant: string | null;
  user_id: string | null;
  user_email: string | null;
  model: string;
  status: string;
  cache_status: "hit" | "miss" | "unknown";
  latency_ms: number;
  input_tokens: number | null;
  output_tokens: number | null;
  total_tokens: number | null;
  cached_tokens: number | null;
  error_message: string | null;
  created_at: string;
};

export type AIAnalysisLogFilters = {
  status?: string;
  cache_status?: string;
  email_id?: string;
  model?: string;
  limit?: string;
};

export type AuthUser = {
  email: string;
  role: UserRole;
};

export type UserRole = "super_admin" | "admin" | "reviewer";

export type UserAdminItem = {
  id: string;
  email: string;
  role: UserRole;
  created_at: string;
  updated_at: string;
  last_seen_at: string | null;
};

export type CreateAdminUserPayload = {
  email: string;
  role: UserRole;
};

export type AuthEventItem = {
  id: string;
  user_id: string | null;
  email: string;
  event_type: string;
  success: boolean;
  ip_address: string | null;
  user_agent: string | null;
  created_at: string;
};

export type AuthEventFilters = {
  email?: string;
  event_type?: string;
  success?: string;
  limit?: string;
};

export type OperationalEventItem = {
  id: string;
  level: "info" | "warn" | "error";
  event_type: string;
  message: string;
  user_id: string | null;
  user_email: string | null;
  request_id: string | null;
  method: string | null;
  path: string | null;
  status_code: number | null;
  duration_ms: number | null;
  metadata: Record<string, unknown>;
  created_at: string;
};

export type OperationalEventFilters = {
  level?: string;
  event_type?: string;
  user_email?: string;
  request_id?: string;
  path?: string;
  limit?: string;
};

export type EmailEventAction =
  | "email_duplicated"
  | "email_adaptation_created"
  | "email_created"
  | "email_archived"
  | "email_updated"
  | "email_planning_updated"
  | "email_review_status_updated"
  | "email_area_approval_updated"
  | "comment_created"
  | "comment_replied"
  | "comment_resolved"
  | "board_created"
  | "board_stage_created"
  | "board_stage_renamed"
  | "board_stage_deleted"
  | "board_stages_reordered"
  | "board_approval_area_created"
  | "board_approval_area_updated"
  | "board_approval_area_deleted"
  | "board_approval_areas_reordered";

export type EmailEventItem = {
  id: string;
  actor_user_id: string | null;
  actor_email: string;
  action: EmailEventAction;
  email_id: string | null;
  email_slug: string | null;
  email_title: string | null;
  metadata: Record<string, unknown>;
  changes: Record<string, unknown>;
  created_at: string;
};

export type EmailActivityType =
  | "comment_created"
  | "comment_replied"
  | "comment_resolved"
  | "email_created"
  | "email_updated"
  | "email_planning_updated"
  | "email_review_status_updated"
  | "email_duplicated"
  | "email_adaptation_created"
  | "email_archived"
  | "ai_analysis_run";

export type EmailActivityItem = {
  id: string;
  type: EmailActivityType;
  actor_email: string | null;
  created_at: string;
  summary: string;
  metadata: Record<string, unknown>;
  changes: Record<string, unknown>;
};

export type EmailEventFilters = {
  actor_email?: string;
  action?: string;
  email?: string;
  limit?: string;
};

export type EmailVersionGroup = {
  key: string;
  stage: string;
  sortOrder: number;
  versions: EmailListItem[];
};

export type StageColumn = {
  stage: string;
  title: string;
  sortOrder: number;
  emailGroups: EmailVersionGroup[];
};

export type EmailComment = {
  id: string;
  email_id: string;
  user_id: string | null;
  author_email: string | null;
  review_block: string;
  selected_text: string;
  start_offset: number;
  end_offset: number;
  body: string;
  status: "open" | "resolved";
  severity: EmailCommentSeverity;
  created_at: string;
  resolved_at: string | null;
  resolved_by: string | null;
  resolved_by_email: string | null;
  messages: EmailCommentMessage[];
};

export type EmailCommentMessage = {
  id: string;
  comment_id: string;
  user_id: string | null;
  author_email: string | null;
  body: string;
  created_at: string;
  updated_at: string;
};

export type CreateEmailCommentPayload = {
  review_block: string;
  selected_text: string;
  start_offset: number;
  end_offset: number;
  body: string;
  severity: EmailCommentSeverity;
};

export type CreateCommentMessagePayload = {
  body: string;
};
