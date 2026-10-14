import { useRef, useState } from "react";
import {
  ActionIcon,
  Alert,
  Button,
  Group,
  Loader,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
  Textarea,
  Title,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { PlusIcon, TrashIcon } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";

import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import { fetchBoards } from "../../boards/api";
import type { UserRole } from "../../emails/types";
import {
  fetchBoardAIConfig,
  resetBoardAIConfig,
  saveBoardAIConfig,
  type BoardAIConfig,
  type BoardAIConfigDraft,
} from "../api";
import {
  configToMarkdown,
  downloadTextFile,
  parseConfigJSON,
} from "./exportConfig";
import styles from "../../auth-events/AuthEvents/AuthEvents.module.css";

export function AIConfig({ currentUserRole }: { currentUserRole: UserRole }) {
  const [selectedBoardKey, setSelectedBoardKey] = useState<string | null>(null);
  const boardsQuery = useQuery({ queryKey: ["boards"], queryFn: fetchBoards });

  if (currentUserRole !== "super_admin") {
    return <Navigate replace to="/" />;
  }

  const boards = boardsQuery.data ?? [];
  const boardKey = selectedBoardKey ?? boards[0]?.key ?? null;

  return (
    <div className={styles.page}>
      <AdminTableHeader
        subtitle="Rules, roles and instructions the AI uses to review each board."
        title="AI review setup"
      />
      <Stack maw={860} p="md">
        <Select
          allowDeselect={false}
          data={boards.map((board) => ({ label: board.name, value: board.key }))}
          disabled={boardsQuery.isLoading}
          label="Board"
          value={boardKey}
          onChange={setSelectedBoardKey}
        />
        {boardKey ? <BoardConfigLoader boardKey={boardKey} /> : <Loader />}
      </Stack>
    </div>
  );
}

function BoardConfigLoader({ boardKey }: { boardKey: string }) {
  const configQuery = useQuery({
    queryKey: ["ai-config", boardKey],
    queryFn: () => fetchBoardAIConfig(boardKey),
  });

  if (configQuery.isLoading) {
    return <Loader />;
  }
  if (configQuery.isError || !configQuery.data) {
    return <Alert color="red">Failed to load the AI config.</Alert>;
  }

  return (
    <ConfigForm
      key={`${boardKey}:${configQuery.data.updated_at}:${configQuery.data.is_default}`}
      config={configQuery.data}
    />
  );
}

function ConfigForm({ config }: { config: BoardAIConfig }) {
  const queryClient = useQueryClient();
  const fileInput = useRef<HTMLInputElement>(null);
  const [draft, setDraft] = useState<BoardAIConfigDraft>({
    instruction: config.instruction,
    sequence_context: config.sequence_context,
    rules: config.rules,
    roles: config.roles,
  });
  const boardKey = config.board_key;

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["ai-config", boardKey] });
  const saveMutation = useMutation({
    mutationFn: () => saveBoardAIConfig(boardKey, draft),
    onSuccess: () => {
      void refresh();
      notifications.show({
        color: "green",
        message: "New analyses on this board use the updated setup.",
        title: "AI setup saved",
      });
    },
    onError: (error) =>
      notifications.show({ color: "red", message: error.message, title: "Save failed" }),
  });
  const resetMutation = useMutation({
    mutationFn: () => resetBoardAIConfig(boardKey),
    onSuccess: () => void refresh(),
    onError: (error) =>
      notifications.show({ color: "red", message: error.message, title: "Reset failed" }),
  });

  const update = (patch: Partial<BoardAIConfigDraft>) =>
    setDraft((current) => ({ ...current, ...patch }));

  const importFile = async (file: File) => {
    try {
      setDraft(parseConfigJSON(await file.text()));
      notifications.show({
        color: "blue",
        message: "Imported into the form. Save to apply.",
        title: "Config imported",
      });
    } catch {
      notifications.show({ color: "red", message: "Not a valid config JSON file.", title: "Import failed" });
    }
  };

  return (
    <Stack gap="lg">
      <Text c="dimmed" size="sm">
        {config.is_default
          ? "This board uses the built-in defaults. Saving creates a board-specific setup."
          : `Last saved by ${config.updated_by_email} on ${new Date(config.updated_at).toLocaleString()}.`}
      </Text>

      <Textarea
        autosize
        description="Who the AI is and how it should review. Output format and language rules are always applied."
        label="Instruction"
        minRows={4}
        value={draft.instruction}
        onChange={(event) => update({ instruction: event.currentTarget.value })}
      />

      <Stack gap="xs">
        <Title order={4}>Roles</Title>
        <Text c="dimmed" size="xs">
          Perspectives the AI reviews from, e.g. copywriter, compliance, partner manager.
        </Text>
        {draft.roles.map((role, index) => (
          <Group key={index} align="flex-start" wrap="nowrap">
            <Switch
              aria-label="Role enabled"
              checked={role.enabled}
              mt={8}
              onChange={(event) =>
                update({ roles: draft.roles.map((item, i) => i === index ? { ...item, enabled: event.currentTarget.checked } : item) })
              }
            />
            <Stack flex={1} gap={4}>
              <TextInput
                placeholder="Role name"
                value={role.name}
                onChange={(event) =>
                  update({ roles: draft.roles.map((item, i) => i === index ? { ...item, name: event.currentTarget.value } : item) })
                }
              />
              <Textarea
                autosize
                minRows={2}
                placeholder="What this role looks for"
                value={role.description}
                onChange={(event) =>
                  update({ roles: draft.roles.map((item, i) => i === index ? { ...item, description: event.currentTarget.value } : item) })
                }
              />
            </Stack>
            <ActionIcon
              aria-label="Remove role"
              color="red"
              mt={4}
              variant="subtle"
              onClick={() => update({ roles: draft.roles.filter((_, i) => i !== index) })}
            >
              <TrashIcon aria-hidden="true" size={16} />
            </ActionIcon>
          </Group>
        ))}
        <Button
          leftSection={<PlusIcon aria-hidden="true" size={14} />}
          size="xs"
          variant="light"
          w="fit-content"
          onClick={() => update({ roles: [...draft.roles, { name: "", description: "", enabled: true }] })}
        >
          Add role
        </Button>
      </Stack>

      <Stack gap="xs">
        <Title order={4}>Rules</Title>
        {draft.rules.map((rule, index) => (
          <Group key={index} align="flex-start" wrap="nowrap">
            <Switch
              aria-label="Rule enabled"
              checked={rule.enabled}
              mt={8}
              onChange={(event) =>
                update({ rules: draft.rules.map((item, i) => i === index ? { ...item, enabled: event.currentTarget.checked } : item) })
              }
            />
            <Stack flex={1} gap={4}>
              <TextInput
                placeholder="Rule title"
                value={rule.title}
                onChange={(event) =>
                  update({ rules: draft.rules.map((item, i) => i === index ? { ...item, title: event.currentTarget.value } : item) })
                }
              />
              <Textarea
                autosize
                minRows={2}
                placeholder="What to check"
                value={rule.body}
                onChange={(event) =>
                  update({ rules: draft.rules.map((item, i) => i === index ? { ...item, body: event.currentTarget.value } : item) })
                }
              />
            </Stack>
            <ActionIcon
              aria-label="Remove rule"
              color="red"
              mt={4}
              variant="subtle"
              onClick={() => update({ rules: draft.rules.filter((_, i) => i !== index) })}
            >
              <TrashIcon aria-hidden="true" size={16} />
            </ActionIcon>
          </Group>
        ))}
        <Button
          leftSection={<PlusIcon aria-hidden="true" size={14} />}
          size="xs"
          variant="light"
          w="fit-content"
          onClick={() => update({ rules: [...draft.rules, { title: "", body: "", enabled: true }] })}
        >
          Add rule
        </Button>
      </Stack>

      <Textarea
        autosize
        description="Background knowledge about this board's sequence: goals, stages, expected CTAs, timing."
        label="Sequence context"
        minRows={6}
        value={draft.sequence_context}
        onChange={(event) => update({ sequence_context: event.currentTarget.value })}
      />

      <Group>
        <Button loading={saveMutation.isPending} onClick={() => saveMutation.mutate()}>
          Save
        </Button>
        <Button
          color="red"
          disabled={config.is_default}
          loading={resetMutation.isPending}
          variant="light"
          onClick={() => {
            if (window.confirm("Reset this board to the built-in defaults? Your custom setup will be deleted.")) {
              resetMutation.mutate();
            }
          }}
        >
          Reset to defaults
        </Button>
        <Button
          variant="default"
          onClick={() => downloadTextFile(JSON.stringify(draft, null, 2), `${boardKey}-ai-config.json`, "application/json")}
        >
          Export JSON
        </Button>
        <Button
          variant="default"
          onClick={() => downloadTextFile(configToMarkdown(boardKey, draft), `${boardKey}-ai-config.md`, "text/markdown")}
        >
          Export Markdown
        </Button>
        <Button variant="default" onClick={() => fileInput.current?.click()}>
          Import JSON
        </Button>
        <input
          ref={fileInput}
          accept="application/json,.json"
          hidden
          type="file"
          onChange={(event) => {
            const file = event.currentTarget.files?.[0];
            event.currentTarget.value = "";
            if (file) void importFile(file);
          }}
        />
      </Group>
    </Stack>
  );
}
