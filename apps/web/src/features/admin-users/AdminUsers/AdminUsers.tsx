import { useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  ScrollArea,
  Select,
  Stack,
  Table,
  Text,
  TextInput,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";

import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import { usePersistedColumnSizing } from "../../admin-table/usePersistedColumnSizing";
import { usePersistedSort } from "../../admin-table/usePersistedSort";
import { createAdminUser, fetchAdminUsers, updateAdminUserRole } from "../api";
import type { UserAdminItem, UserRole } from "../../emails/types";
import styles from "../../auth-events/AuthEvents/AuthEvents.module.css";

const roleOptions: { label: string; value: UserRole }[] = [
  { label: "Super admin", value: "super_admin" },
  { label: "Admin", value: "admin" },
  { label: "Reviewer", value: "reviewer" },
];
type SortDirection = "asc" | "desc";
type UserSortKey = keyof Pick<
  UserAdminItem,
  "email" | "role" | "last_seen_at" | "created_at" | "updated_at"
>;
const defaultSort = {
  direction: "asc" as SortDirection,
  key: "email" as UserSortKey,
};
const sortKeys: readonly UserSortKey[] = [
  "email",
  "role",
  "last_seen_at",
  "created_at",
  "updated_at",
];
const emptyAdminUsers: UserAdminItem[] = [];

type AdminUsersProps = {
  currentUserRole: UserRole;
};

export function AdminUsers({ currentUserRole }: AdminUsersProps) {
  const queryClient = useQueryClient();
  const [sort, setSort] = usePersistedSort(
    "reviewdesk:admin-table-sort:users",
    defaultSort,
    sortKeys
  );
  const [columnSizing, setColumnSizing] = usePersistedColumnSizing(
    "reviewdesk:admin-table-column-sizing:users"
  );
  const [emailDraft, setEmailDraft] = useState("");
  const [roleDraft, setRoleDraft] = useState<UserRole>(
    currentUserRole === "super_admin" ? "reviewer" : "reviewer"
  );
  const usersQuery = useQuery({
    queryKey: ["admin", "users"],
    queryFn: fetchAdminUsers,
  });
  const createUserMutation = useMutation({
    mutationFn: ({ email, role }: { email: string; role: UserRole }) =>
      createAdminUser({ email, role }),
    onSuccess: (createdUser) => {
      queryClient.setQueryData<UserAdminItem[]>(
        ["admin", "users"],
        (currentUsers) => [...(currentUsers ?? []), createdUser]
      );
      setEmailDraft("");
      setRoleDraft("reviewer");
      notifications.show({
        color: "green",
        message: `${createdUser.email} can now log in with OTP.`,
        title: "User added",
      });
    },
    onError: () => {
      notifications.show({
        color: "red",
        message: "Check the email, role, and whether the user already exists.",
        title: "User add failed",
      });
    },
  });
  const updateRoleMutation = useMutation({
    mutationFn: ({ role, userId }: { role: UserRole; userId: string }) =>
      updateAdminUserRole(userId, role),
    onSuccess: (updatedUser) => {
      queryClient.setQueryData<UserAdminItem[]>(
        ["admin", "users"],
        (currentUsers) =>
          currentUsers?.map((user) =>
            user.id === updatedUser.id ? updatedUser : user
          ) ?? [updatedUser]
      );
      void queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
      notifications.show({
        color: "green",
        message: `${updatedUser.email} is now ${formatRole(updatedUser.role)}`,
        title: "Role updated",
      });
    },
    onError: () => {
      notifications.show({
        color: "red",
        message: "Only super admins can update roles. You also cannot demote yourself.",
        title: "Role update failed",
      });
    },
  });

  const createRoleOptions =
    currentUserRole === "super_admin"
      ? roleOptions
      : roleOptions.filter((option) => option.value === "reviewer");
  const canUpdateRoles = currentUserRole === "super_admin";
  const trimmedEmailDraft = emailDraft.trim();
  const columns = useMemo(
    () =>
      buildColumns({
        canUpdateRoles,
        isUpdatingRole: updateRoleMutation.isPending,
        onRoleChange: (userId, role) =>
          updateRoleMutation.mutate({
            role,
            userId,
          }),
      }),
    [canUpdateRoles, updateRoleMutation]
  );
  const sorting = useMemo<SortingState>(
    () => [{ desc: sort.direction === "desc", id: sort.key }],
    [sort]
  );
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    columnResizeMode: "onChange",
    columns,
    data: usersQuery.data ?? emptyAdminUsers,
    enableColumnResizing: true,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    onColumnSizingChange: setColumnSizing,
    onSortingChange: (updater) => {
      const nextSorting =
        typeof updater === "function" ? updater(sorting) : updater;
      const nextSort = nextSorting[0];
      if (!nextSort || !sortKeys.includes(nextSort.id as UserSortKey)) {
        setSort(defaultSort);
        return;
      }
      setSort({
        direction: nextSort.desc ? "desc" : "asc",
        key: nextSort.id as UserSortKey,
      });
    },
    state: {
      columnSizing,
      sorting,
    },
  });
  const rows = table.getRowModel().rows;

  return (
    <div className={styles.page}>
      <AdminTableHeader
        subtitle="Manage ReviewDesk access roles."
        title="Users"
      />

      <section className={styles.filters}>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (!trimmedEmailDraft) {
              return;
            }
            createUserMutation.mutate({
              email: trimmedEmailDraft,
              role: roleDraft,
            });
          }}
        >
          <Group align="end" gap="sm">
            <TextInput
              label="Email"
              placeholder="new.user@example.com"
              value={emailDraft}
              onChange={(event) => setEmailDraft(event.currentTarget.value)}
            />
            <Select
              allowDeselect={false}
              data={createRoleOptions}
              label="Role"
              value={roleDraft}
              w={180}
              onChange={(value) => setRoleDraft((value as UserRole | null) ?? "reviewer")}
            />
            <Button
              disabled={!trimmedEmailDraft}
              loading={createUserMutation.isPending}
              type="submit"
            >
              Add user
            </Button>
          </Group>
        </form>
      </section>

      <section className={styles.tableShell}>
        {usersQuery.isLoading ? (
          <Stack align="center" justify="center" h={240}>
            <Loader />
            <Text c="dimmed">Loading users</Text>
          </Stack>
        ) : null}

        {usersQuery.isError ? (
          <Alert color="red" title="Failed to load users">
            Admin access is required.
          </Alert>
        ) : null}

        {usersQuery.isSuccess && rows.length === 0 ? (
          <Stack align="center" justify="center" h={240}>
            <Text fw={600}>No users found</Text>
          </Stack>
        ) : null}

        {usersQuery.isSuccess && rows.length > 0 ? (
          <ScrollArea type="auto">
            <Table
              className={styles.table}
              highlightOnHover
              horizontalSpacing="md"
              style={{ width: table.getTotalSize() }}
              verticalSpacing="sm"
            >
              <Table.Thead>
                {table.getHeaderGroups().map((headerGroup) => (
                  <Table.Tr key={headerGroup.id}>
                    {headerGroup.headers.map((header) => (
                      <Table.Th
                        className={styles.resizableTh}
                        key={header.id}
                        style={{ width: header.getSize() }}
                      >
                        {header.isPlaceholder ? null : (
                          <button
                            className={styles.sortButton}
                            disabled={!header.column.getCanSort()}
                            type="button"
                            onClick={header.column.getToggleSortingHandler()}
                          >
                            {flexRender(
                              header.column.columnDef.header,
                              header.getContext()
                            )}
                            <span className={styles.sortIndicator}>
                              {formatSortIndicator(header.column.getIsSorted())}
                            </span>
                          </button>
                        )}
                        {header.column.getCanResize() ? (
                          <button
                            aria-label={`Resize ${header.column.columnDef.header} column`}
                            className={styles.resizeHandle}
                            type="button"
                            onDoubleClick={() => header.column.resetSize()}
                            onMouseDown={header.getResizeHandler()}
                            onTouchStart={header.getResizeHandler()}
                          />
                        ) : null}
                      </Table.Th>
                    ))}
                  </Table.Tr>
                ))}
              </Table.Thead>
              <Table.Tbody>
                {rows.map((row) => (
                  <Table.Tr key={row.id}>
                    {row.getVisibleCells().map((cell) => (
                      <Table.Td
                        key={cell.id}
                        style={{ width: cell.column.getSize() }}
                      >
                        {flexRender(
                          cell.column.columnDef.cell,
                          cell.getContext()
                        )}
                      </Table.Td>
                    ))}
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </ScrollArea>
        ) : null}
      </section>
    </div>
  );
}

export default AdminUsers;

function buildColumns({
  canUpdateRoles,
  isUpdatingRole,
  onRoleChange,
}: {
  canUpdateRoles: boolean;
  isUpdatingRole: boolean;
  onRoleChange: (userId: string, role: UserRole) => void;
}): ColumnDef<UserAdminItem>[] {
  return [
    {
      accessorKey: "email",
      cell: ({ row }) => row.original.email,
      header: "Email",
      id: "email",
      maxSize: 420,
      minSize: 220,
      size: 280,
    },
    {
      accessorKey: "role",
      cell: ({ row }) => (
        <Badge color={getRoleColor(row.original.role)} variant="light">
          {formatRole(row.original.role)}
        </Badge>
      ),
      header: "Role",
      id: "role",
      maxSize: 180,
      minSize: 110,
      size: 140,
    },
    {
      accessorKey: "last_seen_at",
      cell: ({ row }) => formatOptionalDateTime(row.original.last_seen_at),
      header: "Last seen",
      id: "last_seen_at",
      maxSize: 260,
      minSize: 140,
      size: 170,
    },
    {
      accessorKey: "created_at",
      cell: ({ row }) => formatDateTime(row.original.created_at),
      header: "Created",
      id: "created_at",
      maxSize: 260,
      minSize: 140,
      size: 170,
    },
    {
      accessorKey: "updated_at",
      cell: ({ row }) => formatDateTime(row.original.updated_at),
      header: "Updated",
      id: "updated_at",
      maxSize: 260,
      minSize: 140,
      size: 170,
    },
    {
      cell: ({ row }) => (
        <Group gap="xs" wrap="nowrap">
          <Select
            allowDeselect={false}
            data={roleOptions}
            disabled={!canUpdateRoles || isUpdatingRole}
            size="xs"
            value={row.original.role}
            w={160}
            onChange={(value) => {
              const role = value as UserRole | null;
              if (!role || role === row.original.role) {
                return;
              }
              onRoleChange(row.original.id, role);
            }}
          />
        </Group>
      ),
      enableSorting: false,
      header: "Change role",
      id: "change_role",
      maxSize: 260,
      minSize: 190,
      size: 220,
    },
  ];
}

function formatSortIndicator(sortState: false | "asc" | "desc") {
  if (sortState === "asc") {
    return "↑";
  }
  if (sortState === "desc") {
    return "↓";
  }
  return "";
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatOptionalDateTime(value: string | null) {
  return value ? formatDateTime(value) : "Never";
}

function formatRole(role: UserRole) {
  return role.replaceAll("_", " ");
}

function getRoleColor(role: UserRole) {
  switch (role) {
    case "super_admin":
      return "violet";
    case "admin":
      return "blue";
    default:
      return "gray";
  }
}
