import type { QueryClient } from "@tanstack/react-query";

import type { InlineEditUpdate } from "../../emails/MailPreview.types";
import {
  formatEmailUpdateChangedFields,
  formatEmailUpdateChangedReviewBlocks,
  formatEmailUpdateSummary,
} from "../../emails/changeSummary";
import type {
  ReviewChangedBlockReason,
  ReviewChangedBlockTarget,
  ReviewCommentTarget,
} from "../../emails/reviewOverlayTypes";
import {
  formatEmailReviewStatus,
  isApprovedEmailReviewStatus,
} from "../../emails/reviewStatus";
import { getVersionForVariant } from "../../emails/stages";
import type {
  EmailActivityItem,
  EmailComment,
  EmailCommentSeverity,
  EmailDetail,
  EmailListItem,
  EmailReviewStatus,
  EmailVariant,
  EmailVersionGroup,
  UpdateEditableFieldsPayload,
  UpdateEmailPlanningFieldsPayload,
} from "../../emails/types";

import type { CommentBlockOption, ReviewUtilityPanel } from "./EmailReview.types";

export function clampPercent(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}

export function nullableTrimmed(value: string) {
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export function formatAreaApprovalStatus(status: string) {
  switch (status) {
    case "pending":
      return "Pending";
    case "approved":
      return "Approved";
    case "changes_requested":
      return "Changes requested";
    case "stale":
      return "Stale";
    default:
      return status.replaceAll("_", " ");
  }
}

export function buildApprovalBlockedMessage(
  openBlockingCommentCount: number,
  incompleteRequiredApprovalCount: number
) {
  const blockers: string[] = [];

  if (openBlockingCommentCount > 0) {
    blockers.push("Resolve blocking comments");
  }
  if (incompleteRequiredApprovalCount > 0) {
    blockers.push("complete required approvals");
  }

  if (blockers.length === 0) {
    return "";
  }

  return `${blockers.join(" and ")} before approval.`;
}

export function areaApprovalStatusColor(status: string) {
  switch (status) {
    case "approved":
      return "green";
    case "changes_requested":
      return "yellow";
    case "stale":
      return "orange";
    case "pending":
      return "gray";
    default:
      return "gray";
  }
}

export function findNeighborEmail(
  groups: EmailVersionGroup[],
  variant: EmailVariant,
  preferredLanguage: string | undefined,
  preferredAdaptation: string
) {
  for (const group of groups) {
    const email = getVersionForVariant(
      group.versions,
      variant,
      preferredLanguage,
      preferredAdaptation
    );
    if (email) {
      return email;
    }
  }

  return undefined;
}

export function formatEmailTitle(title: string) {
  switch (title) {
    case "Follow Up 1":
      return "First Follow-up";
    case "Follow Up 2":
      return "Second Follow-up";
    default:
      return title;
  }
}

export function formatUtilityPanelLabel(tab: ReviewUtilityPanel) {
  switch (tab) {
    case "planning":
      return "Plan";
    case "approvals":
      return "Approvals";
    case "handoff":
      return "Handoff";
    case "history":
      return "History";
    case "activity":
      return "Activity";
  }
}

export function applyReviewStatusToCaches(
  queryClient: QueryClient,
  emailId: string,
  reviewStatus: EmailReviewStatus,
  sequence: string | undefined
) {
  const updateList = (currentEmails: EmailListItem[] | undefined) =>
    currentEmails?.map((currentEmail) =>
      currentEmail.id === emailId
        ? { ...currentEmail, review_status: reviewStatus }
        : currentEmail
    );

  queryClient.setQueryData<EmailListItem[]>(["emails"], updateList);
  if (sequence) {
    queryClient.setQueryData<EmailListItem[]>(["emails", sequence], updateList);
  }
  queryClient.setQueryData<EmailDetail>(
    ["emails", emailId, "review"],
    (currentEmail) =>
      currentEmail
        ? { ...currentEmail, review_status: reviewStatus }
        : currentEmail
  );
}

export function applyPlanningFieldsToCaches(
  queryClient: QueryClient,
  emailId: string,
  planningFields: UpdateEmailPlanningFieldsPayload,
  sequence: string | undefined
) {
  queryClient.setQueryData<EmailDetail>(["emails", emailId, "review"], (currentEmail) =>
    currentEmail?.id === emailId
      ? { ...currentEmail, ...planningFields }
      : currentEmail
  );
  if (sequence) {
    queryClient.setQueryData<EmailListItem[]>(["emails", sequence], (emails) =>
      emails?.map((listEmail) =>
        listEmail.id === emailId ? { ...listEmail, ...planningFields } : listEmail
      )
    );
  }
}

export function buildEditableFieldsPayload(
  email: EmailDetail,
  update: InlineEditUpdate,
  originalHTML?: string
): UpdateEditableFieldsPayload {
  return {
    title: email.title,
    subject: update.subject ?? email.subject ?? "",
    preheader: update.preheader ?? email.preheader ?? "",
    editable_fields: {
      ...email.editable_fields,
      ...(update.editableFields ?? {}),
    },
    ...(originalHTML !== undefined ? { original_html: originalHTML } : {}),
  };
}

export type ReviewBlockFreshness = {
  changedAfterApprovalBlocks: Set<string>;
  changedAfterCommentBlocks: Set<string>;
  changedAfterCommentCommentIds: Set<string>;
};

export function buildReviewBlockFreshness(
  comments: EmailComment[],
  activities: EmailActivityItem[]
): ReviewBlockFreshness {
  const changedAfterApprovalBlocks = new Set<string>();
  const changedAfterCommentBlocks = new Set<string>();
  const changedAfterCommentCommentIds = new Set<string>();
  const latestApprovalAt = latestApprovalTimestamp(activities);
  const commentsByBlock = new Map<string, EmailComment[]>();

  comments.forEach((comment) => {
    if (comment.status !== "open") {
      return;
    }

    commentsByBlock.set(comment.review_block, [
      ...(commentsByBlock.get(comment.review_block) ?? []),
      comment,
    ]);
  });

  activities
    .filter((activity) => activity.type === "email_updated")
    .forEach((activity) => {
      const updatedAt = timestamp(activity.created_at);
      if (updatedAt === null) {
        return;
      }

      const changedBlocks = stringArrayMetadata(
        activity.metadata,
        "changed_review_blocks"
      );
      changedBlocks.forEach((reviewBlock) => {
        if (latestApprovalAt !== null && updatedAt > latestApprovalAt) {
          changedAfterApprovalBlocks.add(reviewBlock);
        }

        for (const comment of commentsByBlock.get(reviewBlock) ?? []) {
          const commentCreatedAt = timestamp(comment.created_at);
          if (commentCreatedAt !== null && updatedAt > commentCreatedAt) {
            changedAfterCommentBlocks.add(reviewBlock);
            changedAfterCommentCommentIds.add(comment.id);
          }
        }
      });
    });

  return {
    changedAfterApprovalBlocks,
    changedAfterCommentBlocks,
    changedAfterCommentCommentIds,
  };
}

export function buildChangedBlockTargets({
  changedAfterApprovalBlocks,
  changedAfterCommentBlocks,
}: ReviewBlockFreshness): ReviewChangedBlockTarget[] {
  const reviewBlocks = new Set([
    ...changedAfterApprovalBlocks,
    ...changedAfterCommentBlocks,
  ]);

  return Array.from(reviewBlocks)
    .sort()
    .map((reviewBlock) => ({
      reason: changedBlockReason(
        changedAfterCommentBlocks.has(reviewBlock),
        changedAfterApprovalBlocks.has(reviewBlock)
      ),
      reviewBlock,
    }));
}

function changedBlockReason(
  changedAfterComment: boolean,
  changedAfterApproval: boolean
): ReviewChangedBlockReason {
  if (changedAfterComment && changedAfterApproval) {
    return "comment_and_approval";
  }
  return changedAfterApproval ? "approval" : "comment";
}

function latestApprovalTimestamp(activities: EmailActivityItem[]) {
  const approvalTimes = activities
    .filter((activity) => activity.type === "email_review_status_updated")
    .filter((activity) => {
      const reviewStatus = activity.changes.review_status;
      return (
        isRecord(reviewStatus) &&
        typeof reviewStatus.after === "string" &&
        isApprovedEmailReviewStatus(reviewStatus.after)
      );
    })
    .map((activity) => timestamp(activity.created_at))
    .filter((value): value is number => value !== null);

  return approvalTimes.length > 0 ? Math.max(...approvalTimes) : null;
}

export function latestApprovalActivity(activities: EmailActivityItem[]) {
  return (
    activities
      .filter((activity) => activity.type === "email_review_status_updated")
      .filter((activity) => {
        const reviewStatus = activity.changes.review_status;
        return (
        isRecord(reviewStatus) &&
        typeof reviewStatus.after === "string" &&
        isApprovedEmailReviewStatus(reviewStatus.after)
      );
      })
      .sort((left, right) => {
        const leftTime = timestamp(left.created_at) ?? 0;
        const rightTime = timestamp(right.created_at) ?? 0;
        return rightTime - leftTime;
      })[0] ?? null
  );
}

function stringArrayMetadata(metadata: Record<string, unknown>, key: string) {
  const value = metadata[key];
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function timestamp(value: string) {
  const parsed = new Date(value).getTime();
  return Number.isNaN(parsed) ? null : parsed;
}

export function commentToTarget(
  comment: EmailComment,
  staleAfterEdit: boolean
): ReviewCommentTarget {
  return {
    authorKey: comment.author_email ?? comment.user_id ?? "unknown",
    id: comment.id,
    reviewBlock: comment.review_block,
    selectedText: comment.selected_text,
    startOffset: comment.start_offset,
    endOffset: comment.end_offset,
    staleAfterEdit,
    status: comment.status,
    severity: comment.severity,
  };
}

export function buildCommentBlockOptions(
  email: EmailDetail | undefined
): CommentBlockOption[] {
  if (!email) {
    return [];
  }

  const options: CommentBlockOption[] = [];
  const seenBlocks = new Set<string>();
  const addOption = (reviewBlock: string, selectedText: string) => {
    const normalizedText = selectedText.trim();
    if (!normalizedText || seenBlocks.has(reviewBlock)) {
      return;
    }

    seenBlocks.add(reviewBlock);
    options.push({
      label: formatReviewBlockOptionLabel(reviewBlock, normalizedText),
      reviewBlock,
      selectedText: normalizedText,
      value: reviewBlock,
    });
  };

  addOption("subject", email.subject ?? email.title);
  if (email.preheader) {
    addOption("preheader", email.preheader);
  }

  if (typeof DOMParser === "undefined") {
    return options;
  }

  const document = new DOMParser().parseFromString(
    email.review_html || email.original_html,
    "text/html"
  );
  document.querySelectorAll("[data-review-block]").forEach((block) => {
    if (block.querySelector("[data-review-block]")) {
      return;
    }

    const reviewBlock = block.getAttribute("data-review-block");
    if (reviewBlock) {
      addOption(reviewBlock, getReviewBlockOptionText(block));
    }
  });

  return options;
}

function getReviewBlockOptionText(block: Element) {
  const text = block.textContent?.trim();
  if (text) {
    return text;
  }

  const image = block.matches("img") ? block : block.querySelector("img");
  if (!image) {
    return "";
  }

  const imageLabel =
    image.getAttribute("alt")?.trim() ||
    image.getAttribute("aria-label")?.trim() ||
    image.getAttribute("title")?.trim();
  if (imageLabel) {
    return imageLabel;
  }

  const imageSource = image.getAttribute("src")?.trim();
  if (imageSource) {
    return `Image: ${imageSource.split("/").pop() ?? imageSource}`;
  }

  return `Image: ${block.getAttribute("data-review-block") ?? "review block"}`;
}

function formatReviewBlockOptionLabel(reviewBlock: string, selectedText: string) {
  const label = formatReviewBlockLabel(reviewBlock);
  const preview = selectedText.replace(/\s+/g, " ").slice(0, 44);
  return `${label} - ${preview}${selectedText.length > 44 ? "..." : ""}`;
}

export function formatCommentDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export function formatActivityType(activity: EmailActivityItem) {
  switch (activity.type) {
    case "comment_created":
      return "Comment";
    case "comment_replied":
      return "Reply";
    case "comment_resolved":
      return "Resolved";
    case "email_created":
      return "Created";
    case "email_updated":
      return "Edited";
    case "email_planning_updated":
      return "Planning";
    case "email_review_status_updated":
      if (isStaleApprovalActivity(activity)) {
        return "Approval stale";
      }
      if (isReapprovalActivity(activity)) {
        return "Re-approved";
      }
      return "Status";
    case "email_duplicated":
      return "Duplicate";
    case "email_adaptation_created":
      return "Adaptation";
    case "email_archived":
      return "Archive";
    case "ai_analysis_run":
      return "AI";
  }
}

export function activityColor(type: EmailActivityItem["type"]) {
  switch (type) {
    case "comment_created":
    case "email_updated":
      return "yellow";
    case "email_planning_updated":
      return "cyan";
    case "comment_replied":
    case "email_duplicated":
      return "blue";
    case "comment_resolved":
    case "email_created":
      return "green";
    case "email_review_status_updated":
      return "violet";
    case "email_adaptation_created":
      return "teal";
    case "email_archived":
      return "red";
    case "ai_analysis_run":
      return "grape";
  }
}

export function activityDetail(activity: EmailActivityItem) {
  switch (activity.type) {
    case "comment_created":
    case "comment_resolved":
      return joinActivityParts([
        metadataText(activity, "severity"),
        metadataText(activity, "review_block"),
      ]);
    case "comment_replied":
      return metadataText(activity, "review_block");
    case "email_review_status_updated":
      return joinActivityParts([
        formatReviewStatusChange(activity),
        formatApprovalSnapshot(activity),
      ]);
    case "email_updated":
      return joinActivityParts([
        formatActivityChangedFields(activity.changes),
        formatEmailUpdateChangedReviewBlocks(activity.metadata),
      ]);
    case "email_planning_updated":
      return formatPlanningChangedFields(activity.changes);
    case "email_adaptation_created":
      return metadataText(activity, "adaptation_label");
    case "ai_analysis_run":
      return joinActivityParts([
        metadataText(activity, "status"),
        metadataText(activity, "model"),
        metadataNumber(activity, "latency_ms"),
      ]);
    default:
      return "";
  }
}

export function activitySummary(activity: EmailActivityItem) {
  if (isStaleApprovalActivity(activity)) {
    return "Approval became stale after edit";
  }
  if (isReapprovalActivity(activity)) {
    return metadataText(activity, "reason") === "reapproved_after_stale_edit"
      ? "Re-approved after stale edit"
      : "Re-approved email";
  }
  if (activity.type === "email_updated") {
    return formatEmailUpdateSummary(activity.changes);
  }
  if (activity.type === "email_planning_updated") {
    return "Updated planning fields";
  }
  return activity.summary;
}

function formatReviewStatusChange(activity: EmailActivityItem) {
  const reviewStatus = activity.changes.review_status;
  if (!isRecord(reviewStatus)) {
    return "";
  }
  const before =
    typeof reviewStatus.before === "string"
      ? formatEmailReviewStatus(reviewStatus.before as EmailReviewStatus)
      : "";
  const after =
    typeof reviewStatus.after === "string"
      ? formatEmailReviewStatus(reviewStatus.after as EmailReviewStatus)
      : "";
  return joinActivityParts([before, after ? `to ${after}` : ""]);
}

function formatApprovalSnapshot(activity: EmailActivityItem) {
  const contentHash = metadataText(activity, "approved_content_hash");
  if (!contentHash) {
    return "";
  }

  return `Content snapshot ${shortHash(contentHash)}`;
}

function isStaleApprovalActivity(activity: EmailActivityItem) {
  return (
    activity.type === "email_review_status_updated" &&
    metadataText(activity, "reason") === "approval_stale_after_edit"
  );
}

function isReapprovalActivity(activity: EmailActivityItem) {
  const reason = metadataText(activity, "reason");
  return (
    activity.type === "email_review_status_updated" &&
    (reason === "reapproved" || reason === "reapproved_after_stale_edit")
  );
}

function formatActivityChangedFields(changes: Record<string, unknown>) {
  return formatEmailUpdateChangedFields(changes);
}

function formatPlanningChangedFields(changes: Record<string, unknown>) {
  const labels: Record<string, string> = {
    adaptation_label: "Adaptation",
    due_date: "Due date",
    implementation_notes: "Implementation notes",
    owner_email: "Owner",
    reviewer_email: "Reviewer",
    send_timing: "Send timing",
    send_condition: "Send condition",
  };
  return Object.keys(changes)
    .map((field) => labels[field] ?? field)
    .join(", ");
}

function metadataText(activity: EmailActivityItem, key: string) {
  const value = activity.metadata[key];
  return typeof value === "string" ? value : "";
}

function metadataNumber(activity: EmailActivityItem, key: string) {
  const value = activity.metadata[key];
  return typeof value === "number" ? `${value}ms` : "";
}

function shortHash(value: string) {
  return value.length > 10 ? value.slice(0, 10) : value;
}

function joinActivityParts(parts: string[]) {
  return parts.filter(Boolean).join(" · ");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

export function formatCommentSeverity(severity: EmailCommentSeverity) {
  switch (severity) {
    case "suggestion":
      return "Suggestion";
    case "blocking":
      return "Blocking";
    case "issue":
      return "Issue";
  }
}

export function commentSeverityColor(severity: EmailCommentSeverity) {
  switch (severity) {
    case "suggestion":
      return "blue";
    case "blocking":
      return "red";
    case "issue":
      return "yellow";
  }
}

export function getReviewTargetLabel(comment: EmailComment) {
  const reviewBlock = comment.review_block;
  if (reviewBlock === "subject") {
    return "Subject";
  }

  if (reviewBlock === "preheader") {
    return "Preheader";
  }

  if (reviewBlock.startsWith("banner")) {
    return "Banner";
  }

  if (reviewBlock.startsWith("headline")) {
    return "Headline";
  }

  if (reviewBlock.startsWith("body")) {
    return isWholeBlockComment(comment) ? "Body block" : "Body text";
  }

  if (reviewBlock.startsWith("cta") || reviewBlock.startsWith("button")) {
    return "CTA";
  }

  if (reviewBlock.startsWith("footer")) {
    return "Footer";
  }

  return formatReviewBlockLabel(reviewBlock);
}

function isWholeBlockComment(comment: EmailComment) {
  return comment.start_offset === 0 && comment.end_offset >= comment.selected_text.length;
}

export function formatReviewBlockLabel(reviewBlock: string) {
  return reviewBlock
    .split(/[-_]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
