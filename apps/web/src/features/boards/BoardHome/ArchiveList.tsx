import { Alert, Badge, Button, Group, Loader, Paper, ScrollArea, Stack, Table, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";

import { fetchArchivedEmails, restoreEmail } from "../../emails/api";
import { formatStageName } from "../../emails/stages";
import type { EmailListItem } from "../../emails/types";
import styles from "./SlotMatrix.module.css";

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });

// ArchiveList is the board's history: emails taken off the board, who did it
// and when, and how much the portal saw them sent. Admins can restore one to
// its old place.
export function ArchiveList({
  boardKey,
  canRestore,
  languages,
  searchQuery,
}: {
  boardKey: string;
  canRestore: boolean;
  languages: string[];
  searchQuery: string;
}) {
  const queryClient = useQueryClient();
  const archiveQuery = useQuery({
    queryKey: ["emails", boardKey, "archived"],
    queryFn: () => fetchArchivedEmails(boardKey),
  });
  const restoreMutation = useMutation({
    mutationFn: restoreEmail,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({ color: "green", message: "It is back in its place on the board.", title: "Email restored" });
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Restore failed" }),
  });

  if (archiveQuery.isLoading) {
    return (
      <Stack align="center" justify="center" h="100%">
        <Loader />
      </Stack>
    );
  }
  if (archiveQuery.isError || !archiveQuery.data) {
    return <Alert color="red" m="md">Failed to load the archive.</Alert>;
  }

  const query = searchQuery.trim().toLowerCase();
  const emails = archiveQuery.data.filter(
    (email) =>
      (languages.length === 0 || languages.includes(email.language)) &&
      (query === "" || [email.title, email.subject ?? "", email.stage ?? ""].some((text) => text.toLowerCase().includes(query)))
  );

  // Same frame as the matrix: the board page does not scroll, the table does.
  return (
    <div className={styles.matrixView}>
      <Paper className={`${styles.wrap} ${styles.matrix}`} radius="md">
        {emails.length === 0 ? (
          <Text c="dimmed" p="md" size="sm">
            {archiveQuery.data.length === 0 ? "Nothing archived on this board yet." : "No archived emails match the filters."}
          </Text>
        ) : (
          <ScrollArea className={styles.scroll} type="auto">
            <Table className={styles.table} highlightOnHover stickyHeader>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Archived</Table.Th>
                  <Table.Th>Slot</Table.Th>
                  <Table.Th>Language</Table.Th>
                  <Table.Th>Subject</Table.Th>
                  <Table.Th>Sent by robots</Table.Th>
                  <Table.Th />
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {emails.map((email) => (
                  <ArchiveRow
                    key={email.id}
                    canRestore={canRestore}
                    email={email}
                    isRestoring={restoreMutation.isPending && restoreMutation.variables === email.id}
                    onRestore={() => restoreMutation.mutate(email.id)}
                  />
                ))}
              </Table.Tbody>
            </Table>
          </ScrollArea>
        )}
      </Paper>
    </div>
  );
}

function ArchiveRow({
  canRestore,
  email,
  isRestoring,
  onRestore,
}: {
  canRestore: boolean;
  email: EmailListItem;
  isRestoring: boolean;
  onRestore: () => void;
}) {
  const variant = [email.variant, email.adaptation_key !== "default" ? email.adaptation_label : ""]
    .filter((part) => part && part !== "new")
    .join(" · ");

  return (
    <Table.Tr>
      <Table.Td>
        <Text size="sm">{email.archived_at ? dateFormat.format(new Date(email.archived_at)) : "—"}</Text>
        {email.archived_by_email ? (
          <Text c="dimmed" size="xs">
            {email.archived_by_email}
          </Text>
        ) : null}
      </Table.Td>
      <Table.Td>
        <Text fw={600} size="sm">
          {email.title}
        </Text>
        <Text c="dimmed" size="xs">
          {formatStageName(email.stage ?? "")} · #{email.sort_order}
        </Text>
      </Table.Td>
      <Table.Td>
        <Group gap={6} wrap="nowrap">
          <Text size="sm">{email.language.toUpperCase()}</Text>
          {variant ? (
            <Badge color="gray" size="xs" variant="light">
              {variant}
            </Badge>
          ) : null}
        </Group>
      </Table.Td>
      <Table.Td>
        <Text lineClamp={2} size="sm">
          {email.subject || "—"}
        </Text>
      </Table.Td>
      <Table.Td>
        {email.portal_send_count > 0 ? (
          <>
            <Text size="sm">{email.portal_send_count} sends</Text>
            {email.portal_last_sent_at ? (
              <Text c="dimmed" size="xs">
                last {dateFormat.format(new Date(email.portal_last_sent_at))}
              </Text>
            ) : null}
          </>
        ) : (
          <Text c="dimmed" size="sm">
            not seen
          </Text>
        )}
      </Table.Td>
      <Table.Td>
        <Group gap="xs" justify="flex-end" wrap="nowrap">
          <Button component={Link} size="compact-xs" to={`/emails/${email.id}/review`} variant="default">
            Open
          </Button>
          {canRestore ? (
            <Button loading={isRestoring} size="compact-xs" onClick={onRestore}>
              Restore
            </Button>
          ) : null}
        </Group>
      </Table.Td>
    </Table.Tr>
  );
}
