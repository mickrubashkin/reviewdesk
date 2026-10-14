import { notifications } from "@mantine/notifications";
import type { QueryClient } from "@tanstack/react-query";

import {
  formatEmailReviewStatus,
  isProductionApprovedEmailReviewStatus,
} from "../../emails/reviewStatus";
import { formatStageName } from "../../emails/stages";
import { matchesProductionFilter } from "../../emails/productionStatus";
import type { EmailDetail, EmailListItem, EmailReviewStatus, StageColumn } from "../../emails/types";

import type {
  BoardFilterOptions,
  BoardFilters,
  HandoffFilters,
} from "./BoardHome.types";

export function filterColumnsBySearch(columns: StageColumn[], query: string) {
  const terms = normalizeSearchText(query).split(" ").filter(Boolean);
  if (terms.length === 0) {
    return columns;
  }

  return columns
    .map((column) => ({
      ...column,
      emailGroups: column.emailGroups.filter((group) =>
        group.versions.some((email) => emailMatchesSearch(email, column, terms))
      ),
    }))
    .filter((column) => column.emailGroups.length > 0);
}


export function applyReviewStatusToBoardCaches(
  queryClient: QueryClient,
  boardKey: string,
  emailId: string,
  reviewStatus: EmailReviewStatus
) {
  const updateList = (currentEmails: EmailListItem[] | undefined) =>
    currentEmails?.map((currentEmail) =>
      currentEmail.id === emailId
        ? { ...currentEmail, review_status: reviewStatus }
        : currentEmail
    );

  queryClient.setQueryData<EmailListItem[]>(["emails"], updateList);
  queryClient.setQueryData<EmailListItem[]>(["emails", boardKey], updateList);
  queryClient.setQueryData<EmailDetail>(
    ["emails", emailId, "review"],
    (currentEmail) =>
      currentEmail
        ? { ...currentEmail, review_status: reviewStatus }
        : currentEmail
  );
}


export function filterColumnsByBoardFilters(
  columns: StageColumn[],
  filters: BoardFilters
) {
  if (getActiveBoardFilterCount(filters) === 0) {
    return columns;
  }

  return columns
    .map((column) => ({
      ...column,
      emailGroups: column.emailGroups
        .map((group) => ({
          ...group,
          versions: group.versions.filter((email) =>
            emailMatchesBoardFilters(email, filters)
          ),
        }))
        .filter((group) => group.versions.length > 0),
    }))
    .filter((column) => column.emailGroups.length > 0);
}


function emailMatchesBoardFilters(email: EmailListItem, filters: BoardFilters) {
  return (
    (filters.comments === "all" || (email.open_comment_count ?? 0) > 0) &&
    (!filters.language || email.language === filters.language) &&
    (!filters.adaptation || email.adaptation_key === filters.adaptation) &&
    (!filters.variant || email.variant === filters.variant) &&
    (!filters.owner || assigneeFilterValue(email.owner_email) === filters.owner) &&
    (!filters.reviewer ||
      assigneeFilterValue(email.reviewer_email) === filters.reviewer) &&
    (!filters.status || email.review_status === filters.status) &&
    matchesProductionFilter(email.production_status, filters.production) &&
    (!filters.translation || (filters.translation === "stale" && email.translation_stale))
  );
}


export function getActiveBoardFilterCount(filters: BoardFilters) {
  return [
    filters.comments === "open" ? filters.comments : "",
    filters.language,
    filters.adaptation,
    filters.variant,
    filters.owner,
    filters.reviewer,
    filters.status,
    filters.production,
    filters.translation,
  ].filter(Boolean).length;
}


export function getBoardFilterOptions(emails: EmailListItem[]): BoardFilterOptions {
  const adaptationByKey = new Map<string, string>();

  for (const email of emails) {
    if (!adaptationByKey.has(email.adaptation_key)) {
      adaptationByKey.set(email.adaptation_key, email.adaptation_label);
    }
  }

  const variants = uniqueSorted(emails.map((email) => email.variant));

  return {
    languages: uniqueSorted(emails.map((email) => email.language)).map((language) => ({
      label: language.toUpperCase(),
      value: language,
    })),
    adaptations: Array.from(adaptationByKey.entries())
      .map(([value, label]) => ({ label, value }))
      .sort((first, second) => {
        if (first.value === "default") {
          return -1;
        }
        if (second.value === "default") {
          return 1;
        }
        return first.label.localeCompare(second.label);
      }),
    variants: variants.map((variant) => ({
      label: formatVariantOptionLabel(variant, variants),
      value: variant,
    })),
    owners: assigneeFilterOptions(emails.map((email) => email.owner_email)),
    reviewers: assigneeFilterOptions(emails.map((email) => email.reviewer_email)),
  };
}


function assigneeFilterOptions(values: Array<string | null>) {
  return uniqueSorted(values.map(assigneeFilterValue)).map((value) => ({
    label: value === "__unassigned__" ? "Unassigned" : value,
    value,
  }));
}


function assigneeFilterValue(value: string | null) {
  return value?.trim() || "__unassigned__";
}


function uniqueSorted(values: string[]) {
  return Array.from(new Set(values.filter(Boolean))).sort((first, second) =>
    first.localeCompare(second)
  );
}


function formatVariantLabel(variant: string) {
  const normalizedVariant = variant.trim().toLowerCase();
  if (normalizedVariant === "old") {
    return "v0";
  }
  if (normalizedVariant === "new") {
    return "v1";
  }

  return variant;
}


function formatVariantOptionLabel(variant: string, variants: string[]) {
  const label = formatVariantLabel(variant);
  const hasLabelCollision = variants.some(
    (otherVariant) =>
      otherVariant !== variant && formatVariantLabel(otherVariant) === label
  );
  const normalizedVariant = variant.trim().toLowerCase();

  if (
    hasLabelCollision &&
    (normalizedVariant === "old" || normalizedVariant === "new")
  ) {
    return `${label} legacy`;
  }

  return label;
}


function emailMatchesSearch(
  email: EmailListItem,
  column: StageColumn,
  terms: string[]
) {
  const searchableText = normalizeSearchText(
    [
      email.title,
      email.subject,
      email.preheader,
      email.send_timing,
      email.stage,
      column.title,
      formatStageName(email.stage ?? column.stage),
      email.language,
      email.variant,
      email.adaptation_label,
      email.adaptation_key,
      email.review_status,
      formatEmailReviewStatus(email.review_status),
      email.sequence,
    ]
      .filter(Boolean)
      .join(" ")
  );

  return terms.every((term) => searchableText.includes(term));
}


function normalizeSearchText(value: string) {
  return value.trim().toLowerCase().replace(/\s+/g, " ");
}


export function filterEmailsByHandoffFilters(
  emails: EmailListItem[],
  filters: HandoffFilters
) {
  const languageSet = new Set(filters.languages);
  const adaptationSet = new Set(filters.adaptations);
  const stageSet = new Set(filters.stages);

  return emails.filter((email) => {
    if (languageSet.size > 0 && !languageSet.has(email.language)) {
      return false;
    }
    if (
      adaptationSet.size > 0 &&
      !adaptationSet.has(email.adaptation_key)
    ) {
      return false;
    }
    if (stageSet.size > 0 && !stageSet.has(email.stage ?? "")) {
      return false;
    }
    return true;
  });
}


export function getActiveHandoffFilterCount(filters: HandoffFilters) {
  return (
    filters.adaptations.length +
    filters.languages.length +
    filters.stages.length
  );
}


export function buildSequenceHandoffManifest(
  boardKey: string,
  boardName: string,
  sequenceEmails: EmailListItem[]
) {
  const emails = sequenceEmails.map((email, index) => ({
    adaptation: email.adaptation_label,
    due_date: email.due_date,
    id: email.id,
    implementation_notes: email.implementation_notes,
    language: email.language,
    open_blocking_comment_count: email.open_blocking_comment_count,
    open_comment_count: email.open_comment_count,
    order: index + 1,
    preheader: email.preheader,
    review_status: email.review_status,
    send_timing: email.send_timing,
    stage: email.stage,
    subject: email.subject,
    title: email.title,
    variant: email.variant,
  }));
  const productionApprovedCount = emails.filter((email) =>
    isProductionApprovedEmailReviewStatus(email.review_status)
  ).length;
  const openBlockingCommentCount = emails.reduce(
    (total, email) => total + email.open_blocking_comment_count,
    0
  );

  return {
    approved_count: productionApprovedCount,
    production_approved_count: productionApprovedCount,
    board_key: boardKey,
    board_name: boardName,
    email_count: emails.length,
    emails,
    open_blocking_comment_count: openBlockingCommentCount,
    ready_for_handoff:
      emails.length > 0 &&
      productionApprovedCount === emails.length &&
      openBlockingCommentCount === 0,
  };
}

export async function copyPlainText(value: string, label: string) {
  try {
    await navigator.clipboard.writeText(value);
    notifications.show({
      color: "green",
      message: `${label} copied to clipboard.`,
      title: "Copied",
    });
  } catch {
    notifications.show({
      color: "red",
      message: "Browser blocked clipboard access.",
      title: "Copy failed",
    });
  }
}


export function downloadJSON(json: string, fileName: string) {
  const blob = new Blob([json], {
    type: "application/json;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");

  link.href = url;
  link.download = sanitizeDownloadFileName(fileName);
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);

  notifications.show({
    color: "green",
    message: `${link.download} is ready.`,
    title: "Downloaded",
  });
}


function sanitizeDownloadFileName(fileName: string) {
  return (
    fileName
      .toLowerCase()
      .trim()
      .replace(/\s+/g, "-")
      .replace(/[^a-z0-9_.-]+/g, "-")
      .replace(/-+/g, "-")
      .replace(/^-|-$/g, "") || "handoff.json"
  );
}
