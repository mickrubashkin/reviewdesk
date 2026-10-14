import { Badge, Group, Paper, ScrollArea, Stack, Table, Text, Tooltip } from "@mantine/core";

import { ProductionBadge } from "../../emails/ProductionBadge";
import type { EmailListItem, StageColumn } from "../../emails/types";
import { matrixLanguages, pickPrimary } from "./slotHelpers";
import styles from "./SlotMatrix.module.css";

// SlotMatrix shows the sequence as slots (rows, grouped by stage) by
// languages (columns): which email goes out in each language, and the gaps.
export function SlotMatrix({
  columns,
  selectedEmailId,
  onOpenEmail,
}: {
  columns: StageColumn[];
  selectedEmailId: string | null;
  onOpenEmail: (email: EmailListItem, slotEmails: EmailListItem[]) => void;
}) {
  const languages = matrixLanguages(columns);

  return (
    <Paper className={`${styles.wrap} ${styles.matrix}`} radius="md">
      <ScrollArea className={styles.scroll} type="auto">
        <Table className={styles.table} stickyHeader withColumnBorders>
          <Table.Thead>
            <Table.Tr>
              <Table.Th className={styles.slotColumn}>Slot</Table.Th>
              {languages.map((language) => (
                <Table.Th key={language} className={styles.languageColumn}>
                  {language.toUpperCase()}
                </Table.Th>
              ))}
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {columns.map((column) => [
              <Table.Tr key={`stage-${column.stage}`} className={styles.stageRow}>
                <Table.Td colSpan={languages.length + 1}>
                  <Text fw={700} size="sm">
                    {column.title}
                  </Text>
                </Table.Td>
              </Table.Tr>,
              ...column.emailGroups.map((group) => {
                const master = pickPrimary(group.versions.filter((v) => v.language === "en")) ?? pickPrimary(group.versions);
                return (
                  <Table.Tr key={group.key}>
                    <Table.Td className={styles.slotColumn}>
                      <Text fw={600} lineClamp={1} size="sm">
                        {master?.title}
                      </Text>
                      <Text c="dimmed" lineClamp={1} size="xs">
                        #{group.sortOrder} · {master?.send_timing ?? "no timing"}
                        {master?.send_condition ? ` · ${master.send_condition}` : ""}
                      </Text>
                    </Table.Td>
                    {languages.map((language) => (
                      <MatrixCell
                        key={language}
                        cellEmails={group.versions.filter((v) => v.language === language)}
                        hasEnglish={group.versions.some((v) => v.language === "en")}
                        selectedEmailId={selectedEmailId}
                        onOpen={(email) => onOpenEmail(email, group.versions)}
                      />
                    ))}
                  </Table.Tr>
                );
              }),
            ])}
          </Table.Tbody>
        </Table>
      </ScrollArea>
    </Paper>
  );
}

function MatrixCell({
  cellEmails,
  hasEnglish,
  selectedEmailId,
  onOpen,
}: {
  cellEmails: EmailListItem[];
  hasEnglish: boolean;
  selectedEmailId: string | null;
  onOpen: (email: EmailListItem) => void;
}) {
  const primary = pickPrimary(cellEmails);
  if (!primary) {
    return (
      <Table.Td className={hasEnglish ? styles.missing : styles.empty}>
        <Text c="dimmed" size="xs">
          {hasEnglish ? "no translation" : "—"}
        </Text>
      </Table.Td>
    );
  }
  const others = cellEmails.length - 1;
  const selected = cellEmails.some((e) => e.id === selectedEmailId);

  return (
    <Table.Td
      className={selected ? `${styles.cell} ${styles.selected}` : styles.cell}
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
      <Stack gap={4}>
        <Group gap={4}>
          <ProductionBadge email={primary} versions={cellEmails} />
          {primary.translation_stale ? (
            <Tooltip label="The EN text changed after this translation; open to see what changed">
              <Badge color="red" size="xs" variant="light">
                EN changed
              </Badge>
            </Tooltip>
          ) : null}
        </Group>
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
