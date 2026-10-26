import { useState } from "react";
import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Paper,
  Radio,
  Stack,
  TagsInput,
  Text,
  TextInput,
  Tooltip,
} from "@mantine/core";
import { ArrowDownIcon, ArrowUpIcon, PlusIcon, TrashIcon } from "@phosphor-icons/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { ApiError } from "../../../shared/api";
import { boardRegionsQueryKey, saveBoardRegions, useBoardRegions } from "../api";
import type { Board, BoardRegion } from "../../emails/types";

// Widths shared by the header and the rows, so the columns line up.
const COLUMN = { language: 70, reference: 76, actions: 80 };

type Draft = { key?: string; name: string; language: string; managers: string[]; reference: boolean; emails: number };

const toDraft = (region: BoardRegion): Draft => ({
  key: region.key,
  name: region.name,
  language: region.language,
  managers: region.managers,
  reference: region.reference,
  emails: region.emails,
});

// BoardRegionsEditor edits the regions of a board: the reference chain and
// the markets that adapt it, each with its language and managers (one robot
// each in Bitrix24). The list is saved as a whole.
export function BoardRegionsEditor({ board }: { board: Board }) {
  const queryClient = useQueryClient();
  const regionsQuery = useBoardRegions(board.key);
  const [draft, setDraft] = useState<Draft[] | null>(null);
  const regions = draft ?? (regionsQuery.data ?? []).map(toDraft);

  const saveMutation = useMutation({
    mutationFn: () => saveBoardRegions(board.key, regions),
    onSuccess: (saved) => {
      queryClient.setQueryData(boardRegionsQueryKey(board.key), saved);
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      setDraft(null);
    },
  });

  const change = (index: number, patch: Partial<Draft>) =>
    setDraft(regions.map((region, i) => (i === index ? { ...region, ...patch } : region)));
  const move = (index: number, offset: -1 | 1) => {
    const next = [...regions];
    [next[index], next[index + offset]] = [next[index + offset], next[index]];
    setDraft(next);
  };
  const removedEmails = (regionsQuery.data ?? [])
    .filter((saved) => !regions.some((region) => region.key === saved.key))
    .reduce((sum, region) => sum + region.emails, 0);

  if (regionsQuery.isLoading) {
    return <Loader size="sm" />;
  }

  return (
    <Stack gap="sm">
      <Text c="dimmed" size="sm">
        A region is a version of the chain: the reference, or a market whose managers adapt it. Each manager has a
        robot in Bitrix24. Every email of the board belongs to a region.
      </Text>
      {regionsQuery.isError ? <Alert color="red">The regions could not be loaded.</Alert> : null}
      {regions.length > 0 ? (
        <Group gap="xs" px="xs" wrap="nowrap">
          <Text fw={500} size="sm" style={{ flex: 2 }}>
            Region
          </Text>
          <Text fw={500} size="sm" w={COLUMN.language}>
            Language
          </Text>
          <Text fw={500} size="sm" style={{ flex: 3 }}>
            Managers
          </Text>
          <Tooltip label="The reference chain the other regions adapt">
            <Text fw={500} size="sm" ta="center" w={COLUMN.reference}>
              Reference
            </Text>
          </Tooltip>
          <span style={{ width: COLUMN.actions }} />
        </Group>
      ) : null}
      <Radio.Group
        value={String(regions.findIndex((region) => region.reference))}
        onChange={(value) => setDraft(regions.map((region, i) => ({ ...region, reference: String(i) === value })))}
      >
        <Stack gap="xs">
          {regions.map((region, index) => (
            <Paper key={region.key ?? `new-${index}`} p="xs" withBorder>
              <Group gap="xs" wrap="nowrap">
                <TextInput
                  placeholder="EU + SSA"
                  style={{ flex: 2 }}
                  value={region.name}
                  onChange={(event) => change(index, { name: event.currentTarget.value })}
                />
                <TextInput
                  placeholder="en"
                  w={COLUMN.language}
                  value={region.language}
                  onChange={(event) => change(index, { language: event.currentTarget.value })}
                />
                <TagsInput
                  placeholder="Add a manager"
                  style={{ flex: 3 }}
                  value={region.managers}
                  onChange={(managers) => change(index, { managers })}
                />
                <Group justify="center" w={COLUMN.reference}>
                  <Radio aria-label={`${region.name || "Region"} is the reference`} value={String(index)} />
                </Group>
                <Group gap={2} w={COLUMN.actions} wrap="nowrap">
                  <ActionIcon aria-label="Move up" disabled={index === 0} size="sm" variant="subtle" onClick={() => move(index, -1)}>
                    <ArrowUpIcon aria-hidden="true" size={14} />
                  </ActionIcon>
                  <ActionIcon
                    aria-label="Move down"
                    disabled={index === regions.length - 1}
                    size="sm"
                    variant="subtle"
                    onClick={() => move(index, 1)}
                  >
                    <ArrowDownIcon aria-hidden="true" size={14} />
                  </ActionIcon>
                  <ActionIcon
                    aria-label={`Remove ${region.name}`}
                    color="red"
                    size="sm"
                    variant="subtle"
                    onClick={() => setDraft(regions.filter((_, i) => i !== index))}
                  >
                    <TrashIcon aria-hidden="true" size={14} />
                  </ActionIcon>
                </Group>
              </Group>
              {region.emails > 0 ? (
                <Badge color="gray" mt={6} size="xs" variant="light">
                  {region.emails} {region.emails === 1 ? "email" : "emails"}
                </Badge>
              ) : null}
            </Paper>
          ))}
        </Stack>
      </Radio.Group>
      <Group justify="space-between">
        <Button
          leftSection={<PlusIcon aria-hidden="true" size={14} />}
          size="xs"
          variant="light"
          onClick={() => setDraft([...regions, { name: "", language: "", managers: [], reference: false, emails: 0 }])}
        >
          Add a region
        </Button>
        {draft ? (
          <Group gap="xs">
            <Button disabled={saveMutation.isPending} size="xs" variant="subtle" onClick={() => setDraft(null)}>
              Cancel
            </Button>
            <Button loading={saveMutation.isPending} size="xs" onClick={() => saveMutation.mutate()}>
              Save regions
            </Button>
          </Group>
        ) : null}
      </Group>
      {removedEmails > 0 ? (
        <Alert color="orange">
          {removedEmails} {removedEmails === 1 ? "email" : "emails"} of the removed regions will have no region.
        </Alert>
      ) : null}
      {saveMutation.isError ? (
        <Alert color="red">
          {saveMutation.error instanceof ApiError ? saveMutation.error.message : "The regions could not be saved."}
        </Alert>
      ) : null}
    </Stack>
  );
}
