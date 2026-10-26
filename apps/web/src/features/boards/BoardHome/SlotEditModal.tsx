import { useMemo, useState } from "react";
import { Alert, Autocomplete, Button, Group, Modal, NumberInput, Select, Stack, Text, TextInput } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { ApiError } from "../../../shared/api";
import { updateSlot } from "../../emails/api";
import { formatStageName } from "../../emails/stages";
import type { Board, EmailListItem, EmailVersionGroup, StageColumn } from "../../emails/types";

// SlotEditModal edits what a slot shares across languages: name, stage,
// position, timing and condition. Saving writes them to every language.
export function SlotEditModal({
  board,
  columns,
  group,
  master,
  onClose,
}: {
  board: Board;
  columns: StageColumn[];
  group: EmailVersionGroup;
  master: EmailListItem;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [title, setTitle] = useState(master.title);
  const [stage, setStage] = useState(group.stage);
  const [position, setPosition] = useState<number | string>(group.sortOrder);
  const [sendTiming, setSendTiming] = useState(master.send_timing ?? "");
  const [sendCondition, setSendCondition] = useState(master.send_condition ?? "");

  const timingOptions = useMemo(
    () =>
      [
        ...new Set(
          columns.flatMap((c) => c.emailGroups.flatMap((g) => g.versions.map((v) => v.send_timing)))
            .filter((t): t is string => Boolean(t))
        ),
      ].sort(),
    [columns]
  );
  const languages = [...new Set(group.versions.map((v) => v.language.toUpperCase()))].join(", ");
  // Languages that disagree today get the master's values on save.
  const differing = group.versions.some(
    (v) =>
      v.title !== master.title ||
      (v.send_timing ?? "") !== (master.send_timing ?? "") ||
      (v.send_condition ?? "") !== (master.send_condition ?? "")
  );
  const stageChanged = stage !== group.stage;
  const positionValue = typeof position === "number" ? position : undefined;
  const targetPosition = stageChanged && position === "" ? undefined : positionValue;
  const takenBy = columns
    .find((c) => c.stage === stage)
    ?.emailGroups.find((g) => g.sortOrder === targetPosition && g.key !== group.key);

  const mutation = useMutation({
    mutationFn: () =>
      updateSlot({
        sequence: board.key,
        stage: group.stage,
        sort_order: group.sortOrder,
        title: title.trim(),
        to_stage: stage,
        to_sort_order: targetPosition,
        send_timing: sendTiming.trim(),
        send_condition: sendCondition.trim(),
      }),
    onSuccess: (place) => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "green",
        message: `${formatStageName(place.stage)} #${place.sort_order}, ${languages}.`,
        title: "Slot saved",
      });
      onClose();
    },
    onError: (error) =>
      notifications.show({
        color: "red",
        message: error instanceof ApiError ? error.message : "Try again.",
        title: "Slot not saved",
      }),
  });

  return (
    <Modal opened title={`Edit slot "${master.title}"`} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          mutation.mutate();
        }}
      >
        <Stack>
          <Text c="dimmed" size="sm">
            Applies to every language of the slot: {languages}.
          </Text>
          {differing ? (
            <Alert color="yellow" variant="light">
              Languages have different names, timing or condition now. Saving sets them all to these values.
            </Alert>
          ) : null}
          <TextInput
            label="Name"
            required
            value={title}
            onChange={(e) => setTitle(e.currentTarget.value)}
          />
          <Group grow align="flex-start">
            <Select
              allowDeselect={false}
              data={board.stages.map((s) => ({ label: formatStageName(s), value: s }))}
              label="Stage"
              value={stage}
              onChange={(value) => {
                if (!value) return;
                setStage(value);
                setPosition(value === group.stage ? group.sortOrder : "");
              }}
            />
            <NumberInput
              allowDecimal={false}
              allowNegative={false}
              description={stageChanged && position === "" ? "Empty: end of the stage" : undefined}
              error={takenBy ? `Taken by "${takenBy.versions[0]?.title}"` : undefined}
              label="Position"
              value={position}
              onChange={setPosition}
            />
          </Group>
          <Autocomplete
            data={timingOptions}
            description="Delay as set in the robot"
            label="Send timing"
            value={sendTiming}
            onChange={setSendTiming}
          />
          <TextInput
            description="Robot condition, if any"
            label="Send condition"
            value={sendCondition}
            onChange={(e) => setSendCondition(e.currentTarget.value)}
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button disabled={!title.trim() || Boolean(takenBy)} loading={mutation.isPending} type="submit">
              Save
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
