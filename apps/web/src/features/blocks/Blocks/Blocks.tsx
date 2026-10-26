import { useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Modal,
  Paper,
  SimpleGrid,
  Stack,
  Text,
  Textarea,
  Tooltip,
} from "@mantine/core";
import { useDebouncedValue } from "@mantine/hooks";
import { notifications } from "@mantine/notifications";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";

import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import type { AuthUser } from "../../emails/types";
import {
  blocksQueryKey,
  frameKey,
  previewBlock,
  updateBlock,
  useBlocks,
  type BlockField,
  type EmailBlock,
} from "../api";
import pageStyles from "../../auth-events/AuthEvents/AuthEvents.module.css";
import styles from "./Blocks.module.css";

const fieldTypeLabels: Record<BlockField["type"], string> = {
  text: "text",
  url: "link",
  image: "image",
  number: "number",
};

export function Blocks({ currentUserRole }: { currentUserRole: AuthUser["role"] }) {
  const isSuperAdmin = currentUserRole === "super_admin";
  const blocksQuery = useBlocks(isSuperAdmin);
  const [editingKey, setEditingKey] = useState<string | null>(null);

  if (!isSuperAdmin) {
    return <Navigate replace to="/" />;
  }

  const editing = blocksQuery.data?.find((block) => block.key === editingKey);

  return (
    <div className={pageStyles.page}>
      <AdminTableHeader
        subtitle="The pieces emails are built from: a frame and blocks that go into it in order. Cut from the most common template, so an email built from them looks the same."
        title="Blocks"
      />
      <Stack maw={960} mx="auto" p="md">
        <Text c="dimmed" size="sm">
          Saving a block writes a new version of every email built with it, like a shared text: live
          emails then need a re-upload to Bitrix24. Fields come from the <code>data-edit-*</code>{" "}
          attributes in the HTML.
        </Text>
        {blocksQuery.isLoading ? (
          <Loader />
        ) : blocksQuery.isError ? (
          <Alert color="red">Failed to load blocks.</Alert>
        ) : (
          (blocksQuery.data ?? []).map((block) => (
            <BlockRow key={block.key} block={block} onOpen={() => setEditingKey(block.key)} />
          ))
        )}
      </Stack>
      {editing ? (
        <BlockEditor key={editing.key} block={editing} onClose={() => setEditingKey(null)} />
      ) : null}
    </div>
  );
}

function BlockRow({ block, onOpen }: { block: EmailBlock; onOpen: () => void }) {
  return (
    <Paper p="sm" withBorder>
      <Group align="flex-start" justify="space-between" wrap="nowrap">
        <Stack gap={6} miw={0}>
          <Group gap="xs">
            <Text fw={700} size="sm">
              {block.name}
            </Text>
            <Text c="dimmed" ff="monospace" size="xs">
              {block.key}
            </Text>
          </Group>
          <Text c="dimmed" size="sm">
            {block.description}
          </Text>
          {block.fields.length > 0 ? (
            <Group gap={6}>
              {block.fields.map((field) => (
                <FieldBadge key={field.key} field={field} />
              ))}
            </Group>
          ) : null}
          <Text c="dimmed" size="xs">
            {formatUsage(block)} · v{block.version}
            {block.updated_by
              ? ` · ${block.updated_by}, ${new Date(block.updated_at).toLocaleString()}`
              : ""}
          </Text>
        </Stack>
        <Button size="xs" variant="light" onClick={onOpen}>
          Open
        </Button>
      </Group>
    </Paper>
  );
}

function formatUsage(block: EmailBlock) {
  if (block.emails === 0) {
    return "No emails yet";
  }
  const emails = `${block.emails} ${block.emails === 1 ? "email" : "emails"}`;
  return block.live_emails > 0 ? `${emails}, ${block.live_emails} live` : emails;
}

function FieldBadge({ field }: { field: BlockField }) {
  const details = [
    field.key,
    fieldTypeLabels[field.type],
    field.required ? "required" : "",
    field.max_length ? `up to ${field.max_length} characters` : "",
    field.shared ? "shared text by default" : "",
  ].filter(Boolean);
  return (
    <Tooltip label={details.join(" · ")}>
      <Badge color={field.shared ? "grape" : "gray"} tt="none" variant="light">
        {field.label}
        {field.required ? " *" : ""}
      </Badge>
    </Tooltip>
  );
}

function BlockEditor({ block, onClose }: { block: EmailBlock; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [html, setHTML] = useState(block.html);
  const [debouncedHTML] = useDebouncedValue(html, 400);
  const changed = html !== block.html;

  const previewQuery = useQuery({
    queryKey: ["block-preview", block.key, debouncedHTML],
    queryFn: () => previewBlock(block.key, debouncedHTML),
    placeholderData: keepPreviousData,
    retry: false,
  });

  const saveMutation = useMutation({
    mutationFn: () => updateBlock(block, html),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: blocksQueryKey });
      if (result.updated_emails > 0) {
        await queryClient.invalidateQueries({ queryKey: ["emails"] });
      }
      const updated =
        result.updated_emails > 0
          ? ` ${result.updated_emails} ${result.updated_emails === 1 ? "email" : "emails"} updated` +
            (result.live_emails > 0 ? `, ${result.live_emails} live to re-upload.` : ".")
          : "";
      notifications.show({
        color: "green",
        message: `${result.block.name} saved, v${result.block.version}.${updated}`,
      });
      onClose();
    },
  });

  return (
    <Modal
      opened
      size="100%"
      title={
        <Group gap="xs">
          <Text fw={700}>{block.name}</Text>
          <Text c="dimmed" ff="monospace" size="xs">
            {block.key} · v{block.version}
          </Text>
        </Group>
      }
      onClose={onClose}
    >
      <Stack gap="sm">
        <SimpleGrid cols={{ base: 1, md: 2 }} spacing="md">
          <Stack gap="xs">
            <Textarea
              autosize
              classNames={{ input: styles.code }}
              label="HTML"
              maxRows={28}
              minRows={12}
              spellCheck={false}
              value={html}
              onChange={(event) => setHTML(event.currentTarget.value)}
            />
            <Text c="dimmed" size="xs">
              {block.key === frameKey
                ? "The frame’s preview shows every block in it. Keep <!-- blocks --> where they go."
                : "A block is a row of the frame’s table: start it with <tr>, mark the element comments attach to with data-review-block."}
            </Text>
          </Stack>
          <Stack gap="xs">
            <Text fw={500} size="sm">
              Preview{previewQuery.isFetching ? "…" : ""}
            </Text>
            {previewQuery.isError ? (
              <Alert color="red" p="xs">
                {previewQuery.error.message}
              </Alert>
            ) : null}
            <div className={styles.previewWrap}>
              {previewQuery.data ? (
                <iframe
                  className={styles.preview}
                  sandbox=""
                  srcDoc={previewQuery.data}
                  title={`${block.name} preview`}
                />
              ) : null}
            </div>
          </Stack>
        </SimpleGrid>
        {changed && block.emails > 0 ? (
          <Alert color="yellow" p="xs">
            Saving changes {block.emails} {block.emails === 1 ? "email" : "emails"}
            {block.live_emails > 0
              ? `, ${block.live_emails} of them live: they will need a re-upload to Bitrix24`
              : ""}
            . Their status stays as it is.
          </Alert>
        ) : null}
        {saveMutation.isError ? (
          <Alert color="red" p="xs">
            {saveMutation.error.message}
          </Alert>
        ) : null}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button
            disabled={!changed}
            loading={saveMutation.isPending}
            onClick={() => saveMutation.mutate()}
          >
            Save
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
