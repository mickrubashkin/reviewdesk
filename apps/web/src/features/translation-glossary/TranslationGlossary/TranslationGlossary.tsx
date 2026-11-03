import { useState } from "react";
import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Paper,
  Select,
  SimpleGrid,
  Stack,
  Table,
  Text,
  TextInput,
  Title,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { PlusIcon, TrashIcon } from "@phosphor-icons/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";

import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import type { UserRole } from "../../emails/types";
import {
  acceptGlossarySuggestion,
  glossaryLanguages,
  glossaryQueryKey,
  glossarySuggestionsQueryKey,
  rejectGlossarySuggestion,
  saveGlossary,
  useGlossary,
  useGlossarySuggestions,
  type GlossarySuggestion,
  type GlossarySuggestionInput,
  type GlossaryTerm,
} from "../api";
import { SuggestTermModal } from "../SuggestTermModal";
import styles from "../../auth-events/AuthEvents/AuthEvents.module.css";

const emptyTerm: GlossaryTerm = { term: "", translations: {}, note: "" };

const emptySuggestion: GlossarySuggestionInput = { term: "", language: "", translation: "", note: "" };

// Super admins edit the glossary and decide on suggestions; everyone else
// reads it and suggests terms.
export function TranslationGlossary({ currentUserRole }: { currentUserRole: UserRole }) {
  const glossaryQuery = useGlossary();
  const isSuperAdmin = currentUserRole === "super_admin";

  return (
    <div className={styles.page}>
      <AdminTableHeader
        subtitle="Terms the AI keeps in English or translates exactly as given when it suggests a translation."
        title="Translation glossary"
      />
      <Stack maw={860} p="md">
        {glossaryQuery.isLoading ? (
          <Loader />
        ) : glossaryQuery.isError || !glossaryQuery.data ? (
          <Alert color="red">Failed to load the glossary.</Alert>
        ) : (
          <>
            {isSuperAdmin ? <SuggestionQueue /> : null}
            {isSuperAdmin ? (
              <GlossaryForm key={glossaryQuery.dataUpdatedAt} terms={glossaryQuery.data} />
            ) : (
              <GlossaryReader terms={glossaryQuery.data} />
            )}
          </>
        )}
      </Stack>
    </div>
  );
}

function GlossaryForm({ terms }: { terms: GlossaryTerm[] }) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<GlossaryTerm[]>(terms);

  const saveMutation = useMutation({
    mutationFn: () => saveGlossary(draft),
    onSuccess: (saved) => {
      queryClient.setQueryData(glossaryQueryKey, saved);
      notifications.show({
        color: "green",
        message: "The next suggested translations follow it.",
        title: "Glossary saved",
      });
    },
    onError: (error) =>
      notifications.show({ color: "red", message: error.message, title: "Save failed" }),
  });

  const update = (index: number, patch: Partial<GlossaryTerm>) =>
    setDraft((current) => current.map((item, i) => (i === index ? { ...item, ...patch } : item)));

  return (
    <Stack gap="md">
      <Text c="dimmed" size="sm">
        Leave a language empty to keep the English term in it, as with product names like Partner
        Portal. Only the terms an email actually uses go to the AI.
      </Text>

      {draft.length === 0 ? (
        <Text c="dimmed" size="sm">
          No terms yet.
        </Text>
      ) : null}

      {draft.map((term, index) => (
        <Paper key={index} p="sm" withBorder>
          <Group align="flex-start" wrap="nowrap">
            <Stack flex={1} gap={6}>
              <Group grow>
                <TextInput
                  label="English term"
                  placeholder="Kickstart Bonus"
                  value={term.term}
                  onChange={(event) => update(index, { term: event.currentTarget.value })}
                />
                <TextInput
                  description="Optional hint for the AI"
                  label="Note"
                  value={term.note}
                  onChange={(event) => update(index, { note: event.currentTarget.value })}
                />
              </Group>
              <SimpleGrid cols={{ base: 2, sm: 4 }} spacing="xs">
                {glossaryLanguages.map((language) => (
                  <TextInput
                    key={language.code}
                    label={language.label}
                    placeholder="Keep in English"
                    value={term.translations[language.code] ?? ""}
                    onChange={(event) =>
                      update(index, {
                        translations: {
                          ...term.translations,
                          [language.code]: event.currentTarget.value,
                        },
                      })
                    }
                  />
                ))}
              </SimpleGrid>
            </Stack>
            <ActionIcon
              aria-label="Remove term"
              color="red"
              mt={28}
              variant="subtle"
              onClick={() => setDraft((current) => current.filter((_, i) => i !== index))}
            >
              <TrashIcon aria-hidden="true" size={16} />
            </ActionIcon>
          </Group>
        </Paper>
      ))}

      <Group justify="space-between">
        <Button
          leftSection={<PlusIcon aria-hidden="true" size={14} />}
          size="xs"
          variant="light"
          onClick={() => setDraft((current) => [...current, emptyTerm])}
        >
          Add term
        </Button>
        <Button loading={saveMutation.isPending} onClick={() => saveMutation.mutate()}>
          Save
        </Button>
      </Group>
    </Stack>
  );
}

function GlossaryReader({ terms }: { terms: GlossaryTerm[] }) {
  const [suggesting, setSuggesting] = useState<GlossarySuggestionInput | null>(null);

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Text c="dimmed" size="sm">
          Missing a term, or one is wrong? Suggest it: a CRM operator adds it or rejects it. You can
          also select a word in an email and pick the glossary icon.
        </Text>
        <Button
          leftSection={<PlusIcon aria-hidden="true" size={14} />}
          size="xs"
          variant="light"
          onClick={() => setSuggesting(emptySuggestion)}
        >
          Suggest a term
        </Button>
      </Group>
      <SuggestTermModal initial={suggesting} onClose={() => setSuggesting(null)} />
      <OwnSuggestions />
      {terms.length === 0 ? (
        <Text c="dimmed" size="sm">
          No terms yet.
        </Text>
      ) : (
        <Table striped withTableBorder>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>English term</Table.Th>
              {glossaryLanguages.map((language) => (
                <Table.Th key={language.code}>{language.label}</Table.Th>
              ))}
              <Table.Th>Note</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {terms.map((term) => (
              <Table.Tr key={term.term}>
                <Table.Td fw={600}>{term.term}</Table.Td>
                {glossaryLanguages.map((language) => (
                  <Table.Td key={language.code}>
                    {term.translations[language.code] ?? (
                      <Text c="dimmed" component="span" size="sm">
                        {term.term}
                      </Text>
                    )}
                  </Table.Td>
                ))}
                <Table.Td>{term.note}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </Stack>
  );
}

const statusColor = { pending: "gray", accepted: "teal", rejected: "red" } as const;

function OwnSuggestions() {
  const suggestionsQuery = useGlossarySuggestions();
  const suggestions = suggestionsQuery.data ?? [];
  if (suggestions.length === 0) {
    return null;
  }
  return (
    <Stack gap={4}>
      <Title order={5}>Your suggestions</Title>
      {suggestions.map((suggestion) => (
        <Group gap="xs" key={suggestion.id} wrap="nowrap">
          <Badge color={statusColor[suggestion.status]} size="sm" variant="light">
            {suggestion.status}
          </Badge>
          <Text size="sm">{describeSuggestion(suggestion)}</Text>
        </Group>
      ))}
    </Stack>
  );
}

function describeSuggestion(suggestion: GlossarySuggestion) {
  return suggestion.translation
    ? `${suggestion.term} → ${suggestion.translation} (${suggestion.language.toUpperCase()})`
    : `${suggestion.term} (keep in English)`;
}

function SuggestionQueue() {
  const suggestionsQuery = useGlossarySuggestions();
  const suggestions = suggestionsQuery.data ?? [];
  if (suggestions.length === 0) {
    return null;
  }
  return (
    <Stack gap="xs">
      <Title order={5}>Suggestions ({suggestions.length})</Title>
      <Text c="dimmed" size="sm">
        Accept adds the term to the glossary as edited here, or sets the translation of a term already in
        it. Unsaved changes below are lost when you accept.
      </Text>
      {suggestions.map((suggestion) => (
        <SuggestionCard key={suggestion.id} suggestion={suggestion} />
      ))}
    </Stack>
  );
}

function SuggestionCard({ suggestion }: { suggestion: GlossarySuggestion }) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<GlossarySuggestionInput>({
    term: suggestion.term,
    language: suggestion.language,
    translation: suggestion.translation,
    note: suggestion.note,
  });
  const update = (patch: Partial<GlossarySuggestionInput>) => setDraft((current) => ({ ...current, ...patch }));
  const onError = (error: Error) => notifications.show({ color: "red", message: error.message, title: "Not saved" });

  const acceptMutation = useMutation({
    mutationFn: () => acceptGlossarySuggestion(suggestion.id, draft),
    onSuccess: (terms) => {
      queryClient.setQueryData(glossaryQueryKey, terms);
      void queryClient.invalidateQueries({ queryKey: glossarySuggestionsQueryKey });
    },
    onError,
  });
  const rejectMutation = useMutation({
    mutationFn: () => rejectGlossarySuggestion(suggestion.id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: glossarySuggestionsQueryKey }),
    onError,
  });
  const busy = acceptMutation.isPending || rejectMutation.isPending;

  return (
    <Paper p="sm" withBorder>
      <Stack gap={6}>
        <Text c="dimmed" size="xs">
          {suggestion.suggested_by_email} · {new Date(suggestion.created_at).toLocaleDateString()}
          {suggestion.email_id ? (
            <>
              {" · "}
              <Link to={`/emails/${suggestion.email_id}/review`}>{suggestion.email_title ?? "email"}</Link>
            </>
          ) : null}
        </Text>
        <SimpleGrid cols={{ base: 1, sm: 4 }} spacing="xs">
          <TextInput
            label="English term"
            size="xs"
            value={draft.term}
            onChange={(event) => update({ term: event.currentTarget.value })}
          />
          <Select
            clearable
            data={glossaryLanguages.map((language) => ({ value: language.code, label: language.label }))}
            label="Language"
            placeholder="Keep in English"
            size="xs"
            value={draft.language || null}
            onChange={(value) => update(value ? { language: value } : { language: "", translation: "" })}
          />
          <TextInput
            disabled={!draft.language}
            label="Translation"
            size="xs"
            value={draft.translation}
            onChange={(event) => update({ translation: event.currentTarget.value })}
          />
          <TextInput
            label="Note"
            size="xs"
            value={draft.note}
            onChange={(event) => update({ note: event.currentTarget.value })}
          />
        </SimpleGrid>
        <Group gap="xs" justify="flex-end">
          <Button
            color="red"
            disabled={busy}
            loading={rejectMutation.isPending}
            size="xs"
            variant="subtle"
            onClick={() => rejectMutation.mutate()}
          >
            Reject
          </Button>
          <Button
            disabled={busy || !draft.term.trim()}
            loading={acceptMutation.isPending}
            size="xs"
            onClick={() => acceptMutation.mutate()}
          >
            Accept
          </Button>
        </Group>
      </Stack>
    </Paper>
  );
}
