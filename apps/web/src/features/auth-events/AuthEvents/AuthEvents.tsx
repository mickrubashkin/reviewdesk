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
import { fetchAuthEvents } from "../api";
import type { AuthEventFilters, AuthEventItem } from "../../emails/types";
import styles from "./AuthEvents.module.css";

const limitOptions = ["50", "100", "250", "500"];
const eventTypeOptions = [
  { value: "otp_requested", label: "OTP requested" },
  { value: "otp_login", label: "OTP login" },
  { value: "failed_otp", label: "Failed OTP" },
  { value: "logout", label: "Logout" },
];
type SortDirection = "asc" | "desc";
type AuthEventSortKey = keyof Pick<
  AuthEventItem,
  "created_at" | "email" | "event_type" | "success" | "ip_address" | "user_agent"
>;

const defaultSort = {
  direction: "desc" as SortDirection,
  key: "created_at" as AuthEventSortKey,
};
const sortKeys: readonly AuthEventSortKey[] = [
  "created_at",
  "email",
  "event_type",
  "success",
  "ip_address",
  "user_agent",
];
const emptyAuthEvents: AuthEventItem[] = [];

export function AuthEvents() {
  const [filters, setFilters] = useState<AuthEventFilters>({
    limit: "100",
  });

  const eventsQuery = useQuery({
    queryKey: ["auth-events", filters],
    queryFn: () => fetchAuthEvents(filters),
    placeholderData: (previous) => previous,
  });

  const columns = useMemo(() => buildColumns(), []);
  const { table, rows } = useAdminTable({
    columns,
    data: eventsQuery.data ?? emptyAuthEvents,
    defaultSort,
    sortKeys,
    storageKey: "auth-events",
  });
  const visibleFilters = useMemo(
    () => ({
      email: filters.email ?? "",
      event_type: filters.event_type ?? "",
      limit: filters.limit ?? "100",
      success: filters.success ?? "",
    }),
    [filters]
  );

  return (
    <div className={styles.page}>
      <AdminTableHeader
        subtitle="Login, logout, and one-time code audit trail."
        title="Auth events"
      />

      <section className={styles.filters}>
        <Group align="end" gap="sm">
          <TextInput
            label="Email"
            placeholder="user@example.com"
            value={visibleFilters.email}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                email: event.currentTarget.value,
              }))
            }
          />
          <Select
            clearable
            data={eventTypeOptions}
            label="Event"
            placeholder="All events"
            value={visibleFilters.event_type || null}
            onChange={(value) =>
              setFilters((current) => ({
                ...current,
                event_type: value ?? "",
              }))
            }
          />
          <Select
            clearable
            data={[
              { value: "true", label: "Success" },
              { value: "false", label: "Failed" },
            ]}
            label="Result"
            placeholder="All results"
            value={visibleFilters.success || null}
            onChange={(value) =>
              setFilters((current) => ({ ...current, success: value ?? "" }))
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
            <Text c="dimmed">Loading auth events</Text>
          </Stack>
        ) : null}

        {eventsQuery.isError ? (
          <Alert color="red" title="Failed to load auth events">
            Admin access is required.
          </Alert>
        ) : null}

        {eventsQuery.isSuccess && rows.length === 0 ? (
          <Stack align="center" justify="center" h={240}>
            <Text fw={600}>No auth events found</Text>
            <Text c="dimmed" size="sm">
              Try changing filters or signing in again.
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

export default AuthEvents;

function buildColumns(): ColumnDef<AuthEventItem>[] {
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
      accessorKey: "email",
      cell: ({ row }) => row.original.email,
      header: "Email",
      id: "email",
      maxSize: 360,
      minSize: 180,
      size: 240,
    },
    {
      accessorKey: "event_type",
      cell: ({ row }) => (
        <Badge variant="light">{formatEventType(row.original.event_type)}</Badge>
      ),
      header: "Event",
      id: "event_type",
      maxSize: 260,
      minSize: 140,
      size: 180,
    },
    {
      accessorKey: "success",
      cell: ({ row }) => (
        <Badge color={row.original.success ? "green" : "red"} variant="light">
          {row.original.success ? "success" : "failed"}
        </Badge>
      ),
      header: "Result",
      id: "success",
      maxSize: 160,
      minSize: 100,
      size: 110,
    },
    {
      accessorKey: "ip_address",
      cell: ({ row }) => (
        <span className={styles.monoCell}>{row.original.ip_address ?? "-"}</span>
      ),
      header: "IP",
      id: "ip_address",
      maxSize: 220,
      minSize: 120,
      size: 150,
    },
    {
      accessorKey: "user_agent",
      cell: ({ row }) => (
        <Text
          c="dimmed"
          className={styles.userAgentCell}
          lineClamp={2}
          size="sm"
        >
          {row.original.user_agent ?? "-"}
        </Text>
      ),
      header: "User agent",
      id: "user_agent",
      maxSize: 720,
      minSize: 280,
      size: 440,
    },
  ];
}


function formatDateTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatEventType(eventType: AuthEventItem["event_type"]) {
  return eventType.replaceAll("_", " ");
}
