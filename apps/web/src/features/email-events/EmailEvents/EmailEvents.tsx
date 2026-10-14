import { useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Select,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";

import { AdminTable } from "../../admin-table/AdminTable";
import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import { useAdminTable } from "../../admin-table/useAdminTable";
import { fetchEmailEvents } from "../api";
import {
  formatEmailUpdateChangedFields,
  formatEmailUpdateChangedReviewBlocks,
  formatEmailUpdateSummary,
} from "../../emails/changeSummary";
import { formatEmailReviewStatus } from "../../emails/reviewStatus";
import type {
  EmailEventAction,
  EmailEventFilters,
  EmailEventItem,
} from "../../emails/types";
import styles from "../../auth-events/AuthEvents/AuthEvents.module.css";

const limitOptions = ["50", "100", "250", "500"];
const actionOptions: { value: EmailEventAction; label: string }[] = [
  { value: "email_updated", label: "Updated" },
  { value: "email_duplicated", label: "Duplicated" },
  { value: "email_adaptation_created", label: "Adaptation created" },
  { value: "email_archived", label: "Archived" },
  { value: "email_created", label: "Created" },
  { value: "email_planning_updated", label: "Planning updated" },
  { value: "email_review_status_updated", label: "Review status updated" },
  { value: "email_area_approval_updated", label: "Area approval updated" },
  { value: "comment_created", label: "Comment created" },
  { value: "comment_replied", label: "Comment replied" },
  { value: "comment_resolved", label: "Comment resolved" },
  { value: "board_created", label: "Board created" },
  { value: "board_stage_created", label: "Stage created" },
  { value: "board_stage_renamed", label: "Stage renamed" },
  { value: "board_stage_deleted", label: "Stage deleted" },
  { value: "board_stages_reordered", label: "Stages reordered" },
  { value: "board_approval_area_created", label: "Approval area created" },
  { value: "board_approval_area_updated", label: "Approval area updated" },
  { value: "board_approval_area_deleted", label: "Approval area deleted" },
  { value: "board_approval_areas_reordered", label: "Approval areas reordered" },
];
type SortDirection = "asc" | "desc";
type EmailEventSortKey =
  | "created_at"
  | "actor_email"
  | "action"
  | "email"
  | "summary"
  | "changed_fields";
const defaultSort = {
  direction: "desc" as SortDirection,
  key: "created_at" as EmailEventSortKey,
};
const sortKeys: readonly EmailEventSortKey[] = [
  "created_at",
  "actor_email",
  "action",
  "email",
  "summary",
  "changed_fields",
];
const emptyEmailEvents: EmailEventItem[] = [];

export function EmailEvents() {
  const [filters, setFilters] = useState<EmailEventFilters>({
    limit: "100",
  });

  const eventsQuery = useQuery({
    queryKey: ["email-events", filters],
    queryFn: () => fetchEmailEvents(filters),
    placeholderData: (previous) => previous,
  });

  const columns = useMemo(() => buildColumns(), []);
  const { table, rows } = useAdminTable({
    columns,
    data: eventsQuery.data ?? emptyEmailEvents,
    defaultSort,
    sortKeys,
    storageKey: "email-events",
  });
  const visibleFilters = useMemo(
    () => ({
      action: filters.action ?? "",
      actor_email: filters.actor_email ?? "",
      email: filters.email ?? "",
      limit: filters.limit ?? "100",
    }),
    [filters]
  );

  return (
    <div className={styles.page}>
      <AdminTableHeader
        subtitle="Admin changes to email content, boards, and lifecycle."
        title="Email events"
      />

      <section className={styles.filters}>
        <Group align="end" gap="sm">
          <TextInput
            label="Actor"
            placeholder="admin@example.com"
            value={visibleFilters.actor_email}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                actor_email: event.currentTarget.value,
              }))
            }
          />
          <Select
            clearable
            data={actionOptions}
            label="Action"
            placeholder="All actions"
            value={visibleFilters.action || null}
            onChange={(value) =>
              setFilters((current) => ({
                ...current,
                action: value ?? "",
              }))
            }
          />
          <TextInput
            label="Email"
            placeholder="Title or slug"
            value={visibleFilters.email}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                email: event.currentTarget.value,
              }))
            }
          />
          <Select
            data={limitOptions}
            label="Limit"
            value={visibleFilters.limit}
            onChange={(value) =>
              setFilters((current) => ({ ...current, limit: value ?? "100" }))
            }
          />
          <Button variant="light" onClick={() => setFilters({ limit: "100" })}>
            Reset
          </Button>
        </Group>
      </section>

      <section className={styles.tableShell}>
        {eventsQuery.isLoading ? (
          <Stack align="center" justify="center" h={240}>
            <Loader />
            <Text c="dimmed">Loading email events</Text>
          </Stack>
        ) : null}

        {eventsQuery.isError ? (
          <Alert color="red" title="Failed to load email events">
            Super admin access is required.
          </Alert>
        ) : null}

        {eventsQuery.isSuccess && rows.length === 0 ? (
          <Stack align="center" justify="center" h={240}>
            <Text fw={600}>No email events found</Text>
            <Text c="dimmed" size="sm">
              Try changing filters or editing an email.
            </Text>
          </Stack>
        ) : null}

        {eventsQuery.isSuccess && rows.length > 0 ? (
          <AdminTable table={table} />
        ) : null}
      </section>
    </div>
  );
}

export default EmailEvents;

function buildColumns(): ColumnDef<EmailEventItem>[] {
  return [
    {
      accessorKey: "created_at",
      cell: ({ row }) => formatDateTime(row.original.created_at),
      header: "Created",
      id: "created_at",
      maxSize: 260,
      minSize: 130,
      size: 170,
    },
    {
      accessorKey: "actor_email",
      cell: ({ row }) => row.original.actor_email,
      header: "Actor",
      id: "actor_email",
      maxSize: 360,
      minSize: 180,
      size: 240,
    },
    {
      accessorKey: "action",
      cell: ({ row }) => (
        <Badge color={getActionColor(row.original.action)} variant="light">
          {formatAction(row.original.action)}
        </Badge>
      ),
      header: "Action",
      id: "action",
      maxSize: 300,
      minSize: 160,
      size: 220,
    },
    {
      accessorFn: targetLabel,
      cell: ({ row }) => <TargetCell event={row.original} />,
      header: "Target",
      id: "email",
      maxSize: 420,
      minSize: 220,
      size: 280,
    },
    {
      accessorFn: formatSummary,
      cell: ({ row }) => <Text size="sm">{formatSummary(row.original)}</Text>,
      header: "Summary",
      id: "summary",
      maxSize: 620,
      minSize: 260,
      size: 360,
    },
    {
      accessorFn: formatChangedFields,
      cell: ({ row }) => (
        <Text c="dimmed" size="sm">
          {formatChangedFields(row.original)}
        </Text>
      ),
      header: "Changed fields",
      id: "changed_fields",
      maxSize: 620,
      minSize: 260,
      size: 360,
    },
  ];
}

function TargetCell({ event }: { event: EmailEventItem }) {
  return (
    <Stack gap={0}>
      <Text size="sm">
        {event.email_title || stringMetadata(event, "board_name") || "-"}
      </Text>
      <Text c="dimmed" className={styles.monoCell}>
        {event.email_slug ||
          event.email_id ||
          stringMetadata(event, "board_key") ||
          "-"}
      </Text>
    </Stack>
  );
}


function formatDateTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatAction(action: EmailEventAction) {
  return action.replace("email_", "").replace("board_", "").replaceAll("_", " ");
}

function getActionColor(action: EmailEventAction) {
  switch (action) {
    case "email_archived":
      return "red";
    case "email_duplicated":
      return "blue";
    case "email_adaptation_created":
      return "teal";
    case "email_created":
      return "green";
    case "email_updated":
      return "yellow";
    case "email_planning_updated":
      return "cyan";
    case "email_review_status_updated":
      return "violet";
    case "email_area_approval_updated":
      return "grape";
    case "comment_created":
      return "yellow";
    case "comment_replied":
      return "blue";
    case "comment_resolved":
      return "green";
    case "board_created":
      return "green";
    case "board_stage_created":
      return "cyan";
    case "board_stage_renamed":
      return "yellow";
    case "board_stage_deleted":
      return "red";
    case "board_stages_reordered":
      return "blue";
    case "board_approval_area_created":
      return "green";
    case "board_approval_area_updated":
      return "grape";
    case "board_approval_area_deleted":
      return "red";
    case "board_approval_areas_reordered":
      return "blue";
    default:
      return "gray";
  }
}

function formatSummary(event: EmailEventItem) {
  switch (event.action) {
    case "email_duplicated":
      return `Duplicated from ${stringMetadata(event, "source_slug") || "source email"}`;
    case "email_adaptation_created":
      return `Adaptation ${stringMetadata(event, "adaptation_label") || "created"} from ${stringMetadata(event, "source_slug") || "source email"}`;
    case "email_archived":
      return "Archived from the active board";
    case "email_created":
      return "Created email";
    case "email_updated":
      return formatEmailUpdateSummary(event.changes);
    case "email_planning_updated":
      return "Updated planning fields";
    case "email_review_status_updated":
      if (isStaleApprovalEvent(event)) {
        return "Approval became stale after edit";
      }
      if (isReapprovalEvent(event)) {
        return stringMetadata(event, "reason") === "reapproved_after_stale_edit"
          ? "Re-approved after stale edit"
          : "Re-approved email";
      }
      return `Changed review status to ${formatChangedReviewStatus(event)}`;
    case "email_area_approval_updated":
      return `Changed ${formatApprovalArea(event)} approval to ${formatAreaApprovalStatus(event)}`;
    case "comment_created":
      return `Added comment on ${stringMetadata(event, "review_block") || "review block"}`;
    case "comment_replied":
      return `Replied to comment on ${stringMetadata(event, "review_block") || "review block"}`;
    case "comment_resolved":
      return `Resolved comment on ${stringMetadata(event, "review_block") || "review block"}`;
    case "board_created":
      return `Created board ${stringMetadata(event, "board_name") || stringMetadata(event, "board_key") || "board"}`;
    case "board_stage_created":
      return `Created stage ${stringMetadata(event, "stage") || "stage"}`;
    case "board_stage_renamed":
      return `Renamed stage to ${formatChangedValue(event, "stage", "new stage")}`;
    case "board_stage_deleted":
      return `Deleted stage ${formatChangedValue(event, "stage", "stage")}`;
    case "board_stages_reordered":
      return "Reordered board stages";
    case "board_approval_area_created":
      return `Created approval area ${stringMetadata(event, "area_name") || stringMetadata(event, "area_key") || "area"}`;
    case "board_approval_area_updated":
      return `Updated approval area ${stringMetadata(event, "area_name") || stringMetadata(event, "area_key") || "area"}`;
    case "board_approval_area_deleted":
      return `Archived approval area ${stringMetadata(event, "area_name") || stringMetadata(event, "area_key") || "area"}`;
    case "board_approval_areas_reordered":
      return "Reordered approval areas";
    default:
      return "Changed email";
  }
}

function formatChangedFields(event: EmailEventItem) {
  if (
    event.action !== "email_updated" &&
    event.action !== "email_planning_updated" &&
    event.action !== "email_review_status_updated" &&
    event.action !== "email_area_approval_updated" &&
    !event.action.startsWith("board_")
  ) {
    return "-";
  }

  if (event.action === "email_updated") {
    return (
      joinEventParts([
        formatEmailUpdateChangedFields(event.changes),
        formatEmailUpdateChangedReviewBlocks(event.metadata),
      ]) || "-"
    );
  }

  const fields = Object.keys(event.changes);
  const editableFields = event.changes.editable_fields;
  if (isRecord(editableFields)) {
    const editableFieldsIndex = fields.indexOf("editable_fields");
    if (editableFieldsIndex >= 0) {
      fields.splice(editableFieldsIndex, 1);
    }
    fields.push(
      ...Object.keys(editableFields).map((field) => `editable_fields.${field}`)
    );
  }

  const changedFields = fields.length > 0 ? fields.join(", ") : "";
  return (
    joinEventParts([changedFields, formatApprovalSnapshot(event)]) || "-"
  );
}

function targetLabel(event: EmailEventItem) {
  return (
    event.email_title ||
    event.email_slug ||
    event.email_id ||
    stringMetadata(event, "board_name") ||
    stringMetadata(event, "board_key") ||
    ""
  );
}

function formatChangedValue(
  event: EmailEventItem,
  key: string,
  fallback: string
) {
  const value = event.changes[key];
  if (!isRecord(value)) {
    return fallback;
  }

  const nextValue = value.after ?? value.before;
  return typeof nextValue === "string" ? nextValue : fallback;
}

function formatChangedReviewStatus(event: EmailEventItem) {
  const reviewStatus = event.changes.review_status;
  if (!isRecord(reviewStatus)) {
    return "new value";
  }

  const nextStatus = reviewStatus.after;
  return typeof nextStatus === "string"
    ? formatEmailReviewStatus(nextStatus)
    : "new value";
}

function formatApprovalArea(event: EmailEventItem) {
  return formatEventToken(stringMetadata(event, "area") || "area");
}

function formatAreaApprovalStatus(event: EmailEventItem) {
  return formatEventToken(stringMetadata(event, "status") || "new value");
}

function formatApprovalSnapshot(event: EmailEventItem) {
  const contentHash =
    stringMetadata(event, "approved_content_hash") ||
    stringMetadata(event, "content_snapshot_hash");
  if (!contentHash) {
    return "";
  }

  return `content snapshot ${shortHash(contentHash)}`;
}

function stringMetadata(event: EmailEventItem, key: string) {
  const value = event.metadata[key];
  return typeof value === "string" ? value : "";
}

function formatEventToken(value: string) {
  return value.replaceAll("_", " ");
}

function joinEventParts(parts: string[]) {
  return parts.filter(Boolean).join(" · ");
}

function shortHash(value: string) {
  return value.length > 10 ? value.slice(0, 10) : value;
}

function isStaleApprovalEvent(event: EmailEventItem) {
  return (
    event.action === "email_review_status_updated" &&
    stringMetadata(event, "reason") === "approval_stale_after_edit"
  );
}

function isReapprovalEvent(event: EmailEventItem) {
  const reason = stringMetadata(event, "reason");
  return (
    event.action === "email_review_status_updated" &&
    (reason === "reapproved" || reason === "reapproved_after_stale_edit")
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}
