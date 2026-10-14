import { useState } from "react";
import { Alert, Badge, Button, Collapse, Group, Modal, Paper, ScrollArea, Select, Stack, Table, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { ApiError } from "../../../shared/api";
import { placeEmail } from "../../emails/api";
import { formatStageName } from "../../emails/stages";
import type { Board, EmailListItem, StageColumn } from "../../emails/types";
import styles from "./SlotMatrix.module.css";

const newSlotValue = "__new__";

// UnsortedPanel lists live emails imported from the portal that are not in
// a slot of this board yet, to preview them and place them into slots.
export function UnsortedPanel({
  board,
  columns,
  emails,
  onPreview,
}: {
  board: Board;
  columns: StageColumn[];
  emails: EmailListItem[];
  onPreview: (email: EmailListItem) => void;
}) {
  const [opened, setOpened] = useState(true);
  const [placing, setPlacing] = useState<EmailListItem | null>(null);
  if (emails.length === 0) return null;

  return (
    <Paper className={styles.wrap} radius="md">
      <Group className={styles.unsortedHeader} justify="space-between">
        <Text fw={700} size="sm">
          Live emails from the portal not in a slot yet ({emails.length})
        </Text>
        <Button size="compact-xs" variant="subtle" onClick={() => setOpened((o) => !o)}>
          {opened ? "Hide" : "Show"}
        </Button>
      </Group>
      <Collapse expanded={opened}>
        <ScrollArea.Autosize mah="30vh" type="auto">
        <Table striped>
          <Table.Tbody>
            {emails.map((email) => (
              <Table.Tr key={email.id}>
                <Table.Td w={90}>
                  <Badge color="green" size="xs" variant="light">
                    Live · {email.portal_send_count}
                  </Badge>
                </Table.Td>
                <Table.Td w={50}>{email.language.toUpperCase()}</Table.Td>
                <Table.Td>
                  <Text lineClamp={1} size="sm">
                    {email.title}
                  </Text>
                  <Text c="dimmed" lineClamp={1} size="xs">
                    sent at portal stage: {formatStageName(email.stage ?? "")}
                  </Text>
                </Table.Td>
                <Table.Td w={170}>
                  <Group gap={6} justify="flex-end" wrap="nowrap">
                    <Button size="compact-xs" variant="default" onClick={() => onPreview(email)}>
                      Preview
                    </Button>
                    <Button size="compact-xs" onClick={() => setPlacing(email)}>
                      Place…
                    </Button>
                  </Group>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
        </ScrollArea.Autosize>
      </Collapse>
      {placing ? (
        <PlaceModal board={board} columns={columns} email={placing} onClose={() => setPlacing(null)} />
      ) : null}
    </Paper>
  );
}

function PlaceModal({
  board,
  columns,
  email,
  onClose,
}: {
  board: Board;
  columns: StageColumn[];
  email: EmailListItem;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [stage, setStage] = useState<string | null>(null);
  const [slot, setSlot] = useState<string | null>(null);
  const groups = columns.find((c) => c.stage === stage)?.emailGroups ?? [];
  const chosenGroup = groups.find((g) => String(g.sortOrder) === slot);
  const sameLanguage = chosenGroup?.versions.filter((v) => v.language === email.language) ?? [];
  const english = chosenGroup?.versions.find((v) => v.language === "en");

  const mutation = useMutation({
    mutationFn: () =>
      placeEmail(email.id, {
        sequence: board.key,
        stage: stage ?? "",
        sort_order: slot && slot !== newSlotValue ? Number(slot) : undefined,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({ color: "green", message: `"${email.title}" placed.`, title: "Placed" });
      onClose();
    },
    onError: (error) =>
      notifications.show({
        color: "red",
        message: error instanceof ApiError ? error.message : "Try again.",
        title: "Not placed",
      }),
  });

  return (
    <Modal opened title={`Place "${email.title}" (${email.language.toUpperCase()})`} onClose={onClose}>
      <Stack>
        <Select
          allowDeselect={false}
          data={board.stages.map((s) => ({ label: formatStageName(s), value: s }))}
          label="Stage"
          value={stage}
          onChange={(value) => {
            setStage(value);
            setSlot(null);
          }}
        />
        <Select
          allowDeselect={false}
          data={[
            ...groups.map((g) => {
              const title = (g.versions.find((v) => v.language === "en") ?? g.versions[0]).title;
              const languages = [...new Set(g.versions.map((v) => v.language.toUpperCase()))].join(", ");
              return { label: `#${g.sortOrder} ${title} — ${languages}`, value: String(g.sortOrder) };
            }),
            { label: "New slot at the end of the stage", value: newSlotValue },
          ]}
          disabled={!stage}
          label="Slot"
          value={slot}
          onChange={setSlot}
        />
        {chosenGroup ? (
          <Text c="dimmed" size="sm">
            {email.language === "en"
              ? "It becomes an EN email of this slot."
              : english
                ? `It becomes a translation of the EN email "${english.title}".`
                : "The slot has no EN email yet, so it is not linked as a translation."}
          </Text>
        ) : null}
        {sameLanguage.length > 0 ? (
          <Alert color="yellow">
            This slot already has {email.language.toUpperCase()} (
            {sameLanguage.map((v) => v.variant).join(", ")}). The portal email is added next to it as the version
            “{email.variant}”; archive whichever is no longer needed.
          </Alert>
        ) : null}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!stage || !slot} loading={mutation.isPending} onClick={() => mutation.mutate()}>
            Place
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
