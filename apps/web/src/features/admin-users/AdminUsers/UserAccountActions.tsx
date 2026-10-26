import { useState } from "react";
import {
  ActionIcon,
  Alert,
  Button,
  CloseButton,
  Group,
  Menu,
  Modal,
  Select,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { DotsThreeIcon } from "@phosphor-icons/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import type { UserAdminItem } from "../../emails/types";
import { userNamesQueryKey } from "../../../shared/userNames";
import { userLabel } from "../userLabel";
import {
  addAdminUserAlias,
  deleteAdminUser,
  mergeAdminUsers,
  removeAdminUserAlias,
  updateAdminUserName,
} from "../api";

const usersQueryKey = ["admin", "users"];

function invalidateUsers(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: usersQueryKey });
  void queryClient.invalidateQueries({ queryKey: userNamesQueryKey });
}

// UserAliases lists a user's extra addresses; super admins can remove one.
export function UserAliases({ canManage, user }: { canManage: boolean; user: UserAdminItem }) {
  const queryClient = useQueryClient();
  const removeMutation = useMutation({
    mutationFn: (email: string) => removeAdminUserAlias(user.id, email),
    onSuccess: () => invalidateUsers(queryClient),
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not removed" }),
  });
  const aliases = user.aliases ?? [];
  if (aliases.length === 0) {
    return null;
  }
  return (
    <Stack gap={0}>
      {aliases.map((alias) => (
        <Group key={alias} gap={2} wrap="nowrap">
          <Text c="dimmed" size="xs">
            also {alias}
          </Text>
          {canManage ? (
            <CloseButton
              aria-label={`Remove alias ${alias}`}
              size="xs"
              onClick={() => removeMutation.mutate(alias)}
            />
          ) : null}
        </Group>
      ))}
    </Stack>
  );
}

// UserAccountActions renames a user, adds an alias, merges them into
// another user or deletes them (super admins only).
export function UserAccountActions({ user, users }: { user: UserAdminItem; users: UserAdminItem[] }) {
  const [mode, setMode] = useState<"name" | "alias" | "merge" | "delete" | null>(null);
  return (
    <>
      <Menu position="bottom-end" shadow="md">
        <Menu.Target>
          <ActionIcon aria-label={`Account actions for ${userLabel(user)}`} size="sm" variant="subtle">
            <DotsThreeIcon aria-hidden="true" size={16} />
          </ActionIcon>
        </Menu.Target>
        <Menu.Dropdown>
          <Menu.Item onClick={() => setMode("name")}>{user.name ? "Rename…" : "Add name…"}</Menu.Item>
          <Menu.Item onClick={() => setMode("alias")}>Add another address…</Menu.Item>
          <Menu.Item color="red" onClick={() => setMode("merge")}>
            Merge into another user…
          </Menu.Item>
          <Menu.Item color="red" onClick={() => setMode("delete")}>
            Delete user…
          </Menu.Item>
        </Menu.Dropdown>
      </Menu>
      {mode === "name" ? <EditNameModal user={user} onClose={() => setMode(null)} /> : null}
      {mode === "alias" ? <AddAliasModal user={user} onClose={() => setMode(null)} /> : null}
      {mode === "delete" ? <DeleteUserModal user={user} onClose={() => setMode(null)} /> : null}
      {mode === "merge" ? <MergeUserModal user={user} users={users} onClose={() => setMode(null)} /> : null}
    </>
  );
}

function EditNameModal({ user, onClose }: { user: UserAdminItem; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(user.name);
  const mutation = useMutation({
    mutationFn: () => updateAdminUserName(user.id, name),
    onSuccess: () => {
      invalidateUsers(queryClient);
      onClose();
    },
  });
  return (
    <Modal centered opened title={`Name for ${user.email}`} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          mutation.mutate();
        }}
      >
        <Stack gap="sm">
          <TextInput
            data-autofocus
            description="Shown instead of the email across ReviewDesk. Leave empty to show the email."
            label="Name"
            maxLength={100}
            placeholder="First Last"
            value={name}
            onChange={(event) => setName(event.currentTarget.value)}
          />
          {mutation.isError ? (
            <Alert color="red" variant="light">
              {mutation.error.message}
            </Alert>
          ) : null}
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button loading={mutation.isPending} type="submit">
              Save
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}

function AddAliasModal({ user, onClose }: { user: UserAdminItem; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const mutation = useMutation({
    mutationFn: () => addAdminUserAlias(user.id, email.trim()),
    onSuccess: () => {
      invalidateUsers(queryClient);
      onClose();
    },
  });
  return (
    <Modal centered opened title={`Another address for ${userLabel(user)}`} onClose={onClose}>
      <Stack gap="sm">
        <Text c="dimmed" size="sm">
          Signing in with this address opens {userLabel(user)}&apos;s account. Use it for a second work address that has not
          signed in yet; if it already has its own user, merge that user instead.
        </Text>
        <TextInput
          data-autofocus
          label="Work address"
          placeholder="name@company.com"
          value={email}
          onChange={(event) => setEmail(event.currentTarget.value)}
        />
        {mutation.isError ? (
          <Alert color="red" variant="light">
            {mutation.error.message}
          </Alert>
        ) : null}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!email.trim()} loading={mutation.isPending} onClick={() => mutation.mutate()}>
            Add address
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}

function MergeUserModal({
  user,
  users,
  onClose,
}: {
  user: UserAdminItem;
  users: UserAdminItem[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [targetId, setTargetId] = useState<string | null>(null);
  const target = users.find((item) => item.id === targetId);
  const mutation = useMutation({
    mutationFn: () => mergeAdminUsers(targetId ?? "", user.id),
    onSuccess: (merged) => {
      invalidateUsers(queryClient);
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "green",
        message: `${user.email} now signs in to ${userLabel(merged)}.`,
        title: "Users merged",
      });
      onClose();
    },
  });
  return (
    <Modal centered opened title={`Merge ${userLabel(user)}`} onClose={onClose}>
      <Stack gap="sm">
        <Select
          data={users
            .filter((item) => item.id !== user.id)
            .map((item) => ({
              label: `${item.name ? `${item.name} · ` : ""}${item.email} (${item.role.replaceAll("_", " ")})`,
              value: item.id,
            }))}
          label="Into"
          placeholder="The user to keep"
          searchable
          value={targetId}
          onChange={setTargetId}
        />
        {target ? (
          <Alert color="orange" variant="light">
            <Text size="sm">
              {user.email} will be removed. Their emails, comments, history and languages move to {userLabel(target)},
              and signing in with {user.email} opens {userLabel(target)}&apos;s account. {userLabel(target)} keeps the
              role {target.role.replaceAll("_", " ")}
              {target.name || !user.name ? "" : ` and takes the name ${user.name}`}. There is no undo button.
            </Text>
          </Alert>
        ) : null}
        {mutation.isError ? (
          <Alert color="red" variant="light">
            {mutation.error.message}
          </Alert>
        ) : null}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button color="red" disabled={!target} loading={mutation.isPending} onClick={() => mutation.mutate()}>
            Merge
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}

function DeleteUserModal({ user, onClose }: { user: UserAdminItem; onClose: () => void }) {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => deleteAdminUser(user.id),
    onSuccess: () => {
      invalidateUsers(queryClient);
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({ color: "green", message: `${userLabel(user)} no longer has access.`, title: "User deleted" });
      onClose();
    },
  });
  const addresses = [user.email, ...(user.aliases ?? [])].join(", ");
  return (
    <Modal centered opened title={`Delete ${userLabel(user)}`} onClose={onClose}>
      <Stack gap="sm">
        <Alert color="red" variant="light">
          <Text size="sm">
            {addresses} will lose access. Emails assigned to them become unassigned. Their comments and history stay,
            but show &quot;Unknown author&quot;. There is no undo button.
          </Text>
        </Alert>
        <Text c="dimmed" size="sm">
          If this is the same person as another user, use Merge instead to keep their history.
        </Text>
        {mutation.isError ? (
          <Alert color="red" variant="light">
            {mutation.error.message}
          </Alert>
        ) : null}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button color="red" loading={mutation.isPending} onClick={() => mutation.mutate()}>
            Delete
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
