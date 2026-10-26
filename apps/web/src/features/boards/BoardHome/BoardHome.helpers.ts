import { notifications } from "@mantine/notifications";
import type { QueryClient } from "@tanstack/react-query";

import {
  formatEmailReviewStatus,
  isApprovedEmailReviewStatus,
  needsBitrixUpload,
} from "../../emails/reviewStatus";
import { formatStageName } from "../../emails/stages";
import { matchesProductionFilter } from "../../emails/productionStatus";
import type { BoardRegion, EmailDetail, EmailListItem, EmailReviewStatus, StageColumn } from "../../emails/types";

import {
  noRegionFilter,
  type BoardEmailState,
  type BoardFilterCounts,
  type BoardFilterOptions,
  type BoardFilters,
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
  const mine = filters.mine.toLowerCase();
  return (
    (!mine ||
      email.owner_email?.trim().toLowerCase() === mine ||
      email.reviewer_email?.trim().toLowerCase() === mine) &&
    (!filters.openComments || hasOpenComments(email)) &&
    (!filters.staleTranslation || email.translation_stale) &&
    (!filters.needsUpload || needsBitrixUpload(email)) &&
    (filters.languages.length === 0 || filters.languages.includes(email.language)) &&
    (filters.states.length === 0 ||
      filters.states.some((state) => emailHasState(email, state))) &&
    (!filters.owner || assigneeFilterValue(email.owner_email) === filters.owner) &&
    (!filters.variant || email.variant === filters.variant) &&
    (!filters.adaptation || email.adaptation_key === filters.adaptation) &&
    (!filters.region ||
      (filters.region === noRegionFilter ? !email.region_key : email.region_key === filters.region))
  );
}


function hasOpenComments(email: EmailListItem) {
  return (email.open_comment_count ?? 0) > 0;
}

function emailHasState(email: EmailListItem, state: BoardEmailState) {
  return state === "live"
    ? matchesProductionFilter(email.production_status, "live")
    : email.review_status === state;
}


export function getActiveBoardFilterCount(filters: BoardFilters) {
  return (
    [
      filters.mine,
      filters.openComments,
      filters.staleTranslation,
      filters.needsUpload,
      filters.owner,
      filters.variant,
      filters.adaptation,
      filters.region,
    ].filter(Boolean).length +
    filters.languages.length +
    filters.states.length
  );
}


export function getBoardFilterCounts(emails: EmailListItem[]): BoardFilterCounts {
  return {
    openComments: emails.filter(hasOpenComments).length,
    staleTranslation: emails.filter((email) => email.translation_stale).length,
    needsUpload: emails.filter(needsBitrixUpload).length,
  };
}


// getBoardFilterOptions lists what the board's emails can be filtered by. On
// a board with regions the region replaces the adaptation, which follows it.
export function getBoardFilterOptions(emails: EmailListItem[], regions: BoardRegion[] = []): BoardFilterOptions {
  const adaptationByKey = new Map<string, string>();

  for (const email of emails) {
    if (!adaptationByKey.has(email.adaptation_key)) {
      adaptationByKey.set(email.adaptation_key, email.adaptation_label);
    }
  }

  const variants = uniqueSorted(emails.map((email) => email.variant));

  return {
    // EN is the master, so it goes first.
    languages: uniqueSorted(emails.map((email) => email.language))
      .sort((first, second) => Number(second === "en") - Number(first === "en"))
      .map((language) => ({
        label: language.toUpperCase(),
        value: language,
      })),
    regions: regions.length
      ? [
          ...[...regions]
            .sort((first, second) => Number(second.reference) - Number(first.reference))
            .map((region) => ({ label: region.name, value: region.key })),
          ...(emails.some((email) => !email.region_key) ? [{ label: "No region", value: noRegionFilter }] : []),
        ]
      : [],
    adaptations: (regions.length ? [] : Array.from(adaptationByKey.entries()))
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
  const readyCount = emails.filter((email) =>
    isApprovedEmailReviewStatus(email.review_status)
  ).length;
  const openBlockingCommentCount = emails.reduce(
    (total, email) => total + email.open_blocking_comment_count,
    0
  );

  return {
    ready_count: readyCount,
    board_key: boardKey,
    board_name: boardName,
    email_count: emails.length,
    emails,
    open_blocking_comment_count: openBlockingCommentCount,
    ready_for_handoff:
      emails.length > 0 &&
      readyCount === emails.length &&
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
