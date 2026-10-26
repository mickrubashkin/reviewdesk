import { useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  MultiSelect,
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

import { fetchBoards } from "../../boards/api";
import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import { usePersistedColumnSizing } from "../../admin-table/usePersistedColumnSizing";
import { usePersistedSort } from "../../admin-table/usePersistedSort";
import {
  createAdminUser,
  fetchAdminUsers,
  updateAdminUserBoards,
  updateAdminUserLanguages,
  updateAdminUserRole,
} from "../api";
import type { UserAdminItem, UserRole } from "../../emails/types";
import { userLabel } from "../userLabel";
import { UserAccountActions, UserAliases } from "./UserAccountActions";
import styles from "../../auth-events/AuthEvents/AuthEvents.module.css";

const languageOptions = [
  { label: "EN", value: "en" },
  { label: "DE", value: "de" },
  { label: "ES", value: "es" },
  { label: "PL", value: "pl" },
  { label: "BR", value: "br" },
];
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
  const boardsQuery = useQuery({ queryKey: ["boards"], queryFn: fetchBoards });
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
        message: `${userLabel(updatedUser)} is now ${formatRole(updatedUser.role)}`,
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

  const updateLanguagesMutation = useMutation({
    mutationFn: ({ languages, userId }: { languages: string[]; userId: string }) =>
      updateAdminUserLanguages(userId, languages),
    onSuccess: (updatedUser) => {
      queryClient.setQueryData<UserAdminItem[]>(
        ["admin", "users"],
        (currentUsers) =>
          currentUsers?.map((user) =>
            user.id === updatedUser.id ? updatedUser : user
          ) ?? [updatedUser]
      );
      void queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
    onError: () => {
      notifications.show({
        color: "red",
        message: "The languages were not saved.",
        title: "Update failed",
      });
    },
  });

  const updateBoardsMutation = useMutation({
    mutationFn: ({ boards, userId }: { boards: string[]; userId: string }) =>
      updateAdminUserBoards(userId, boards),
    onSuccess: (updatedUser) => {
      queryClient.setQueryData<UserAdminItem[]>(
        ["admin", "users"],
        (currentUsers) =>
          currentUsers?.map((user) =>
            user.id === updatedUser.id ? updatedUser : user
          ) ?? [updatedUser]
      );
      void queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
    onError: () => {
      notifications.show({
        color: "red",
        message: "The boards were not saved.",
        title: "Update failed",
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
        boardOptions: (boardsQuery.data ?? []).map((board) => ({ label: board.name, value: board.key })),
        isUpdatingRole: updateRoleMutation.isPending,
        onBoardsChange: (userId, boards) => updateBoardsMutation.mutate({ boards, userId }),
        onLanguagesChange: (userId, languages) =>
          updateLanguagesMutation.mutate({ languages, userId }),
        onRoleChange: (userId, role) =>
          updateRoleMutation.mutate({
            role,
            userId,
          }),
        users: usersQuery.data ?? emptyAdminUsers,
      }),
    [
      boardsQuery.data,
      canUpdateRoles,
      updateBoardsMutation,
      updateLanguagesMutation,
      updateRoleMutation,
      usersQuery.data,
    ]
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

  // Board managers can read the user list (owner pickers) but not change it.
  if (currentUserRole === "reviewer") {
    return (
      <div className={styles.page}>
        <Alert color="red" title="Admin access is required" />
      </div>
    );
  }

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
  boardOptions,
  canUpdateRoles,
  isUpdatingRole,
  onBoardsChange,
  onLanguagesChange,
  onRoleChange,
  users,
}: {
  boardOptions: { label: string; value: string }[];
  canUpdateRoles: boolean;
  isUpdatingRole: boolean;
  onBoardsChange: (userId: string, boards: string[]) => void;
  onLanguagesChange: (userId: string, languages: string[]) => void;
  onRoleChange: (userId: string, role: UserRole) => void;
  users: UserAdminItem[];
}): ColumnDef<UserAdminItem>[] {
  return [
    {
      // Keeps the "email" id so saved sort and column widths still apply.
      accessorFn: (user) => userLabel(user).toLowerCase(),
      cell: ({ row }) => (
        <Group align="flex-start" gap="xs" justify="space-between" wrap="nowrap">
          <Stack gap={0}>
            <Text fw={row.original.name ? 600 : undefined} size="sm">
              {userLabel(row.original)}
            </Text>
            {row.original.name ? (
              <Text c="dimmed" size="xs">
                {row.original.email}
              </Text>
            ) : null}
            <UserAliases canManage={canUpdateRoles} user={row.original} />
          </Stack>
          {canUpdateRoles ? <UserAccountActions user={row.original} users={users} /> : null}
        </Group>
      ),
      header: "User",
      id: "email",
      maxSize: 420,
      minSize: 220,
      size: 300,
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
      cell: ({ row }) => (
        <MultiSelect
          aria-label={`Languages ${userLabel(row.original)} translates`}
          data={languageOptions}
          placeholder={(row.original.languages ?? []).length === 0 ? "None" : undefined}
          size="xs"
          value={row.original.languages ?? []}
          onChange={(languages) => onLanguagesChange(row.original.id, languages)}
        />
      ),
      enableSorting: false,
      header: "Translates",
      id: "languages",
      maxSize: 320,
      minSize: 150,
      size: 200,
    },
    {
      // Admins manage every board; others manage the boards picked here and
      // only read the rest.
      cell: ({ row }) =>
        row.original.role === "reviewer" ? (
          <MultiSelect
            aria-label={`Boards ${userLabel(row.original)} manages`}
            data={boardOptions}
            placeholder={(row.original.managed_boards ?? []).length === 0 ? "None" : undefined}
            size="xs"
            value={row.original.managed_boards ?? []}
            onChange={(boards) => onBoardsChange(row.original.id, boards)}
          />
        ) : (
          <Text c="dimmed" size="xs">
            All boards
          </Text>
        ),
      enableSorting: false,
      header: "Manages",
      id: "managed_boards",
      maxSize: 360,
      minSize: 150,
      size: 220,
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
