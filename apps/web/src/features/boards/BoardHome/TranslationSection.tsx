import { Alert, Badge, Button, Group, Select, Stack, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { diffWords } from "diff";

import { confirmTranslation, fetchTranslationStatus, setTranslationLink } from "../../emails/api";
import type { EmailListItem } from "../../emails/types";

// TranslationSection tells whether a translation is behind its EN master and
// lists what changed in EN since, so the translator updates just that.
export function TranslationSection({ email, canManage }: { email: EmailListItem; canManage: boolean }) {
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
        {status.changes.map((change) => (
          <Stack key={change.key} gap={2}>
            <Text fw={600} size="xs">
              {change.key}
            </Text>
            <Text size="xs" style={{ whiteSpace: "pre-wrap" }}>
              {diffWords(change.before, change.after).map((part, index) => (
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
          </Stack>
        ))}
        {canManage ? (
          <Group>
            <Button loading={confirmMutation.isPending} size="compact-xs" onClick={() => confirmMutation.mutate()}>
              Mark up to date
            </Button>
            <Text c="dimmed" size="xs">
              after updating this translation
            </Text>
          </Group>
        ) : null}
      </Stack>
    </Alert>
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
