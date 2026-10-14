import { useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Checkbox,
  Divider,
  Group,
  Modal,
  MultiSelect,
  ScrollArea,
  Stack,
  Switch,
  Text,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation } from "@tanstack/react-query";

import { exportBoardForAI, type BoardAIExportResult } from "../../ai-config/api";
import {
  allMarkdownSections,
  markdownSectionOptions,
} from "../../emails/markdownSections";
import { buildStageColumns } from "../../emails/stages";
import type { Board, EmailListItem } from "../../emails/types";

import { downloadTextFile } from "../../ai-config/AIConfig/exportConfig";
import { formatEmailReviewStatus } from "../../emails/reviewStatus";

const tokenWarningThreshold = 100_000;

// Same convention the board uses for its default view: v1, else new.
function defaultVariant(variants: string[]) {
  return variants.find((v) => v === "v1") ?? variants.find((v) => v === "new") ?? variants[0];
}

function estimateTokens(text: string) {
  return Math.round(text.length / 4);
}

export function BoardAIExportModal({
  board,
  emails,
  onClose,
}: {
  board: Board;
  emails: EmailListItem[];
  onClose: () => void;
}) {
  const languages = useMemo(() => uniqueSorted(emails.map((e) => e.language)), [emails]);
  const variants = useMemo(() => uniqueSorted(emails.map((e) => e.variant)), [emails]);
  const adaptations = useMemo(
    () =>
      Array.from(
        new Map(emails.map((e) => [e.adaptation_key, e.adaptation_label])).entries()
      ).map(([value, label]) => ({ label, value })),
    [emails]
  );

  const [selectedLanguages, setSelectedLanguages] = useState(
    languages.includes("en") ? ["en"] : languages
  );
  const [selectedVariants, setSelectedVariants] = useState(() => {
    const variant = defaultVariant(variants);
    return variant ? [variant] : [];
  });
  const [selectedAdaptations, setSelectedAdaptations] = useState(
    adaptations.map((a) => a.value)
  );
  const [excluded, setExcluded] = useState<Set<string>>(new Set());
  const [includeBoard, setIncludeBoard] = useState(true);
  const [includeAISetup, setIncludeAISetup] = useState(true);
  const [emailSections, setEmailSections] = useState(allMarkdownSections);
  const [openCommentsOnly, setOpenCommentsOnly] = useState(false);
  const [result, setResult] = useState<{
    signature: string;
    data: BoardAIExportResult;
  } | null>(null);

  const matches = (email: EmailListItem) =>
    selectedLanguages.includes(email.language) &&
    selectedVariants.includes(email.variant) &&
    selectedAdaptations.includes(email.adaptation_key);

  const columns = useMemo(
    () => buildStageColumns(emails, board.stages),
    [emails, board.stages]
  );
  const selectedIds = emails
    .filter((email) => matches(email) && !excluded.has(email.id))
    .map((email) => email.id);

  const payload = {
    email_ids: selectedIds,
    include_board: includeBoard,
    include_ai_setup: includeAISetup,
    email_sections: emailSections,
    open_comments_only: openCommentsOnly,
  };
  const signature = JSON.stringify(payload);
  const currentResult = result?.signature === signature ? result.data : null;

  const exportMutation = useMutation({
    mutationFn: () => exportBoardForAI(board.key, payload),
    onSuccess: (data) => setResult({ data, signature }),
    onError: () =>
      notifications.show({
        color: "red",
        message: "Try again or check that you are a super admin.",
        title: "Export failed",
      }),
  });

  const toggle = (ids: string[], checked: boolean) =>
    setExcluded((current) => {
      const next = new Set(current);
      ids.forEach((id) => (checked ? next.delete(id) : next.add(id)));
      return next;
    });

  const applyPreset = (preset: "default" | "all") => {
    setExcluded(new Set());
    setSelectedVariants(
      preset === "all" ? variants : [defaultVariant(variants)].filter(Boolean)
    );
  };

  const tokens = currentResult ? estimateTokens(currentResult.markdown) : 0;

  return (
    <Modal
      opened
      size="xl"
      title={`Export "${board.name}" for AI`}
      onClose={onClose}
    >
      <Stack>
        <Group>
          <Switch
            checked={includeBoard}
            label="Board overview and stages"
            onChange={(e) => setIncludeBoard(e.currentTarget.checked)}
          />
          <Switch
            checked={includeAISetup}
            label="AI rules, roles and instruction"
            onChange={(e) => setIncludeAISetup(e.currentTarget.checked)}
          />
        </Group>

        <Divider label="Which emails" labelPosition="left" />
        <Group grow align="flex-start">
          <MultiSelect
            data={languages.map((l) => ({ label: l.toUpperCase(), value: l }))}
            label="Language"
            value={selectedLanguages}
            onChange={setSelectedLanguages}
          />
          <MultiSelect
            data={variants}
            label="Version"
            value={selectedVariants}
            onChange={setSelectedVariants}
          />
          <MultiSelect
            data={adaptations}
            label="Adaptation"
            value={selectedAdaptations}
            onChange={setSelectedAdaptations}
          />
        </Group>
        <Group gap="xs">
          <Button size="compact-xs" variant="light" onClick={() => applyPreset("default")}>
            Default version only
          </Button>
          <Button size="compact-xs" variant="light" onClick={() => applyPreset("all")}>
            All versions
          </Button>
        </Group>

        <ScrollArea.Autosize mah={320} type="auto">
          <Stack gap="sm" pr="sm">
            {columns.map((column) => {
              const rows = column.emailGroups.flatMap((group) =>
                group.versions.filter(matches)
              );
              const checkedCount = rows.filter((e) => !excluded.has(e.id)).length;
              return (
                <Stack key={column.stage} gap={4}>
                  <Checkbox
                    checked={rows.length > 0 && checkedCount === rows.length}
                    disabled={rows.length === 0}
                    indeterminate={checkedCount > 0 && checkedCount < rows.length}
                    label={
                      <Text fw={600} size="sm">
                        {column.title}{" "}
                        <Text c="dimmed" component="span" size="xs">
                          {checkedCount}/{rows.length}
                        </Text>
                      </Text>
                    }
                    onChange={(e) => toggle(rows.map((r) => r.id), e.currentTarget.checked)}
                  />
                  {column.emailGroups.map((group) => {
                    const groupRows = group.versions.filter(matches);
                    if (groupRows.length === 0) {
                      return (
                        <Text key={group.key} c="dimmed" ml="xl" size="xs">
                          {group.versions[0]?.title} — no email matches the filters
                        </Text>
                      );
                    }
                    return groupRows.map((email) => (
                      <Checkbox
                        key={email.id}
                        checked={!excluded.has(email.id)}
                        ml="xl"
                        size="xs"
                        label={
                          <Group gap={6} wrap="nowrap">
                            <Text size="xs">{email.title}</Text>
                            <Badge size="xs" variant="light">
                              {email.language.toUpperCase()} · {email.variant}
                            </Badge>
                            {email.adaptation_key !== "default" ? (
                              <Badge color="grape" size="xs" variant="light">
                                {email.adaptation_label}
                              </Badge>
                            ) : null}
                            <Text c="dimmed" size="xs">
                              {formatEmailReviewStatus(email.review_status)}
                            </Text>
                          </Group>
                        }
                        onChange={(e) => toggle([email.id], e.currentTarget.checked)}
                      />
                    ));
                  })}
                </Stack>
              );
            })}
          </Stack>
        </ScrollArea.Autosize>

        <Divider label="What to include for each email" labelPosition="left" />
        <Checkbox.Group value={emailSections} onChange={setEmailSections}>
          <Group gap="md">
            {markdownSectionOptions.map((option) => (
              <Checkbox key={option.value} label={option.label.split(" (")[0]} value={option.value} />
            ))}
          </Group>
        </Checkbox.Group>
        <Checkbox
          checked={openCommentsOnly}
          disabled={!emailSections.includes("comments")}
          label="Only open comments"
          ml="md"
          onChange={(e) => setOpenCommentsOnly(e.currentTarget.checked)}
        />

        <Divider />
        <Group justify="space-between">
          <Text size="sm">{selectedIds.length} emails selected</Text>
          <Button
            loading={exportMutation.isPending}
            onClick={() => exportMutation.mutate()}
          >
            Generate document
          </Button>
        </Group>

        {currentResult ? (
          <Stack gap="xs">
            <Text size="sm">
              {currentResult.email_count} emails · {(currentResult.markdown.length / 1024).toFixed(0)} KB · ≈{" "}
              {tokens.toLocaleString()} tokens
            </Text>
            {tokens > tokenWarningThreshold ? (
              <Alert color="yellow">
                This is large and may not fit into a model context. Narrow the
                languages, versions or stages.
              </Alert>
            ) : null}
            <Group>
              <Button
                variant="default"
                onClick={() =>
                  downloadTextFile(currentResult.markdown, currentResult.file_name, "text/markdown")
                }
              >
                Download .md
              </Button>
              <Button
                variant="default"
                onClick={() =>
                  void navigator.clipboard
                    .writeText(currentResult.markdown)
                    .then(() =>
                      notifications.show({ color: "green", message: "Document copied.", title: "Copied" })
                    )
                    .catch(() =>
                      notifications.show({ color: "red", message: "Browser blocked clipboard access.", title: "Copy failed" })
                    )
                }
              >
                Copy to clipboard
              </Button>
            </Group>
          </Stack>
        ) : null}
      </Stack>
    </Modal>
  );
}

function uniqueSorted(values: string[]) {
  return Array.from(new Set(values.filter(Boolean))).sort((a, b) => a.localeCompare(b));
}
