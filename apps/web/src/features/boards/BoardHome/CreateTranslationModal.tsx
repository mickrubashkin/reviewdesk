import { useState } from "react";
import { Button, Checkbox, Group, Modal, Stack, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { ApiError } from "../../../shared/api";
import { createTranslationWithEnglish, startTranslationFromEN } from "../../emails/api";
import type { EmailListItem } from "../../emails/types";

// CreateTranslationModal fills an empty language of a slot from its EN email:
// the AI translates every text field (glossary applied) into a new draft. The
// translation runs in the background and the cell shows it until the draft
// appears; with the English text the draft is created at once.
export function CreateTranslationModal({
  language,
  master,
  onClose,
  onCreated,
}: {
  language: string;
  master: EmailListItem;
  onClose: () => void;
  onCreated: (emailId: string) => void;
}) {
  const queryClient = useQueryClient();
  const [translate, setTranslate] = useState(true);
  const label = language.toUpperCase();
  const version = master.latest_version ? ` v${master.latest_version}` : "";

  const mutation = useMutation({
    mutationFn: async () => {
      if (translate) {
        await startTranslationFromEN(master.id, language);
        return null;
      }
      return (await createTranslationWithEnglish(master.id, language)).email.id;
    },
    onSuccess: (createdId) => {
      if (createdId) {
        void queryClient.invalidateQueries({ queryKey: ["emails"] });
        notifications.show({ color: "green", title: `${label} created`, message: "Draft with the English text, ready to translate." });
        onCreated(createdId);
      } else {
        void queryClient.invalidateQueries({ queryKey: ["translation-jobs"] });
        notifications.show({
          color: "blue",
          title: `Translating into ${label}`,
          message: "The draft appears in its cell in a minute or two. You can keep working meanwhile.",
        });
      }
      onClose();
    },
    onError: (error) =>
      notifications.show({
        color: "red",
        title: `${label} not created`,
        message: error instanceof ApiError ? error.message : "Try again.",
      }),
  });

  return (
    <Modal opened title={`Create ${label} from EN`} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          mutation.mutate();
        }}
      >
        <Stack>
          <Text size="sm">
            A new {label} draft of "{master.title}" in this slot, linked to EN{version}: same template, timing and
            condition.
          </Text>
          <Checkbox
            checked={translate}
            description="Subject, preheader and every text field; terms from the translation glossary are kept. Fields where Bitrix24 expressions would break stay in English."
            label="Pre-translate with AI"
            onChange={(event) => setTranslate(event.currentTarget.checked)}
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button loading={mutation.isPending} type="submit">
              {translate ? "Translate and create" : "Create with EN text"}
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
