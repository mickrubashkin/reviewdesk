import { useState } from "react";
import { Button, Group, Modal, Select, Stack, Text, TextInput } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import {
  glossaryLanguages,
  glossarySuggestionsQueryKey,
  suggestGlossaryTerm,
  type GlossarySuggestionInput,
} from "./api";

// SuggestTermModal sends a term to the glossary queue; a super admin decides.
export function SuggestTermModal({
  initial,
  onClose,
}: {
  initial: GlossarySuggestionInput | null;
  onClose: () => void;
}) {
  return (
    <Modal opened={initial !== null} title="Suggest for the glossary" onClose={onClose}>
      {initial ? <SuggestTermForm initial={initial} onDone={onClose} /> : null}
    </Modal>
  );
}

function SuggestTermForm({ initial, onDone }: { initial: GlossarySuggestionInput; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(initial);
  const update = (patch: Partial<GlossarySuggestionInput>) => setDraft((current) => ({ ...current, ...patch }));

  const mutation = useMutation({
    mutationFn: () => suggestGlossaryTerm(draft),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: glossarySuggestionsQueryKey });
      notifications.show({
        color: "green",
        message: "A CRM operator will add it to the glossary or reject it.",
        title: "Suggestion sent",
      });
      onDone();
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not sent" }),
  });

  return (
    <Stack gap="sm">
      <Text c="dimmed" size="sm">
        The AI keeps glossary terms in English or translates them exactly as given.
      </Text>
      <TextInput
        data-autofocus
        label="English term"
        placeholder="Deals"
        required
        value={draft.term}
        onChange={(event) => update({ term: event.currentTarget.value })}
      />
      <Group grow align="flex-start">
        <Select
          clearable
          data={glossaryLanguages.map((language) => ({ value: language.code, label: language.label }))}
          label="Language"
          placeholder="Any"
          value={draft.language || null}
          onChange={(value) => update(value ? { language: value } : { language: "", translation: "" })}
        />
        <TextInput
          description={draft.language ? undefined : "Empty: keep the English term"}
          disabled={!draft.language}
          label="Translation"
          value={draft.translation}
          onChange={(event) => update({ translation: event.currentTarget.value })}
        />
      </Group>
      <TextInput
        description="Why, or a hint for the AI"
        label="Note"
        value={draft.note}
        onChange={(event) => update({ note: event.currentTarget.value })}
      />
      <Group justify="flex-end">
        <Button variant="subtle" onClick={onDone}>
          Cancel
        </Button>
        <Button disabled={!draft.term.trim()} loading={mutation.isPending} onClick={() => mutation.mutate()}>
          Send
        </Button>
      </Group>
    </Stack>
  );
}
