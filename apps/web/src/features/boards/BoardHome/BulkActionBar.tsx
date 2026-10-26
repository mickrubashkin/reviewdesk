import { useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Button,
  Checkbox,
  Group,
  List,
  Menu,
  Modal,
  Paper,
  ScrollArea,
  Select,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CaretDownIcon, XIcon } from "@phosphor-icons/react";

import { ApiError } from "../../../shared/api";
import { fetchAdminUsers } from "../../admin-users/api";
import { bulkUpdateEmails, type BulkEmailAction, type BulkEmailResult } from "../../emails/api";
import { formatStageName } from "../../emails/stages";
import type { Board, EmailListItem, StageColumn } from "../../emails/types";
import { pickPrimary } from "./slotHelpers";
import styles from "./SlotMatrix.module.css";

type Dialog = "owner" | "slot" | "timing" | "archive" | null;

const newSlotValue = "new";

// BulkActionBar applies one change to the emails selected in the matrix:
// owner, status, slot, timing and condition, archive.
export function BulkActionBar({
  board,
  columns,
  selected,
  onClear,
  onExport,
}: {
  board: Board;
  // columns are all slots of the board, for the slot picker.
  columns: StageColumn[];
  selected: EmailListItem[];
  onClear: () => void;
  onExport: () => void;
}) {
  const queryClient = useQueryClient();
  const [dialog, setDialog] = useState<Dialog>(null);
  const languages = [...new Set(selected.map((e) => e.language.toUpperCase()))].join(", ");

  const mutation = useMutation({
    mutationFn: (action: BulkEmailAction) =>
      bulkUpdateEmails({ ...action, sequence: board.key, email_ids: selected.map((e) => e.id) }),
    onSuccess: (result, action) => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      showResult(action, result);
      setDialog(null);
      if (action.action === "archive") onClear();
    },
    onError: (error) => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "red",
        message: error instanceof ApiError ? error.message : "Try again.",
        title: "Nothing changed",
      });
    },
  });

  return (
    <Paper className={styles.bulkBar} p="xs" radius={0}>
      <Group gap="xs" wrap="wrap">
        <Text fw={600} size="sm">
          {selected.length} selected
        </Text>
        <Text c="dimmed" size="xs">
          {languages}
        </Text>
        <Button size="compact-sm" variant="default" onClick={() => setDialog("owner")}>
          Owner
        </Button>
        <Menu position="top-start" withinPortal>
          <Menu.Target>
            <Button
              loading={mutation.isPending && mutation.variables?.action === "status"}
              rightSection={<CaretDownIcon aria-hidden="true" size={12} />}
              size="compact-sm"
              variant="default"
            >
              Status
            </Button>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item onClick={() => mutation.mutate({ action: "status", review_status: "draft" })}>Draft</Menu.Item>
            <Menu.Item onClick={() => mutation.mutate({ action: "status", review_status: "ready" })}>Ready</Menu.Item>
          </Menu.Dropdown>
        </Menu>
        <Button size="compact-sm" variant="default" onClick={() => setDialog("slot")}>
          Move to slot
        </Button>
        <Button size="compact-sm" variant="default" onClick={() => setDialog("timing")}>
          Timing
        </Button>
        <Button size="compact-sm" variant="default" onClick={onExport}>
          Export…
        </Button>
        <Button color="red" size="compact-sm" variant="light" onClick={() => setDialog("archive")}>
          Archive
        </Button>
        <Button
          color="gray"
          leftSection={<XIcon aria-hidden="true" size={12} />}
          ml="auto"
          size="compact-sm"
          variant="subtle"
          onClick={onClear}
        >
          Clear
        </Button>
      </Group>

      {dialog === "owner" ? (
        <OwnerDialog
          count={selected.length}
          pending={mutation.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(owner) => mutation.mutate({ action: "owner", owner_email: owner })}
        />
      ) : null}
      {dialog === "slot" ? (
        <SlotDialog
          board={board}
          columns={columns}
          pending={mutation.isPending}
          selected={selected}
          onClose={() => setDialog(null)}
          onSubmit={(stage, sortOrder) => mutation.mutate({ action: "slot", stage, sort_order: sortOrder })}
        />
      ) : null}
      {dialog === "timing" ? (
        <TimingDialog
          columns={columns}
          pending={mutation.isPending}
          selected={selected}
          onClose={() => setDialog(null)}
          onSubmit={(fields) => mutation.mutate({ action: "timing", ...fields })}
        />
      ) : null}
      {dialog === "archive" ? (
        <Modal opened title={`Archive ${plural(selected.length, "email")}?`} onClose={() => setDialog(null)}>
          <Stack>
            <Text c="dimmed" size="sm">
              They leave the board and stay in Archive, where an admin can restore them.
            </Text>
            <EmailList emails={selected} />
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setDialog(null)}>
                Cancel
              </Button>
              <Button color="red" loading={mutation.isPending} onClick={() => mutation.mutate({ action: "archive" })}>
                Archive
              </Button>
            </Group>
          </Stack>
        </Modal>
      ) : null}
    </Paper>
  );
}

function showResult(action: BulkEmailAction, result: BulkEmailResult) {
  const done: Record<BulkEmailAction["action"], string> = {
    owner: "Owner changed",
    status: "Status changed",
    slot: "Moved",
    timing: "Timing changed",
    archive: "Archived",
  };
  let message = `${plural(result.updated, "email")}.`;
  if (action.action === "slot" && result.stage) {
    message = `${plural(result.updated, "email")}, now ${formatStageName(result.stage)} #${result.sort_order}.`;
  }
  if (action.action === "timing") {
    message = `${plural(result.slots ?? 0, "slot")}, every language.`;
  }
  notifications.show({ color: "green", message, title: done[action.action] });
  if (result.skipped.length > 0) {
    notifications.show({
      autoClose: false,
      color: "yellow",
      title: `${result.skipped.length} not changed`,
      message: (
        <List size="xs">
          {result.skipped.map((s) => (
            <List.Item key={s.id}>
              {s.title} · {s.language.toUpperCase()}: {s.reason}
            </List.Item>
          ))}
        </List>
      ),
    });
  }
}

function plural(count: number, noun: string) {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

function EmailList({ emails }: { emails: EmailListItem[] }) {
  return (
    <ScrollArea.Autosize mah={240}>
      <List size="sm" spacing={2}>
        {emails.map((email) => (
          <List.Item key={email.id}>
            {email.title} · {email.language.toUpperCase()} · {email.variant}
          </List.Item>
        ))}
      </List>
    </ScrollArea.Autosize>
  );
}

function OwnerDialog({
  count,
  pending,
  onClose,
  onSubmit,
}: {
  count: number;
  pending: boolean;
  onClose: () => void;
  onSubmit: (owner: string | null) => void;
}) {
  const [owner, setOwner] = useState<string | null>(null);
  const usersQuery = useQuery({ queryKey: ["admin", "users"], queryFn: fetchAdminUsers });
  const options = (usersQuery.data ?? []).map((user) => ({
    label: `${user.name ? `${user.name} · ` : ""}${user.email}`,
    value: user.email,
  }));

  return (
    <Modal opened title={`Owner of ${plural(count, "email")}`} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit(owner);
        }}
      >
        <Stack>
          <Select
            clearable
            data={options}
            data-autofocus
            description="Empty removes the owner"
            label="Owner"
            nothingFoundMessage={usersQuery.isLoading ? "Loading users" : "No users found"}
            placeholder="No owner"
            searchable
            value={owner}
            onChange={setOwner}
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button loading={pending} type="submit">
              {owner ? "Set owner" : "Remove owner"}
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}

function SlotDialog({
  board,
  columns,
  pending,
  selected,
  onClose,
  onSubmit,
}: {
  board: Board;
  columns: StageColumn[];
  pending: boolean;
  selected: EmailListItem[];
  onClose: () => void;
  onSubmit: (stage: string, sortOrder?: number) => void;
}) {
  const [stage, setStage] = useState<string>(selected[0]?.stage ?? board.stages[0] ?? "");
  const [slot, setSlot] = useState<string>(newSlotValue);
  const stageGroups = columns.find((c) => c.stage === stage)?.emailGroups ?? [];
  const target = stageGroups.find((g) => String(g.sortOrder) === slot);
  // A slot holds one email per language, version and adaptation; the server
  // refuses the move otherwise, so say so before sending.
  const sameKind = (a: EmailListItem, b: EmailListItem) =>
    a.id !== b.id && a.language === b.language && a.variant === b.variant && a.adaptation_key === b.adaptation_key;
  const clashes = selected.filter(
    (email) =>
      selected.some((other) => sameKind(email, other)) ||
      (target?.versions ?? []).some((v) => sameKind(email, v) && !selected.some((s) => s.id === v.id))
  );
  const slotOptions = [
    { label: "New slot at the end of the stage", value: newSlotValue },
    ...stageGroups.map((g) => ({
      label: `#${g.sortOrder} ${(pickPrimary(g.versions.filter((v) => v.language === "en")) ?? g.versions[0])?.title ?? ""}`,
      value: String(g.sortOrder),
    })),
  ];

  return (
    <Modal opened title={`Move ${plural(selected.length, "email")}`} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit(stage, target?.sortOrder);
        }}
      >
        <Stack>
          <Text c="dimmed" size="sm">
            Each email keeps its language and takes the slot's timing and condition. Translations link to the slot's EN.
          </Text>
          <Select
            allowDeselect={false}
            data={board.stages.map((s) => ({ label: formatStageName(s), value: s }))}
            label="Stage"
            value={stage}
            onChange={(value) => {
              if (!value) return;
              setStage(value);
              setSlot(newSlotValue);
            }}
          />
          <Select allowDeselect={false} data={slotOptions} label="Slot" searchable value={slot} onChange={(v) => setSlot(v ?? newSlotValue)} />
          {clashes.length > 0 ? (
            <Alert color="red" variant="light">
              A slot holds one email per language and version:{" "}
              {[...new Set(clashes.map((e) => `${e.language.toUpperCase()} ${e.variant}`))].join(", ")} would be there
              twice. Archive one first or change the selection.
            </Alert>
          ) : null}
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button disabled={clashes.length > 0} loading={pending} type="submit">
              Move
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}

function TimingDialog({
  columns,
  pending,
  selected,
  onClose,
  onSubmit,
}: {
  columns: StageColumn[];
  pending: boolean;
  selected: EmailListItem[];
  onClose: () => void;
  onSubmit: (fields: { send_timing?: string; send_condition?: string }) => void;
}) {
  const [setTiming, setSetTiming] = useState(true);
  const [setCondition, setSetCondition] = useState(false);
  const [sendTiming, setSendTiming] = useState("");
  const [sendCondition, setSendCondition] = useState("");
  const slotCount = new Set(selected.map((e) => `${e.stage}#${e.sort_order}`)).size;
  const timingOptions = useMemo(
    () =>
      [
        ...new Set(
          columns
            .flatMap((c) => c.emailGroups.flatMap((g) => g.versions.map((v) => v.send_timing)))
            .filter((t): t is string => Boolean(t))
        ),
      ].sort(),
    [columns]
  );

  return (
    <Modal opened title="Timing and condition" onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit({
            send_timing: setTiming ? sendTiming.trim() : undefined,
            send_condition: setCondition ? sendCondition.trim() : undefined,
          });
        }}
      >
        <Stack>
          <Text c="dimmed" size="sm">
            Applies to {slotCount === 1 ? "the whole slot" : `${slotCount} whole slots`}, every language, not only the
            selected cells. Empty clears the value.
          </Text>
          <Checkbox checked={setTiming} label="Set send timing" onChange={(e) => setSetTiming(e.currentTarget.checked)} />
          {setTiming ? (
            <Autocomplete
              data={timingOptions}
              data-autofocus
              description="Delay as set in the robot"
              label="Send timing"
              value={sendTiming}
              onChange={setSendTiming}
            />
          ) : null}
          <Checkbox
            checked={setCondition}
            label="Set send condition"
            onChange={(e) => setSetCondition(e.currentTarget.checked)}
          />
          {setCondition ? (
            <TextInput
              description="Robot condition, if any"
              label="Send condition"
              value={sendCondition}
              onChange={(e) => setSendCondition(e.currentTarget.value)}
            />
          ) : null}
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button disabled={!setTiming && !setCondition} loading={pending} type="submit">
              Apply
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
