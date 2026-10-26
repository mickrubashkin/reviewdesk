import { useState } from "react";
import {
  Alert,
  Button,
  Checkbox,
  Group,
  Modal,
  Radio,
  SegmentedControl,
  Stack,
  Text,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation } from "@tanstack/react-query";

import { downloadEmailsExport } from "../../emails/api";
import { allMarkdownSections, markdownSectionOptions } from "../../emails/markdownSections";
import type { AuthUser, Board, EmailListItem } from "../../emails/types";
import { buildSequenceHandoffManifest, copyPlainText } from "./BoardHome.helpers";
import { useBoardRegions } from "../api";
import { buildChainTable, chainTableCSV, chainTableXLSX } from "./chainTable";
import { canManageBoard } from "../../emails/permissions";

export type ExportScope = "board" | "filtered" | "selected";

type ExportFormat = "table" | "html" | "md" | "json";

// The API exports at most this many emails as HTML or Markdown at once.
const maxServerExport = 500;

const formats: { value: ExportFormat; label: string; hint: string; adminOnly: boolean }[] = [
  {
    value: "table",
    label: "Chain table",
    hint: "One row per email in sequence order: slot, timing, language, status, subject, owner, link.",
    adminOnly: false,
  },
  {
    value: "html",
    label: "HTML (zip)",
    hint: "The final HTML of each email, as it goes into Bitrix24.",
    adminOnly: true,
  },
  {
    value: "md",
    label: "Markdown for AI (zip)",
    hint: "The copy, links, notes and comments of each email, plus one combined file.",
    adminOnly: true,
  },
  {
    value: "json",
    label: "JSON handoff",
    hint: "The list with statuses and counts as JSON, for scripts and other tools.",
    adminOnly: false,
  },
];

function stamp() {
  return new Date().toISOString().slice(0, 10);
}

function downloadBlob(blob: Blob, fileName: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

// ExportModal is the one place to export from a board: pick what (the whole
// board, what the filters show, or what is selected in the matrix) and how.
export function ExportModal({
  board,
  boardEmails,
  currentUser,
  filteredEmails,
  initialScope,
  selectedEmails,
  onClose,
  onOpenBoardPackForAI,
}: {
  board: Board;
  boardEmails: EmailListItem[];
  currentUser: AuthUser;
  filteredEmails: EmailListItem[];
  initialScope: ExportScope;
  selectedEmails: EmailListItem[];
  onClose: () => void;
  onOpenBoardPackForAI: () => void;
}) {
  const regions = useBoardRegions(board.key).data ?? [];
  const isAdmin = canManageBoard(currentUser, board.key);
  const [scope, setScope] = useState<ExportScope>(initialScope);
  const [format, setFormat] = useState<ExportFormat>("table");
  const [tableType, setTableType] = useState<"xlsx" | "csv">("xlsx");
  const [sections, setSections] = useState(allMarkdownSections);
  const [openCommentsOnly, setOpenCommentsOnly] = useState(false);

  const scopes: { value: ExportScope; label: string; emails: EmailListItem[] }[] = [
    { value: "board", label: "Whole board", emails: boardEmails },
    ...(filteredEmails.length !== boardEmails.length
      ? [{ value: "filtered" as const, label: "Current filters", emails: filteredEmails }]
      : []),
    ...(selectedEmails.length > 0
      ? [{ value: "selected" as const, label: "Selected", emails: selectedEmails }]
      : []),
  ];
  const activeScope = scopes.find((option) => option.value === scope) ?? scopes[0];
  const emails = activeScope.emails;
  const isServerFormat = format === "html" || format === "md";
  const tooMany = isServerFormat && emails.length > maxServerExport;
  const baseName = `${board.key}-${activeScope.value}-${stamp()}`;

  const handoffJSON = () =>
    JSON.stringify(buildSequenceHandoffManifest(board.key, board.name, emails), null, 2);

  const exportMutation = useMutation({
    mutationFn: async () => {
      if (format === "table") {
        const table = buildChainTable(emails, board.stages, window.location.origin, regions);
        if (tableType === "csv") {
          downloadBlob(new Blob([chainTableCSV(table)], { type: "text/csv;charset=utf-8" }), `${baseName}.csv`);
        } else {
          downloadBlob(await chainTableXLSX(table), `${baseName}.xlsx`);
        }
        return;
      }
      if (format === "json") {
        downloadBlob(new Blob([handoffJSON()], { type: "application/json;charset=utf-8" }), `${baseName}.json`);
        return;
      }
      const { blob, fileName } = await downloadEmailsExport(
        emails.map((email) => email.id),
        format,
        format === "md" ? { openCommentsOnly, sections } : undefined
      );
      downloadBlob(blob, fileName);
    },
    onSuccess: onClose,
    onError: (error) =>
      notifications.show({ color: "red", message: error.message, title: "Export failed" }),
  });

  return (
    <Modal opened size="lg" title={`Export from "${board.name}"`} onClose={onClose}>
      <Stack>
        <Stack gap={6}>
          <Text fw={600} size="sm">
            What
          </Text>
          <SegmentedControl
            data={scopes.map((option) => ({
              label: `${option.label} · ${option.emails.length}`,
              value: option.value,
            }))}
            value={activeScope.value}
            onChange={(value) => setScope(value as ExportScope)}
          />
          <Text c="dimmed" size="xs">
            Archived emails are not included. Every version of a slot is included; narrow it with the board filters or
            select cells in the matrix.
          </Text>
        </Stack>

        <Radio.Group label="How" value={format} onChange={(value) => setFormat(value as ExportFormat)}>
          <Stack gap="xs" mt={6}>
            {formats.map((option) => {
              const disabled = option.adminOnly && !isAdmin;
              return (
                <Radio.Card key={option.value} disabled={disabled} p="sm" radius="sm" value={option.value}>
                  <Group align="flex-start" gap="sm" wrap="nowrap">
                    <Radio.Indicator disabled={disabled} />
                    <Stack gap={2}>
                      <Text fw={600} size="sm">
                        {option.label}
                      </Text>
                      <Text c="dimmed" size="xs">
                        {disabled ? "Admins only." : option.hint}
                      </Text>
                    </Stack>
                  </Group>
                </Radio.Card>
              );
            })}
          </Stack>
        </Radio.Group>

        {format === "table" ? (
          <SegmentedControl
            data={[
              { label: "Excel (XLSX)", value: "xlsx" },
              { label: "CSV", value: "csv" },
            ]}
            value={tableType}
            onChange={(value) => setTableType(value as "xlsx" | "csv")}
          />
        ) : null}

        {format === "md" ? (
          <Stack gap="xs">
            <Checkbox.Group value={sections} onChange={setSections}>
              <Stack gap={6}>
                {markdownSectionOptions.map((option) => (
                  <Checkbox key={option.value} label={option.label} size="xs" value={option.value} />
                ))}
              </Stack>
            </Checkbox.Group>
            <Checkbox
              checked={openCommentsOnly}
              disabled={!sections.includes("comments")}
              label="Only open comments"
              ml="md"
              size="xs"
              onChange={(event) => setOpenCommentsOnly(event.currentTarget.checked)}
            />
          </Stack>
        ) : null}

        {tooMany ? (
          <Alert color="orange" variant="light">
            HTML and Markdown export at most {maxServerExport} emails at once. Narrow it with the filters or a
            selection.
          </Alert>
        ) : null}

        {currentUser.role === "super_admin" ? (
          <Text c="dimmed" size="xs">
            Need the board as one document with the AI rules for an AI chat?{" "}
            <Button size="compact-xs" variant="subtle" onClick={onOpenBoardPackForAI}>
              Board pack for AI…
            </Button>
          </Text>
        ) : null}

        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          {format === "json" ? (
            <Button
              disabled={emails.length === 0}
              variant="light"
              onClick={() => void copyPlainText(handoffJSON(), "Sequence handoff")}
            >
              Copy JSON
            </Button>
          ) : null}
          <Button
            disabled={emails.length === 0 || tooMany || (format === "md" && sections.length === 0)}
            loading={exportMutation.isPending}
            onClick={() => exportMutation.mutate()}
          >
            Download {emails.length} {emails.length === 1 ? "email" : "emails"}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
