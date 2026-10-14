import {
  Alert,
  Button,
  Group,
  Modal,
  Select,
  Stack,
  Text,
  Textarea,
  TextInput,
} from "@mantine/core";

import { type FormEvent, useMemo, useState } from "react";

import { ApiError } from "../../../shared/api";
import { formatStageName } from "../../emails/stages";
import type {
  Board,
  DuplicateEmailPayload,
  EmailDetail,
  EmailListItem,
  EmailVariant,
} from "../../emails/types";

const currentEventValue = "current-event";
const newEventValue = "new-event";

export function SourceHTMLModal({
  draft,
  isSubmitting,
  onChange,
  onClose,
  onSubmit,
  opened,
}: {
  draft: string;
  isSubmitting: boolean;
  onChange: (draft: string) => void;
  onClose: () => void;
  onSubmit: () => void;
  opened: boolean;
}) {
  return (
    <Modal opened={opened} size="xl" title="Edit template HTML" onClose={onClose}>
      <Stack gap="sm">
        <Textarea
          autosize
          disabled={isSubmitting}
          minRows={18}
          value={draft}
          onChange={(event) => onChange(event.currentTarget.value)}
        />
        <Group justify="flex-end" gap="xs">
          <Button disabled={isSubmitting} variant="subtle" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={isSubmitting} onClick={onSubmit}>
            Save template HTML
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}

export function ArchiveEmailModal({
  email,
  isSubmitting,
  onClose,
  onSubmit,
  opened,
}: {
  email: EmailDetail;
  isSubmitting: boolean;
  onClose: () => void;
  onSubmit: (emailId: string) => Promise<void>;
  opened: boolean;
}) {
  const handleArchive = async () => {
    try {
      await onSubmit(email.id);
    } catch {
      // The parent mutation shows the notification; keep the modal open.
    }
  };

  return (
    <Modal centered opened={opened} title="Archive email" onClose={onClose}>
      <Stack gap="sm">
        <Text size="sm">
          Archive "{email.title}"? It will be hidden from the active board, but
          comments and history will stay in the database.
        </Text>
        <Group justify="flex-end" mt="xs">
          <Button
            disabled={isSubmitting}
            type="button"
            variant="default"
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            color="red"
            loading={isSubmitting}
            onClick={handleArchive}
          >
            Archive email
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}


export function DuplicateEmailModal({
  boards,
  boardsLoading,
  email,
  emails,
  emailsLoading,
  error,
  isSubmitting,
  onClose,
  onResetError,
  onSubmit,
}: {
  boards: Board[];
  boardsLoading: boolean;
  email: EmailDetail;
  emails: EmailListItem[];
  emailsLoading: boolean;
  error: Error | null;
  isSubmitting: boolean;
  onClose: () => void;
  onResetError: () => void;
  onSubmit: (emailId: string, payload: DuplicateEmailPayload) => Promise<void>;
}) {
  const [sequence, setSequence] = useState(email.sequence);
  const [stage, setStage] = useState(email.stage ?? "");
  const [eventMode, setEventMode] = useState(currentEventValue);
  const [language, setLanguage] = useState(email.language);
  const [variant, setVariant] = useState<EmailVariant>(email.variant);
  const [adaptationLabel, setAdaptationLabel] = useState(email.adaptation_label);
  const [newTitle, setNewTitle] = useState("");
  const [subject, setSubject] = useState(email.subject ?? "");
  const [preheader, setPreheader] = useState(email.preheader ?? "");
  const [sequenceError, setSequenceError] = useState<string | null>(null);
  const [stageError, setStageError] = useState<string | null>(null);
  const [titleError, setTitleError] = useState<string | null>(null);
  const [languageError, setLanguageError] = useState<string | null>(null);
  const [variantError, setVariantError] = useState<string | null>(null);
  const [adaptationError, setAdaptationError] = useState<string | null>(null);
  const [targetError, setTargetError] = useState<string | null>(null);
  const boardOptions = useMemo(
    () => boards.map((board) => ({ label: board.name, value: board.key })),
    [boards]
  );
  const selectedBoard = boards.find((board) => board.key === sequence);
  const stageOptions = selectedBoard?.stages ?? [];
  const selectedStage = stageOptions.includes(stage)
    ? stage
    : stageOptions[0] ?? "";
  const normalizedNewTitle = newTitle.trim();
  const targetTitle =
    eventMode === newEventValue ? normalizedNewTitle : email.title;
  const shouldUseNewSortOrder =
    eventMode === newEventValue ||
    sequence !== email.sequence ||
    selectedStage !== (email.stage ?? "");
  const sortOrder = shouldUseNewSortOrder
    ? getNextSortOrder(emails, sequence, selectedStage)
    : email.sort_order;

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    const normalizedSequence = sequence.trim();
    const normalizedStage = selectedStage.trim();
    const normalizedLanguage = language.trim().toLowerCase();
    const normalizedVariant = variant.trim().toLowerCase();
    const normalizedAdaptationLabel = adaptationLabel.trim();
    if (!normalizedSequence) {
      setSequenceError("Board is required");
      return;
    }
    if (!normalizedStage) {
      setStageError("Stage is required");
      return;
    }
    if (eventMode === newEventValue && !normalizedNewTitle) {
      setTitleError("Title is required");
      return;
    }
    if (!normalizedLanguage) {
      setLanguageError("Language is required");
      return;
    }
    if (!normalizedVariant) {
      setVariantError("Version is required");
      return;
    }
    if (!normalizedAdaptationLabel) {
      setAdaptationError("Adaptation is required");
      return;
    }

    const payload: DuplicateEmailPayload = {
      sequence: normalizedSequence,
      sort_order: sortOrder,
      stage: normalizedStage,
      ...buildOptionalTextPayload("title", targetTitle, email.title),
      ...buildOptionalTextPayload("subject", subject, email.subject),
      ...buildOptionalTextPayload("preheader", preheader, email.preheader),
    };
    if (normalizedLanguage !== email.language) {
      payload.language = normalizedLanguage;
    }
    if (normalizedVariant !== email.variant) {
      payload.variant = normalizedVariant;
    }
    if (normalizedAdaptationLabel !== email.adaptation_label) {
      payload.adaptation_label = normalizedAdaptationLabel;
    }
    const hasTargetChange =
      normalizedSequence !== email.sequence ||
      normalizedStage !== (email.stage ?? "") ||
      sortOrder !== email.sort_order ||
      normalizedLanguage !== email.language ||
      normalizedVariant !== email.variant ||
      normalizedAdaptationLabel !== email.adaptation_label ||
      targetTitle !== email.title ||
      subject !== (email.subject ?? "") ||
      preheader !== (email.preheader ?? "");
    if (!hasTargetChange) {
      setTargetError("Change board, stage, language, version, adaptation, event, or copy before creating.");
      return;
    }

    setSequenceError(null);
    setStageError(null);
    setTitleError(null);
    setLanguageError(null);
    setVariantError(null);
    setAdaptationError(null);
    setTargetError(null);
    try {
      await onSubmit(email.id, payload);
    } catch {
      // React Query stores the error; keep the modal open and show it inline.
    }
  };

  const isConflict = error instanceof ApiError && error.status === 409;

  return (
    <Modal centered opened title="Duplicate as..." onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <Stack gap="sm">
          {error ? (
            <Alert color="red" title="Could not duplicate email">
              {isConflict
                ? "This language, version, and adaptation already exist for the selected email."
                : "Try again or check that the API server is reachable."}
            </Alert>
          ) : null}
          {targetError ? (
            <Alert color="yellow" title="Nothing to create">
              {targetError}
            </Alert>
          ) : null}

          <Group grow>
            <Select
              allowDeselect={false}
              data={boardOptions}
              disabled={boardsLoading || isSubmitting}
              error={sequenceError}
              label="Board"
              placeholder={boardsLoading ? "Loading boards" : "Select board"}
              required
              searchable
              value={sequence || null}
              onChange={(value) => {
                onResetError();
                setTargetError(null);
                setSequenceError(null);
                const nextSequence = value ?? "";
                const nextStages =
                  boards.find((board) => board.key === nextSequence)?.stages ?? [];
                setSequence(nextSequence);
                setStage(
                  nextSequence === email.sequence && nextStages.includes(email.stage ?? "")
                    ? email.stage ?? ""
                    : nextStages[0] ?? ""
                );
              }}
            />
            <Select
              allowDeselect={false}
              data={stageOptions.map((stageOption) => ({
                label: formatStageName(stageOption),
                value: stageOption,
              }))}
              disabled={!sequence || stageOptions.length === 0 || isSubmitting}
              error={stageError}
              label="Stage"
              placeholder={sequence ? "Select stage" : "Select a board first"}
              required
              searchable
              value={selectedStage || null}
              onChange={(value) => {
                onResetError();
                setTargetError(null);
                setStageError(null);
                setStage(value ?? "");
              }}
            />
          </Group>

          <Select
            allowDeselect={false}
            data={[
              { label: `Current event: ${email.title}`, value: currentEventValue },
              { label: "Create new event", value: newEventValue },
            ]}
            disabled={isSubmitting}
            label="Event"
            value={eventMode}
            onChange={(value) => {
              onResetError();
              setTargetError(null);
              setTitleError(null);
              setEventMode(value ?? currentEventValue);
            }}
          />

          {eventMode === newEventValue ? (
            <TextInput
              data-autofocus
              disabled={isSubmitting}
              error={titleError}
              label="New event title"
              required
              value={newTitle}
              onChange={(event) => {
                onResetError();
                setTargetError(null);
                setTitleError(null);
                setNewTitle(event.currentTarget.value);
              }}
            />
          ) : null}

          <TextInput
            disabled={isSubmitting}
            error={languageError}
            label="Language"
            placeholder="es"
            value={language}
            onChange={(event) => {
              onResetError();
              setTargetError(null);
              setLanguage(event.currentTarget.value);
              if (languageError) {
                setLanguageError(null);
              }
            }}
          />

          <TextInput
            disabled={isSubmitting}
            error={variantError}
            label="Version"
            placeholder="v2"
            value={variant}
            onChange={(event) => {
              onResetError();
              setTargetError(null);
              setVariant(event.currentTarget.value);
              if (variantError) {
                setVariantError(null);
              }
            }}
          />

          <TextInput
            disabled={isSubmitting}
            error={adaptationError}
            label="Adaptation"
            placeholder="Legal"
            value={adaptationLabel}
            onChange={(event) => {
              onResetError();
              setTargetError(null);
              setAdaptationLabel(event.currentTarget.value);
              if (adaptationError) {
                setAdaptationError(null);
              }
            }}
          />

          <TextInput
            disabled={isSubmitting}
            label="Subject"
            value={subject}
            onChange={(event) => {
              onResetError();
              setTargetError(null);
              setSubject(event.currentTarget.value);
            }}
          />

          <TextInput
            disabled={isSubmitting}
            label="Preheader"
            value={preheader}
            onChange={(event) => {
              onResetError();
              setTargetError(null);
              setPreheader(event.currentTarget.value);
            }}
          />

          <Group justify="flex-end" mt="xs">
            <Button
              disabled={isSubmitting}
              type="button"
              variant="default"
              onClick={onClose}
            >
              Cancel
            </Button>
            <Button
              disabled={boardsLoading || emailsLoading}
              loading={isSubmitting}
              type="submit"
            >
              Duplicate as
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}

function getNextSortOrder(
  emails: Array<{ sequence: string; stage: string | null; sort_order: number }>,
  sequence: string,
  stage: string
) {
  const matchingSortOrders = emails
    .filter((email) => email.sequence === sequence && (email.stage ?? "") === stage)
    .map((email) => email.sort_order);

  if (matchingSortOrders.length === 0) {
    return 0;
  }

  return Math.max(...matchingSortOrders) + 1;
}


function buildOptionalTextPayload<Key extends "title" | "subject" | "preheader">(
  key: Key,
  value: string,
  originalValue: string | null
): Partial<Record<Key, string>> {
  if (value === "" && originalValue === null) {
    return {};
  }

  if (key === "title" && value.trim() === "") {
    return {};
  }

  return {
    [key]: key === "title" ? value.trim() : value,
  } as Partial<Record<Key, string>>;
}
