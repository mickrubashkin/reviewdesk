import { ActionIcon, Badge, Group, Paper, Select, Stack, Text, Tooltip } from "@mantine/core";
import { ArrowDownIcon, ArrowUpIcon, TrashIcon } from "@phosphor-icons/react";
import { useState } from "react";

import type { LayoutItem } from "../emails/types";
import type { BlockCatalogItem } from "./api";
import { layoutItemId, moveBlock, withBlock } from "./layout";

// LayoutEditor lists the blocks of an email in order; when editable it adds
// blocks from the library, moves and removes them. The caller keeps the
// layout and decides when to save it.
export function LayoutEditor({
  catalog,
  disabled = false,
  editable,
  layout,
  removeHint,
  onChange,
}: {
  catalog: BlockCatalogItem[];
  disabled?: boolean;
  editable: boolean;
  layout: LayoutItem[];
  // What removing a block does to its texts, as a tooltip.
  removeHint?: string;
  onChange: (layout: LayoutItem[]) => void;
}) {
  const [added, setAdded] = useState<string | null>(null);
  const blockName = (key: string) => catalog.find((block) => block.key === key)?.name ?? key;

  return (
    <Stack gap={6}>
      <Stack gap={4}>
        {layout.map((item, index) => {
          const id = layoutItemId(item);
          const removeButton = (
            <ActionIcon
              aria-label={`Remove ${id}`}
              color="red"
              disabled={disabled || layout.length === 1}
              size="sm"
              variant="subtle"
              onClick={() => onChange(layout.filter((_, position) => position !== index))}
            >
              <TrashIcon aria-hidden="true" size={14} />
            </ActionIcon>
          );
          return (
            <Paper key={id} px="xs" py={4} withBorder>
              <Group gap="xs" justify="space-between" wrap="nowrap">
                <Group gap={6} wrap="nowrap">
                  <Text size="sm">{blockName(item.block)}</Text>
                  {item.suffix ? (
                    <Badge color="gray" size="xs" tt="none" variant="light">
                      {id}
                    </Badge>
                  ) : null}
                </Group>
                {editable ? (
                  <Group gap={2} wrap="nowrap">
                    <ActionIcon
                      aria-label={`Move ${id} up`}
                      disabled={disabled || index === 0}
                      size="sm"
                      variant="subtle"
                      onClick={() => onChange(moveBlock(layout, index, -1))}
                    >
                      <ArrowUpIcon aria-hidden="true" size={14} />
                    </ActionIcon>
                    <ActionIcon
                      aria-label={`Move ${id} down`}
                      disabled={disabled || index === layout.length - 1}
                      size="sm"
                      variant="subtle"
                      onClick={() => onChange(moveBlock(layout, index, 1))}
                    >
                      <ArrowDownIcon aria-hidden="true" size={14} />
                    </ActionIcon>
                    {removeHint ? <Tooltip label={removeHint}>{removeButton}</Tooltip> : removeButton}
                  </Group>
                ) : null}
              </Group>
            </Paper>
          );
        })}
      </Stack>
      {editable ? (
        <Select
          aria-label="Add a block"
          data={catalog.map((block) => ({ label: block.name, value: block.key }))}
          disabled={disabled}
          placeholder="Add a block…"
          size="xs"
          value={added}
          renderOption={({ option }) => (
            <Stack gap={0}>
              <Text size="sm">{option.label}</Text>
              <Text c="dimmed" size="xs">
                {catalog.find((block) => block.key === option.value)?.description}
              </Text>
            </Stack>
          )}
          onChange={(value) => {
            if (value) {
              onChange(withBlock(layout, value));
            }
            setAdded(null);
          }}
        />
      ) : null}
    </Stack>
  );
}
