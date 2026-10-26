import { Button, Group, Stack, Text } from "@mantine/core";
import { useState } from "react";

import type { BlockCatalogItem } from "../../../blocks/api";
import { LayoutEditor } from "../../../blocks/LayoutEditor";
import { sameLayout } from "../../../blocks/layout";
import type { LayoutItem } from "../../../emails/types";

// BlocksPanel shows the blocks an email is built from; a manager of the
// board changes them on the EN email and the translations follow.
export function BlocksPanel({
  canEdit,
  catalog,
  isSaving,
  isTranslation,
  layout,
  onSave,
}: {
  canEdit: boolean;
  catalog: BlockCatalogItem[];
  isSaving: boolean;
  isTranslation: boolean;
  layout: LayoutItem[];
  onSave: (layout: LayoutItem[]) => void;
}) {
  const [draft, setDraft] = useState(layout);
  const changed = !sameLayout(draft, layout);

  return (
    <Stack gap={6}>
      <Text fw={700} size="sm">
        Blocks
      </Text>
      <Text c="dimmed" size="xs">
        {isTranslation
          ? "Built from the block library; the blocks come from the EN email."
          : canEdit
            ? "Built from the block library: a change to a block changes this email too. Translations get the same blocks; a new block comes to them with the EN text to translate."
            : "Built from the block library: a change to a block changes this email too."}
      </Text>
      <LayoutEditor
        catalog={catalog}
        disabled={isSaving}
        editable={canEdit}
        layout={draft}
        removeHint="Its texts go too; adding it back before saving keeps them"
        onChange={setDraft}
      />
      {canEdit && changed ? (
        <Group gap="xs" justify="flex-end">
          <Button disabled={isSaving} size="xs" variant="subtle" onClick={() => setDraft(layout)}>
            Cancel
          </Button>
          <Button loading={isSaving} size="xs" onClick={() => onSave(draft)}>
            Save blocks
          </Button>
        </Group>
      ) : null}
    </Stack>
  );
}
