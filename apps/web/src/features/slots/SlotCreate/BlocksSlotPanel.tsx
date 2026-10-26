import { Alert, Button, Chip, Group, Loader, Stack, Tabs, Text, TextInput } from "@mantine/core";
import { useDebouncedValue } from "@mantine/hooks";
import { useQuery } from "@tanstack/react-query";

import { previewLayout, type BlockCatalogItem } from "../../blocks/api";
import { LayoutEditor } from "../../blocks/LayoutEditor";
import { preheaderMaxLength, subjectMaxLength } from "../../blocks/layout";
import type { LayoutItem } from "../../emails/types";
import { slotLanguages, type SlotLanguage } from "./slotLanguages";
import styles from "./SlotCreate.module.css";

export type BlocksDraft = { subject: string; preheader: string };

// BlocksSlotPanel builds a new slot from the block library: the blocks, the
// languages and their subject and preheader, with a preview per language.
export function BlocksSlotPanel({
  board,
  catalog,
  drafts,
  isDefault,
  languages,
  layout,
  layoutError,
  onDraftChange,
  onLanguagesChange,
  onLayoutChange,
  onLayoutReset,
}: {
  board: string;
  catalog: BlockCatalogItem[];
  drafts: Record<SlotLanguage, BlocksDraft>;
  // Whether the layout is still the stage's usual one.
  isDefault: boolean;
  languages: SlotLanguage[];
  layout: LayoutItem[] | undefined;
  layoutError: boolean;
  onDraftChange: (language: SlotLanguage, patch: Partial<BlocksDraft>) => void;
  onLanguagesChange: (languages: SlotLanguage[]) => void;
  onLayoutChange: (layout: LayoutItem[]) => void;
  onLayoutReset: () => void;
}) {
  if (layoutError) {
    return <Alert color="red">The usual blocks of this stage could not be loaded.</Alert>;
  }
  if (!layout) {
    return <Loader size="sm" />;
  }

  return (
    <Stack>
      <Stack gap={6}>
        <Group justify="space-between">
          <Text fw={700} size="sm">
            Blocks
          </Text>
          {!isDefault ? (
            <Button size="compact-xs" variant="subtle" onClick={onLayoutReset}>
              Back to the usual blocks
            </Button>
          ) : null}
        </Group>
        <Text c="dimmed" size="xs">
          {isDefault
            ? "The usual blocks of this stage. Change them here or later in Details → Blocks of the EN email."
            : "Changed from the usual blocks of this stage."}
        </Text>
        <LayoutEditor catalog={catalog} editable layout={layout} onChange={onLayoutChange} />
      </Stack>

      <Stack gap={6}>
        <Text fw={700} size="sm">
          Languages
        </Text>
        <Chip.Group
          multiple
          value={languages}
          onChange={(value) =>
            onLanguagesChange(slotLanguages.filter((language) => language === "en" || value.includes(language)))
          }
        >
          <Group gap="xs">
            {slotLanguages.map((language) => (
              <Chip disabled={language === "en"} key={language} size="xs" value={language}>
                {language.toUpperCase()}
              </Chip>
            ))}
          </Group>
        </Chip.Group>
        <Text c="dimmed" size="xs">
          Every language gets the same blocks and the shared texts of its language (footer, signature). The
          texts of the email start as written in the blocks: write them in EN after creating, then translate.
        </Text>
      </Stack>

      <Tabs defaultValue="en" keepMounted={false}>
        <Tabs.List>
          {languages.map((language) => (
            <Tabs.Tab key={language} value={language}>
              {language.toUpperCase()}
            </Tabs.Tab>
          ))}
        </Tabs.List>
        {languages.map((language) => (
          <Tabs.Panel key={language} pt="md" value={language}>
            <LanguageBlocksPanel
              board={board}
              draft={drafts[language]}
              language={language}
              layout={layout}
              onChange={(patch) => onDraftChange(language, patch)}
            />
          </Tabs.Panel>
        ))}
      </Tabs>
    </Stack>
  );
}

function LanguageBlocksPanel({
  board,
  draft,
  language,
  layout,
  onChange,
}: {
  board: string;
  draft: BlocksDraft;
  language: SlotLanguage;
  layout: LayoutItem[];
  onChange: (patch: Partial<BlocksDraft>) => void;
}) {
  const [debounced] = useDebouncedValue({ layout, ...draft }, 400);
  const previewQuery = useQuery({
    queryKey: ["layout-preview", board, language, debounced],
    queryFn: () =>
      previewLayout({
        sequence: board,
        language,
        layout: debounced.layout,
        subject: debounced.subject.trim() || undefined,
        preheader: debounced.preheader.trim() || undefined,
      }),
    retry: false,
    placeholderData: (previous) => previous,
  });

  return (
    <Stack>
      <Group grow align="flex-start">
        <TextInput
          description={<LengthCounter limit={subjectMaxLength} value={draft.subject} />}
          inputWrapperOrder={["label", "input", "description"]}
          label="Subject"
          value={draft.subject}
          onChange={(event) => onChange({ subject: event.currentTarget.value })}
        />
        <TextInput
          description={<LengthCounter limit={preheaderMaxLength} value={draft.preheader} />}
          inputWrapperOrder={["label", "input", "description"]}
          label="Preheader"
          value={draft.preheader}
          onChange={(event) => onChange({ preheader: event.currentTarget.value })}
        />
      </Group>
      {previewQuery.isError ? (
        <Alert color="red">
          {previewQuery.error instanceof Error ? previewQuery.error.message : "The preview failed."}
        </Alert>
      ) : null}
      {previewQuery.data ? (
        <iframe
          className={styles.previewFrame}
          sandbox=""
          srcDoc={previewQuery.data}
          title={`${language.toUpperCase()} preview`}
        />
      ) : null}
    </Stack>
  );
}

function LengthCounter({ limit, value }: { limit: number; value: string }) {
  return (
    <Text c={value.length > limit ? "red" : "dimmed"} component="span" size="xs">
      {value.length} / {limit}
    </Text>
  );
}
