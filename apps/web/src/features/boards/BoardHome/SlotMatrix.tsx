import { useState } from "react";
import {
  ActionIcon,
  Badge,
  Checkbox,
  Group,
  Paper,
  ScrollArea,
  SegmentedControl,
  Stack,
  Table,
  Text,
  Tooltip,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { DotsSixVerticalIcon, PencilSimpleIcon, TranslateIcon } from "@phosphor-icons/react";

import { ApiError } from "../../../shared/api";
import { fetchReferenceDifferences, moveSlot } from "../../emails/api";
import { EmailStatusLine } from "../../emails/EmailStatusLine";
import { formatStageName } from "../../emails/stages";
import type {
  Board,
  BoardRegion,
  EmailListItem,
  EmailVersionGroup,
  ReferenceDifference,
  StageColumn,
} from "../../emails/types";
import { BulkActionBar } from "./BulkActionBar";
import { CreateTranslationModal } from "./CreateTranslationModal";
import { SlotEditModal } from "./SlotEditModal";
import { aiTranslationLanguages, matrixLanguages, pickPrimary } from "./slotHelpers";
import styles from "./SlotMatrix.module.css";

const stageDropPrefix = "stage:";

// SlotMatrix shows the sequence as slots (rows, grouped by stage) by
// languages (columns): which email goes out in each language, and the gaps.
// With regions the columns are the board's regions instead, the reference
// first: a map of who differs from the reference and how.
// Admins edit a slot as a whole, drag rows to reorder or change stage, and
// select cells, rows or stages to change many emails at once.
export function SlotMatrix({
  board,
  boardColumns,
  canManage,
  columns,
  selectedEmailId,
  canTranslate,
  onEmailCreated,
  onExportSelected,
  onOpenEmail,
  byRegion,
  regions,
  onByRegionChange,
}: {
  board?: Board;
  // boardColumns are all slots of the board; columns may be filtered.
  boardColumns: StageColumn[];
  canManage: boolean;
  columns: StageColumn[];
  selectedEmailId: string | null;
  // canTranslate tells whether the user may create emails in a language.
  canTranslate: (language: string) => boolean;
  onEmailCreated: (emailId: string) => void;
  onExportSelected: (emails: EmailListItem[]) => void;
  onOpenEmail: (email: EmailListItem, slotEmails: EmailListItem[]) => void;
  // byRegion shows the board's regions as columns: a map of who differs
  // from the reference. Offered when the board has regions.
  byRegion: boolean;
  regions: BoardRegion[];
  onByRegionChange: (byRegion: boolean) => void;
}) {
  const matrixColumns = byRegion ? regionColumns(regions, columns) : languageColumns(columns);
  const differencesQuery = useQuery({
    queryKey: ["emails", board?.key ?? "", "reference-differences"],
    queryFn: () => fetchReferenceDifferences(board?.key ?? ""),
    enabled: byRegion && Boolean(board),
  });
  const differences = differencesQuery.data ?? {};
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<EmailVersionGroup | null>(null);
  const [translating, setTranslating] = useState<{ master: EmailListItem; language: string } | null>(null);
  const [dragging, setDragging] = useState<EmailVersionGroup | null>(null);
  const [overId, setOverId] = useState<string | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor)
  );
  const canDrag = canManage && Boolean(board);

  const visibleGroups = columns.flatMap((c) => c.emailGroups);
  // A cell stands for the email it shows; filters decide which one that is
  // (e.g. the "old" version filter selects old emails).
  const [checkedIds, setCheckedIds] = useState<Set<string>>(() => new Set());
  const cellEmails = (group: EmailVersionGroup) =>
    matrixColumns.flatMap((column) => pickPrimary(column.emailsOf(group.versions)) ?? []);
  const shownEmails = visibleGroups.flatMap(cellEmails);
  // Emails that left the view (filtered out, archived) drop out of the
  // selection.
  const selected = shownEmails.filter((email) => checkedIds.has(email.id));
  const setChecked = (emails: EmailListItem[], checked: boolean) =>
    setCheckedIds((current) => {
      const next = new Set(current);
      for (const email of emails) {
        if (checked) next.add(email.id);
        else next.delete(email.id);
      }
      return next;
    });
  const selection = (emails: EmailListItem[]) => ({
    checked: emails.length > 0 && emails.every((e) => checkedIds.has(e.id)),
    indeterminate: emails.some((e) => checkedIds.has(e.id)) && !emails.every((e) => checkedIds.has(e.id)),
    onChange: (checked: boolean) => setChecked(emails, checked),
  });
  const indexOf = (key: string) => visibleGroups.findIndex((g) => g.key === key);
  const dropBelow = dragging && overId ? indexOf(dragging.key) < indexOf(overId) : false;

  const mutation = useMutation({
    mutationFn: moveSlot,
    onSuccess: (place) => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "green",
        message: `Now ${formatStageName(place.stage)} #${place.sort_order}.`,
        title: "Slot moved",
      });
    },
    onError: (error) => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "red",
        message: error instanceof ApiError ? error.message : "Try again.",
        title: "Slot not moved",
      });
    },
  });

  // A drop on a row puts the slot in its place: above it when dragging up,
  // below it when dragging down. A drop on a stage title puts it first.
  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    setDragging(null);
    setOverId(null);
    const source = visibleGroups.find((g) => g.key === active.id);
    if (!board || !source || !over || over.id === active.id) return;

    const overKey = String(over.id);
    let toStage: string;
    let before: number | null;
    if (overKey.startsWith(stageDropPrefix)) {
      toStage = overKey.slice(stageDropPrefix.length);
      const first = boardColumns.find((c) => c.stage === toStage)?.emailGroups[0];
      before = first && first.key !== source.key ? first.sortOrder : null;
      if (first?.key === source.key) return;
    } else {
      const target = visibleGroups.find((g) => g.key === overKey);
      if (!target) return;
      toStage = target.stage;
      if (indexOf(source.key) < indexOf(target.key)) {
        const stageGroups = boardColumns.find((c) => c.stage === target.stage)?.emailGroups ?? [];
        const next = stageGroups[stageGroups.findIndex((g) => g.key === target.key) + 1];
        before = next ? next.sortOrder : null;
      } else {
        before = target.sortOrder;
      }
    }
    mutation.mutate({
      sequence: board.key,
      stage: source.stage,
      sort_order: source.sortOrder,
      to_stage: toStage,
      before_sort_order: before,
    });
  };

  const editingMaster = editing ? slotMaster(editing) : undefined;

  return (
    <Paper
      className={`${styles.wrap} ${styles.matrix} ${selected.length > 0 ? styles.selecting : ""}`}
      radius="md"
    >
      <DndContext
        collisionDetection={closestCenter}
        sensors={sensors}
        onDragCancel={() => {
          setDragging(null);
          setOverId(null);
        }}
        onDragEnd={handleDragEnd}
        onDragOver={({ over }) => setOverId(over ? String(over.id) : null)}
        onDragStart={({ active }) => setDragging(visibleGroups.find((g) => g.key === active.id) ?? null)}
      >
        <ScrollArea className={styles.scroll} type="auto">
          <Table className={styles.table} stickyHeader withColumnBorders>
            <Table.Thead>
              <Table.Tr>
                <Table.Th className={styles.slotColumn}>
                  <Group gap="xs" justify="space-between" wrap="nowrap">
                    <span>Slot</span>
                    {regions.length > 0 ? (
                      <SegmentedControl
                        aria-label="Columns"
                        data={[
                          { label: "Languages", value: "languages" },
                          { label: "Regions", value: "regions" },
                        ]}
                        size="xs"
                        value={byRegion ? "regions" : "languages"}
                        onChange={(value) => onByRegionChange(value === "regions")}
                      />
                    ) : null}
                  </Group>
                </Table.Th>
                {matrixColumns.map((column) => (
                  <Table.Th key={column.key} className={styles.languageColumn}>
                    {column.tooltip ? (
                      <Tooltip label={column.tooltip}>
                        <span>{column.title}</span>
                      </Tooltip>
                    ) : (
                      column.title
                    )}
                  </Table.Th>
                ))}
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {columns.map((column) => [
                <StageRow
                  key={`stage-${column.stage}`}
                  colSpan={matrixColumns.length + 1}
                  column={column}
                  droppable={canDrag}
                  highlighted={overId === stageDropPrefix + column.stage}
                  selection={canDrag ? selection(column.emailGroups.flatMap(cellEmails)) : undefined}
                />,
                ...column.emailGroups.map((group) => (
                  <SlotRow
                    key={group.key}
                    canDrag={canDrag && !mutation.isPending}
                    canEdit={canDrag}
                    dropMark={overId === group.key && dragging?.key !== group.key ? (dropBelow ? "below" : "above") : null}
                    group={group}
                    isDragging={dragging?.key === group.key}
                    differences={differences}
                    matrixColumns={matrixColumns}
                    rowSelection={canDrag ? selection(cellEmails(group)) : undefined}
                    selectedEmailId={selectedEmailId}
                    isChecked={(id) => checkedIds.has(id)}
                    onCheck={canDrag ? (email, checked) => setChecked([email], checked) : undefined}
                    canTranslate={canTranslate}
                    onEdit={() => setEditing(group)}
                    onTranslate={(master, language) => setTranslating({ master, language })}
                    onOpenEmail={onOpenEmail}
                  />
                )),
              ])}
            </Table.Tbody>
          </Table>
        </ScrollArea>
        <DragOverlay dropAnimation={null}>
          {dragging ? (
            <Paper className={styles.dragOverlay} shadow="md" p="xs" radius="sm">
              <Text fw={600} size="sm">
                {slotMaster(dragging)?.title}
              </Text>
              <Text c="dimmed" size="xs">
                {formatStageName(dragging.stage)} #{dragging.sortOrder}
              </Text>
            </Paper>
          ) : null}
        </DragOverlay>
      </DndContext>
      {board && canDrag && selected.length > 0 ? (
        <BulkActionBar
          board={board}
          columns={boardColumns}
          selected={selected}
          onClear={() => setCheckedIds(new Set())}
          onExport={() => onExportSelected(selected)}
        />
      ) : null}
      {translating ? (
        <CreateTranslationModal
          language={translating.language}
          master={translating.master}
          onClose={() => setTranslating(null)}
          onCreated={onEmailCreated}
        />
      ) : null}
      {editing && editingMaster && board ? (
        <SlotEditModal
          board={board}
          columns={boardColumns}
          group={editing}
          master={editingMaster}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </Paper>
  );
}

// A column of the matrix: a language, or a region of the board.
type MatrixColumn = {
  key: string;
  title: string;
  tooltip?: string;
  // The emails of a slot that belong in the column.
  emailsOf: (versions: EmailListItem[]) => EmailListItem[];
  language?: string;
  region?: BoardRegion;
};

function languageColumns(columns: StageColumn[]): MatrixColumn[] {
  return matrixLanguages(columns).map((language) => ({
    key: language,
    title: language.toUpperCase(),
    emailsOf: (versions) => versions.filter((v) => v.language === language),
    language,
  }));
}

// regionColumns puts the reference first; emails without a region get a
// column of their own while there are any.
function regionColumns(regions: BoardRegion[], columns: StageColumn[]): MatrixColumn[] {
  const ordered = [...regions].sort((a, b) => Number(b.reference) - Number(a.reference));
  const known = new Set(regions.map((r) => r.key));
  const result: MatrixColumn[] = ordered.map((region) => ({
    key: `region:${region.key}`,
    title: `${region.name} · ${region.language.toUpperCase()}`,
    tooltip: [region.reference ? "The reference" : "", region.managers.length ? `Robots of ${region.managers.join(", ")}` : ""]
      .filter(Boolean)
      .join(" · ") || undefined,
    emailsOf: (versions) => versions.filter((v) => v.region_key === region.key),
    region,
  }));
  const unplaced = (v: EmailListItem) => !v.region_key || !known.has(v.region_key);
  if (columns.some((c) => c.emailGroups.some((g) => g.versions.some(unplaced)))) {
    result.push({
      key: "region:",
      title: "No region",
      tooltip: "Put these emails in a region in Details",
      emailsOf: (versions) => versions.filter(unplaced),
    });
  }
  return result;
}

type CellNote = { label: string; color: string; detail: string };

// regionNote tells how a region's email stands against the reference.
function regionNote(email: EmailListItem, difference: ReferenceDifference | undefined): CellNote | undefined {
  if (email.translation_of) {
    return { label: "translation", color: "dimmed", detail: "A translation of the EN email: same blocks, translated text." };
  }
  if (!email.based_on) {
    return { label: "own email", color: "violet", detail: "Not based on a reference email." };
  }
  if (!difference) {
    return undefined;
  }
  const added = difference.blocks.filter((b) => b.action === "added").map((b) => b.name || b.block);
  const removed = difference.blocks.filter((b) => b.action === "removed").map((b) => b.name || b.block);
  const parts = [
    difference.fields > 0 ? `${difference.fields} ${difference.fields === 1 ? "field" : "fields"}` : "",
    added.length || removed.length ? `${added.length ? `+${added.length}` : ""}${removed.length ? `−${removed.length}` : ""} blocks` : "",
    difference.timing ? "timing" : "",
  ].filter(Boolean);
  if (parts.length === 0) {
    return { label: "same as reference", color: "teal", detail: "Same fields, blocks and timing as the reference email." };
  }
  const detail = [
    difference.fields > 0 ? `${difference.fields} fields differ` : "",
    added.length ? `own blocks: ${added.join(", ")}` : "",
    removed.length ? `without: ${removed.join(", ")}` : "",
    difference.timing
      ? `timing ${difference.timing.own || "none"} (reference ${difference.timing.reference || "none"})`
      : "",
  ]
    .filter(Boolean)
    .join(" · ");
  return { label: `differs: ${parts.join(" · ")}`, color: "blue", detail };
}

function slotMaster(group: EmailVersionGroup) {
  return pickPrimary(group.versions.filter((v) => v.language === "en")) ?? pickPrimary(group.versions);
}

type Selection = { checked: boolean; indeterminate: boolean; onChange: (checked: boolean) => void };

function SelectBox({ label, selection }: { label: string; selection: Selection }) {
  return (
    <Checkbox
      aria-label={label}
      checked={selection.checked}
      className={styles.selectBox}
      indeterminate={selection.indeterminate}
      size="xs"
      onChange={(event) => selection.onChange(event.currentTarget.checked)}
      onClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
    />
  );
}

function StageRow({
  colSpan,
  column,
  droppable,
  highlighted,
  selection,
}: {
  colSpan: number;
  column: StageColumn;
  droppable: boolean;
  highlighted: boolean;
  selection?: Selection;
}) {
  const { setNodeRef } = useDroppable({ id: stageDropPrefix + column.stage, disabled: !droppable });
  return (
    <Table.Tr ref={setNodeRef} className={highlighted ? `${styles.stageRow} ${styles.dropAbove}` : styles.stageRow}>
      <Table.Td colSpan={colSpan}>
        <Group gap="xs" wrap="nowrap">
          {selection ? <SelectBox label={`Select every email of ${column.title}`} selection={selection} /> : null}
          <Text fw={700} size="sm">
            {column.title}
          </Text>
        </Group>
      </Table.Td>
    </Table.Tr>
  );
}

function SlotRow({
  canDrag,
  canEdit,
  dropMark,
  group,
  isDragging,
  differences,
  matrixColumns,
  rowSelection,
  selectedEmailId,
  isChecked,
  onCheck,
  canTranslate,
  onEdit,
  onTranslate,
  onOpenEmail,
}: {
  canDrag: boolean;
  canEdit: boolean;
  dropMark: "above" | "below" | null;
  group: EmailVersionGroup;
  isDragging: boolean;
  differences: Record<string, ReferenceDifference>;
  matrixColumns: MatrixColumn[];
  rowSelection?: Selection;
  selectedEmailId: string | null;
  isChecked: (emailId: string) => boolean;
  onCheck?: (email: EmailListItem, checked: boolean) => void;
  canTranslate: (language: string) => boolean;
  onEdit: () => void;
  onTranslate: (master: EmailListItem, language: string) => void;
  onOpenEmail: (email: EmailListItem, slotEmails: EmailListItem[]) => void;
}) {
  const { attributes, listeners, setNodeRef: setHandleRef } = useDraggable({ id: group.key, disabled: !canDrag });
  const { setNodeRef: setRowRef } = useDroppable({ id: group.key, disabled: !canEdit });
  const master = slotMaster(group);
  const englishMaster = pickPrimary(group.versions.filter((v) => v.language === "en"));
  const hasEnglish = Boolean(englishMaster);
  const rowClass = [
    isDragging ? styles.draggingRow : "",
    dropMark === "above" ? styles.dropAbove : "",
    dropMark === "below" ? styles.dropBelow : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <Table.Tr ref={setRowRef} className={rowClass || undefined}>
      <Table.Td className={styles.slotColumn}>
        <Group gap={4} wrap="nowrap" align="flex-start">
          {rowSelection ? <SelectBox label={`Select the emails of ${master?.title ?? "the slot"}`} selection={rowSelection} /> : null}
          {canEdit ? (
            <ActionIcon
              ref={setHandleRef}
              aria-label="Drag to move the slot"
              className={styles.dragHandle}
              color="gray"
              size="sm"
              variant="subtle"
              {...attributes}
              {...listeners}
            >
              <DotsSixVerticalIcon aria-hidden="true" size={16} />
            </ActionIcon>
          ) : null}
          <div className={styles.slotText}>
            <Text fw={600} lineClamp={1} size="sm">
              {master?.title}
            </Text>
            <Text c="dimmed" lineClamp={1} size="xs">
              #{group.sortOrder} · {master?.send_timing ?? "no timing"}
              {master?.send_condition ? ` · ${master.send_condition}` : ""}
            </Text>
          </div>
          {canEdit ? (
            <Tooltip label="Edit slot: name, place, timing for all languages">
              <ActionIcon aria-label="Edit slot" className={styles.editSlot} color="gray" size="sm" variant="subtle" onClick={onEdit}>
                <PencilSimpleIcon aria-hidden="true" size={14} />
              </ActionIcon>
            </Tooltip>
          ) : null}
        </Group>
      </Table.Td>
      {matrixColumns.map((column) => {
        const { language, region } = column;
        const emails = column.emailsOf(group.versions);
        const primary = pickPrimary(emails);
        const adapting = region && !region.reference;
        return (
          <MatrixCell
            key={column.key}
            cellEmails={emails}
            emptyLabel={language && hasEnglish ? "no translation" : undefined}
            note={adapting && primary ? regionNote(primary, differences[primary.id]) : undefined}
            onTranslate={
              language && englishMaster && aiTranslationLanguages.includes(language) && canTranslate(language)
                ? () => onTranslate(englishMaster, language)
                : undefined
            }
            isChecked={isChecked}
            selectedEmailId={selectedEmailId}
            onCheck={onCheck}
            onOpen={(email) => onOpenEmail(email, group.versions)}
          />
        );
      })}
    </Table.Tr>
  );
}

function MatrixCell({
  cellEmails,
  emptyLabel,
  note,
  isChecked,
  selectedEmailId,
  onCheck,
  onOpen,
  onTranslate,
}: {
  cellEmails: EmailListItem[];
  // emptyLabel marks a gap worth seeing; without it an empty cell is a dash.
  emptyLabel?: string;
  // note tells how a region's email stands against the reference.
  note?: CellNote;
  isChecked: (emailId: string) => boolean;
  selectedEmailId: string | null;
  onCheck?: (email: EmailListItem, checked: boolean) => void;
  onOpen: (email: EmailListItem) => void;
  // onTranslate is set when the user can fill this empty cell from EN.
  onTranslate?: () => void;
}) {
  const primary = pickPrimary(cellEmails);
  if (!primary) {
    return (
      <Table.Td className={emptyLabel ? styles.missing : styles.empty}>
        <Group gap={4} justify="space-between" wrap="nowrap">
          <Text c="dimmed" size="xs">
            {emptyLabel ?? "—"}
          </Text>
          {onTranslate ? (
            <Tooltip label="Create from EN">
              <ActionIcon aria-label="Create from EN" className={styles.editSlot} color="gray" size="sm" variant="subtle" onClick={onTranslate}>
                <TranslateIcon aria-hidden="true" size={14} />
              </ActionIcon>
            </Tooltip>
          ) : null}
        </Group>
      </Table.Td>
    );
  }
  const others = cellEmails.length - 1;
  const selected = cellEmails.some((e) => e.id === selectedEmailId);
  const checked = isChecked(primary.id);

  return (
    <Table.Td
      className={[styles.cell, selected ? styles.selected : "", checked ? styles.checked : ""].filter(Boolean).join(" ")}
      role="button"
      tabIndex={0}
      onClick={() => onOpen(primary)}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen(primary);
        }
      }}
    >
      {onCheck ? (
        <span className={styles.cellSelect}>
          <SelectBox
            label={`Select ${primary.language.toUpperCase()} ${primary.variant}`}
            selection={{ checked, indeterminate: false, onChange: (value) => onCheck(primary, value) }}
          />
        </span>
      ) : null}
      <Stack gap={4}>
        <EmailStatusLine email={primary} versions={cellEmails} />
        {note ? (
          <Tooltip label={note.detail} multiline maw={320}>
            <Text c={note.color} lineClamp={1} size="xs">
              {note.label}
            </Text>
          </Tooltip>
        ) : null}
        <Group gap={4} wrap="nowrap">
          <Text c="dimmed" size="xs">
            {primary.variant}
          </Text>
          {others > 0 ? (
            <Badge color="gray" size="xs" variant="outline">
              +{others}
            </Badge>
          ) : null}
          {primary.open_comment_count > 0 ? (
            <Badge color="orange" size="xs" variant="light">
              {primary.open_comment_count}
            </Badge>
          ) : null}
        </Group>
      </Stack>
    </Table.Td>
  );
}
