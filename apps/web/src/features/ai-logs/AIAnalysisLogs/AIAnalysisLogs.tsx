import { useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Code,
  Group,
  Loader,
  Modal,
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
import { ApiError } from "../../../shared/api";
import { fetchAIAnalysisLogs } from "../api";
import type { AIAnalysisLogFilters, AIAnalysisLogItem } from "../../emails/types";
import styles from "./AIAnalysisLogs.module.css";

const limitOptions = ["50", "100", "250", "500"];
const cacheStatusOptions = [
  { value: "hit", label: "Hit" },
  { value: "miss", label: "Miss" },
  { value: "unknown", label: "Unknown" },
];
type SortDirection = "asc" | "desc";
type AILogSortKey =
  | "created_at"
  | "user_email"
  | "email"
  | "status"
  | "cache_status"
  | "model"
  | "latency_ms"
  | "tokens"
  | "cached_tokens"
  | "error_message";
const defaultSort = {
  direction: "desc" as SortDirection,
  key: "created_at" as AILogSortKey,
};
const sortKeys: readonly AILogSortKey[] = [
  "created_at",
  "user_email",
  "email",
  "status",
  "cache_status",
  "model",
  "latency_ms",
  "tokens",
  "cached_tokens",
  "error_message",
];
const emptyAIAnalysisLogs: AIAnalysisLogItem[] = [];

export function AIAnalysisLogs() {
  const [filters, setFilters] = useState<AIAnalysisLogFilters>({
    limit: "100",
  });
  const [selectedErrorLog, setSelectedErrorLog] =
    useState<AIAnalysisLogItem | null>(null);

  const logsQuery = useQuery({
    queryKey: ["ai-analysis-logs", filters],
    queryFn: () => fetchAIAnalysisLogs(filters),
    placeholderData: (previous) => previous,
  });

  const columns = useMemo(
    () => buildColumns({ onViewError: setSelectedErrorLog }),
    []
  );
  const { table, rows: logs } = useAdminTable({
    columns,
    data: logsQuery.data ?? emptyAIAnalysisLogs,
    defaultSort,
    sortKeys,
    storageKey: "ai-analysis-logs",
  });
  const summary = useMemo(
    () => buildCacheSummary(logsQuery.data ?? []),
    [logsQuery.data]
  );
  const errorCopy = getAIAnalysisLogsErrorCopy(logsQuery.error);
  const visibleFilters = useMemo(
    () => ({
      cache_status: filters.cache_status ?? "",
      email_id: filters.email_id ?? "",
      limit: filters.limit ?? "100",
      model: filters.model ?? "",
      status: filters.status ?? "",
    }),
    [filters]
  );

  return (
    <div className={styles.page}>
      <AdminTableHeader
        subtitle="Stored request metrics and errors from ai_analysis_logs."
        title="AI analysis logs"
      />

      <section className={styles.filters}>
        <Group align="end" gap="sm">
          <Select
            clearable
            data={[
              { value: "success", label: "Success" },
              { value: "error", label: "Error" },
            ]}
            label="Status"
            placeholder="All statuses"
            value={visibleFilters.status || null}
            onChange={(value) =>
              setFilters((current) => ({ ...current, status: value ?? "" }))
            }
          />
          <Select
            clearable
            data={cacheStatusOptions}
            label="Cache"
            placeholder="All cache states"
            value={visibleFilters.cache_status || null}
            onChange={(value) =>
              setFilters((current) => ({
                ...current,
                cache_status: value ?? "",
              }))
            }
          />
          <TextInput
            label="Email ID"
            placeholder="UUID"
            value={visibleFilters.email_id}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                email_id: event.currentTarget.value,
              }))
            }
          />
          <TextInput
            label="Model"
            placeholder="gpt-5-nano"
            value={visibleFilters.model}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                model: event.currentTarget.value,
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
          <Button
            variant="light"
            onClick={() => setFilters({ limit: "100" })}
          >
            Reset
          </Button>
        </Group>
      </section>

      {logsQuery.isSuccess ? (
        <section className={styles.summary}>
          <Group gap="xs">
            <SummaryBadge label="Total" value={summary.total} />
            <SummaryBadge color="green" label="Hits" value={summary.hit} />
            <SummaryBadge color="yellow" label="Misses" value={summary.miss} />
            <SummaryBadge color="gray" label="Unknown" value={summary.unknown} />
            <SummaryBadge
              color="blue"
              label="Avg cached"
              value={summary.averageCachedTokens}
            />
          </Group>
        </section>
      ) : null}

      <section className={styles.tableShell}>
        {logsQuery.isLoading ? (
          <Stack align="center" justify="center" h={240}>
            <Loader />
            <Text c="dimmed">Loading AI logs</Text>
          </Stack>
        ) : null}

        {logsQuery.isError ? (
          <Alert color="red" title={errorCopy.title}>
            {errorCopy.message}
          </Alert>
        ) : null}

        {logsQuery.isSuccess && table.getRowModel().rows.length === 0 ? (
          <Stack align="center" justify="center" h={240}>
            <Text fw={600}>No AI logs found</Text>
            <Text c="dimmed" size="sm">
              Try changing filters or running a fresh AI analysis.
            </Text>
          </Stack>
        ) : null}

        {logsQuery.isSuccess && logs.length > 0 ? (
          <AdminTable table={table} />
        ) : null}
      </section>

      <Modal
        opened={selectedErrorLog !== null}
        title="AI analysis error"
        onClose={() => setSelectedErrorLog(null)}
      >
        <Stack gap="sm">
          <Text c="dimmed" size="sm">
            {selectedErrorLog
              ? `${selectedErrorLog.model} · ${formatDateTime(selectedErrorLog.created_at)}`
              : ""}
          </Text>
          <Code block className={styles.errorText}>
            {selectedErrorLog?.error_message ?? ""}
          </Code>
        </Stack>
      </Modal>
    </div>
  );
}

export default AIAnalysisLogs;

function buildColumns({
  onViewError,
}: {
  onViewError: (log: AIAnalysisLogItem) => void;
}): ColumnDef<AIAnalysisLogItem>[] {
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
      accessorKey: "user_email",
      cell: ({ row }) => (
        <Text size="sm">{row.original.user_email ?? "Unknown user"}</Text>
      ),
      header: "User",
      id: "user_email",
      maxSize: 340,
      minSize: 140,
      size: 220,
    },
    {
      accessorFn: emailSortValue,
      cell: ({ row }) => <EmailCell log={row.original} />,
      header: "Email",
      id: "email",
      maxSize: 520,
      minSize: 220,
      size: 320,
    },
    {
      accessorKey: "status",
      cell: ({ row }) => (
        <Badge color={statusColor(row.original.status)} variant="light">
          {row.original.status}
        </Badge>
      ),
      header: "Status",
      id: "status",
      maxSize: 180,
      minSize: 100,
      size: 120,
    },
    {
      accessorKey: "cache_status",
      cell: ({ row }) => (
        <Badge color={cacheStatusColor(row.original.cache_status)} variant="light">
          {row.original.cache_status}
        </Badge>
      ),
      header: "Cache",
      id: "cache_status",
      maxSize: 160,
      minSize: 100,
      size: 110,
    },
    {
      accessorKey: "model",
      cell: ({ row }) => <span className={styles.monoCell}>{row.original.model}</span>,
      header: "Model",
      id: "model",
      maxSize: 260,
      minSize: 120,
      size: 150,
    },
    {
      accessorKey: "latency_ms",
      cell: ({ row }) => `${row.original.latency_ms} ms`,
      header: "Latency",
      id: "latency_ms",
      maxSize: 180,
      minSize: 100,
      size: 120,
    },
    {
      accessorFn: tokenSortValue,
      cell: ({ row }) => formatTokens(row.original),
      header: "Tokens",
      id: "tokens",
      maxSize: 360,
      minSize: 160,
      size: 220,
    },
    {
      accessorFn: (log) => log.cached_tokens ?? -1,
      cell: ({ row }) => formatCachedTokens(row.original),
      header: "Cached",
      id: "cached_tokens",
      maxSize: 220,
      minSize: 110,
      size: 130,
    },
    {
      accessorKey: "error_message",
      cell: ({ row }) =>
        row.original.error_message ? (
          <button
            className={styles.errorButton}
            type="button"
            onClick={() => onViewError(row.original)}
          >
            View error
          </button>
        ) : (
          <Text c="dimmed" size="sm">
            -
          </Text>
        ),
      header: "Error",
      id: "error_message",
      maxSize: 220,
      minSize: 100,
      size: 120,
    },
  ];
}

function EmailCell({ log }: { log: AIAnalysisLogItem }) {
  return (
    <Stack className={styles.emailCell} gap={2}>
      <Text fw={600} size="sm" lineClamp={1}>
        {log.email_title ?? "Deleted email"}
      </Text>
      <Group gap={6}>
        {log.language ? (
          <Badge size="xs" variant="light">
            {log.language.toUpperCase()}
          </Badge>
        ) : null}
        {log.variant ? (
          <Badge color="gray" size="xs" variant="light">
            {log.variant}
          </Badge>
        ) : null}
      </Group>
    </Stack>
  );
}


function emailSortValue(log: AIAnalysisLogItem) {
  return log.email_title ?? log.email_slug ?? log.email_id ?? "";
}

function tokenSortValue(log: AIAnalysisLogItem) {
  return log.total_tokens ?? log.input_tokens ?? log.output_tokens ?? -1;
}

function getAIAnalysisLogsErrorCopy(error: unknown) {
  if (error instanceof ApiError && error.status === 403) {
    return {
      title: "Access denied",
      message: "Admin access is required to view AI analysis logs.",
    };
  }

  if (error instanceof ApiError && error.status === 401) {
    return {
      title: "Session expired",
      message: "Sign in again to continue.",
    };
  }

  return {
    title: "Failed to load AI logs",
    message: "Check that the API server is reachable.",
  };
}

function SummaryBadge({
  color,
  label,
  value,
}: {
  color?: string;
  label: string;
  value: number | string;
}) {
  return (
    <Badge color={color} radius="sm" size="lg" variant="light">
      {label}: {value}
    </Badge>
  );
}

function buildCacheSummary(logs: AIAnalysisLogItem[]) {
  const summary = {
    averageCachedTokens: "-",
    hit: 0,
    miss: 0,
    total: logs.length,
    unknown: 0,
  };
  const knownCachedTokenValues = logs
    .map((log) => log.cached_tokens)
    .filter((value): value is number => value !== null);

  logs.forEach((log) => {
    if (log.cache_status === "hit") {
      summary.hit += 1;
    } else if (log.cache_status === "miss") {
      summary.miss += 1;
    } else {
      summary.unknown += 1;
    }
  });

  if (knownCachedTokenValues.length > 0) {
    const totalCachedTokens = knownCachedTokenValues.reduce(
      (total, value) => total + value,
      0
    );
    summary.averageCachedTokens = String(
      Math.round(totalCachedTokens / knownCachedTokenValues.length)
    );
  }

  return summary;
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function statusColor(status: string) {
  if (status === "success") {
    return "green";
  }
  if (status === "error") {
    return "red";
  }

  return "gray";
}

function cacheStatusColor(status: AIAnalysisLogItem["cache_status"]) {
  if (status === "hit") {
    return "green";
  }
  if (status === "miss") {
    return "yellow";
  }

  return "gray";
}

function formatTokens(log: AIAnalysisLogItem) {
  if (
    log.input_tokens === null &&
    log.output_tokens === null &&
    log.total_tokens === null
  ) {
    return "-";
  }

  return `${log.total_tokens ?? "-"} total (${log.input_tokens ?? "-"} in / ${
    log.output_tokens ?? "-"
  } out)`;
}

function formatCachedTokens(log: AIAnalysisLogItem) {
  if (log.cached_tokens === null) {
    return "-";
  }

  if (!log.input_tokens) {
    return `${log.cached_tokens}`;
  }

  const percentage = Math.round((log.cached_tokens / log.input_tokens) * 100);
  return `${log.cached_tokens} (${percentage}%)`;
}
