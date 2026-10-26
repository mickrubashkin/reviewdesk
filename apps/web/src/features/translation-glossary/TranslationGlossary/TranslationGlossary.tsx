import { useState } from "react";
import {
  ActionIcon,
  Alert,
  Button,
  Group,
  Loader,
  Paper,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { PlusIcon, TrashIcon } from "@phosphor-icons/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";

import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import type { UserRole } from "../../emails/types";
import {
  glossaryLanguages,
  glossaryQueryKey,
  saveGlossary,
  useGlossary,
  type GlossaryTerm,
} from "../api";
import styles from "../../auth-events/AuthEvents/AuthEvents.module.css";

const emptyTerm: GlossaryTerm = { term: "", translations: {}, note: "" };

export function TranslationGlossary({ currentUserRole }: { currentUserRole: UserRole }) {
  const glossaryQuery = useGlossary();

  if (currentUserRole !== "super_admin") {
    return <Navigate replace to="/" />;
  }

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
          <GlossaryForm key={glossaryQuery.dataUpdatedAt} terms={glossaryQuery.data} />
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
