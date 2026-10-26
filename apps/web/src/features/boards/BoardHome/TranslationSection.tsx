import { Alert, Badge, Button, Group, Select, Stack, Text, Textarea } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { SparkleIcon } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { diffWords } from "diff";
import { useState } from "react";

import {
  confirmTranslation,
  fetchEmailDetail,
  fetchTranslationStatus,
  setTranslationLink,
  suggestTranslation,
  updateEmailEditableFields,
} from "../../emails/api";
import type { EmailListItem, TranslationSuggestions } from "../../emails/types";

// TranslationSection tells whether a translation is behind its EN master and
// lists what changed in EN since, so the translator updates just that. They
// can ask AI for updated text of the changed fields, edit it and apply it.
export function TranslationSection({ email, canEdit }: { email: EmailListItem; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const statusQuery = useQuery({
    queryKey: ["emails", email.id, "translation"],
    queryFn: () => fetchTranslationStatus(email.id),
    enabled: Boolean(email.translation_of),
  });
  const confirmMutation = useMutation({
    mutationFn: () => confirmTranslation(email.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails", email.id, "translation"] });
      void queryClient.invalidateQueries({ queryKey: ["emails", email.sequence] });
      notifications.show({ color: "green", message: "Marked up to date with EN.", title: "Translation" });
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });

  const [suggestions, setSuggestions] = useState<TranslationSuggestions | null>(null);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const suggestMutation = useMutation({
    mutationFn: () => suggestTranslation(email.id),
    onSuccess: (result) => {
      setSuggestions(result);
      setDrafts(Object.fromEntries(result.fields.map((field) => [field.key, field.suggestion])));
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "No suggestion" }),
  });
  const applyMutation = useMutation({
    mutationFn: async () => {
      const detail = await fetchEmailDetail(email.id);
      const fields = { ...detail.editable_fields };
      for (const [key, value] of Object.entries(drafts)) {
        if (key !== "subject" && key !== "preheader" && fields[key]) {
          fields[key] = { ...fields[key], value };
        }
      }
      await updateEmailEditableFields(email.id, {
        title: detail.title,
        subject: drafts.subject ?? detail.subject ?? "",
        preheader: drafts.preheader ?? detail.preheader ?? "",
        editable_fields: fields,
      });
      return confirmTranslation(email.id);
    },
    onSuccess: () => {
      setSuggestions(null);
      setDrafts({});
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({ color: "green", message: "Saved and marked up to date with EN.", title: "Translation" });
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });

  const status = statusQuery.data;
  if (!email.translation_of || !status) {
    return null;
  }
  if (!status.stale) {
    return (
      <Group gap={6}>
        <Badge color="green" size="xs" variant="light">
          Up to date with EN
        </Badge>
        <Text c="dimmed" size="xs">
          {status.master_title} · EN v{status.master_latest_version}
        </Text>
      </Group>
    );
  }

  return (
    <Alert color="orange" title={`EN changed since this translation (${status.changes.length})`} variant="light">
      <Stack gap="xs">
        {status.block_changes?.length ? (
          <Text size="xs">
            {status.block_changes
              .map((change) => `${change.action === "added" ? "Added" : "Removed"} block ${change.name ?? change.block}`)
              .join(" · ")}
            {status.block_changes.some((change) => change.action === "added")
              ? " (with the EN text, translate it below)"
              : ""}
          </Text>
        ) : null}
        {status.changes.map((change) => (
          <Stack key={change.key} gap={2}>
            <Text fw={600} size="xs">
              {change.key}
            </Text>
            <WordDiff after={change.after} before={change.before} />
          </Stack>
        ))}
        {canEdit && !suggestions ? (
          <Group>
            <Button
              leftSection={<SparkleIcon aria-hidden="true" size={14} />}
              loading={suggestMutation.isPending}
              size="compact-xs"
              variant="light"
              onClick={() => suggestMutation.mutate()}
            >
              Suggest translation
            </Button>
            <Button loading={confirmMutation.isPending} size="compact-xs" variant="default" onClick={() => confirmMutation.mutate()}>
              Mark up to date
            </Button>
          </Group>
        ) : null}
        {canEdit && suggestions ? (
          <SuggestionEditor
            drafts={drafts}
            isApplying={applyMutation.isPending}
            language={email.language}
            suggestions={suggestions}
            onApply={() => applyMutation.mutate()}
            onChange={(key, value) => setDrafts((current) => ({ ...current, [key]: value }))}
            onDiscard={() => {
              setSuggestions(null);
              setDrafts({});
            }}
          />
        ) : null}
      </Stack>
    </Alert>
  );
}

// SuggestionEditor shows the AI text for each changed field next to the
// current translation; the translator edits it and applies it in one go.
function SuggestionEditor({
  drafts,
  isApplying,
  language,
  suggestions,
  onApply,
  onChange,
  onDiscard,
}: {
  drafts: Record<string, string>;
  isApplying: boolean;
  language: string;
  suggestions: TranslationSuggestions;
  onApply: () => void;
  onChange: (key: string, value: string) => void;
  onDiscard: () => void;
}) {
  // A field with a problem must be edited before applying.
  const blocked = suggestions.fields.some((field) => field.problem && drafts[field.key] === field.suggestion);
  return (
    <Stack gap="xs">
      {suggestions.fields.map((field) => (
        <Stack key={field.key} gap={2}>
          <Text fw={600} size="xs">
            {field.key} ({language.toUpperCase()})
          </Text>
          {field.current ? (
            <Text c="dimmed" lineClamp={3} size="xs" style={{ whiteSpace: "pre-wrap" }} title={field.current}>
              Now: {field.current}
            </Text>
          ) : null}
          <Textarea
            aria-label={`New ${field.key}`}
            autosize
            error={field.problem && drafts[field.key] === field.suggestion ? field.problem : undefined}
            minRows={1}
            size="xs"
            value={drafts[field.key] ?? ""}
            onChange={(event) => onChange(field.key, event.currentTarget.value)}
          />
        </Stack>
      ))}
      {suggestions.skipped.length > 0 ? (
        <Text c="dimmed" size="xs">
          Update by hand (the translation has no field with this key): {suggestions.skipped.join(", ")}
        </Text>
      ) : null}
      {suggestions.glossary.length > 0 ? (
        <Text c="dimmed" size="xs">
          Glossary: {suggestions.glossary.join(", ")}
        </Text>
      ) : null}
      <Group>
        <Button disabled={blocked || suggestions.fields.length === 0} loading={isApplying} size="compact-xs" onClick={onApply}>
          Apply and mark up to date
        </Button>
        <Button disabled={isApplying} size="compact-xs" variant="default" onClick={onDiscard}>
          Discard
        </Button>
        <Text c="dimmed" size="xs">
          AI ({suggestions.model}) — check before applying
        </Text>
      </Group>
    </Stack>
  );
}

// WordDiff shows a field's text with the words added and removed marked.
export function WordDiff({ before, after }: { before: string; after: string }) {
  return (
    <Text size="xs" style={{ whiteSpace: "pre-wrap" }}>
      {diffWords(before, after).map((part, index) => (
        <span
          key={index}
          style={
            part.added
              ? { background: "var(--mantine-color-green-1)" }
              : part.removed
                ? { background: "var(--mantine-color-red-1)", textDecoration: "line-through" }
                : undefined
          }
        >
          {part.added || part.removed ? part.value : shortenUnchanged(part.value)}
        </span>
      ))}
    </Text>
  );
}

// shortenUnchanged keeps a little context around changes in long fields.
function shortenUnchanged(text: string) {
  if (text.length <= 120) return text;
  return `${text.slice(0, 50)} … ${text.slice(-50)}`;
}

const notTranslation = "__none__";

// TranslationLinkSelect sets by hand which EN email of the slot this email
// translates, for translations the automatic matching could not pair.
export function TranslationLinkSelect({ email, siblings }: { email: EmailListItem; siblings: EmailListItem[] }) {
  const queryClient = useQueryClient();
  const masters = siblings.filter((s) => s.language === "en");
  const mutation = useMutation({
    mutationFn: (masterId: string | null) => setTranslationLink(email.id, masterId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });
  if (masters.length === 0) return null;

  return (
    <Select
      allowDeselect={false}
      data={[
        ...masters.map((m) => ({
          label: `EN · ${m.variant}${m.adaptation_key !== "default" ? ` · ${m.adaptation_label}` : ""} — ${m.title}`,
          value: m.id,
        })),
        { label: "Not a translation of any EN email here", value: notTranslation },
      ]}
      disabled={mutation.isPending}
      label="Translation of"
      size="xs"
      value={email.translation_of ?? notTranslation}
      onChange={(value) => mutation.mutate(value === notTranslation || !value ? null : value)}
    />
  );
}
