import {
  Alert,
  Autocomplete,
  Badge,
  Button,
  FileButton,
  Group,
  NumberInput,
  Select,
  Stack,
  Text,
  Textarea,
  TextInput,
  Title,
} from "@mantine/core";
import { Link } from "react-router-dom";
import { PlusIcon, UploadSimpleIcon } from "@phosphor-icons/react";

import type { EmailHTMLInspection } from "../types";

import {
  type CreateEmailFormState,
  type EventGroupOption,
  newEventGroupValue,
} from "./EmailCreate.types";
import styles from "./EmailCreate.module.css";

export function EmailCreateFormPanel({
  adaptationOptions,
  boardOptions,
  boardsLoading,
  canSubmit,
  createError,
  eventSelectOptions,
  formState,
  htmlInspection,
  htmlInspectionError,
  htmlInspectionPending,
  htmlInspectionWarnings,
  isCreating,
  languageOptions,
  selectedBoard,
  selectedEventGroup,
  selectedSortOrder,
  selectedStage,
  sortOrderAuto,
  stageOptions,
  trimmedHTML,
  variantOptions,
  onBoardChange,
  onCreate,
  onEventGroupChange,
  onHTMLFileLoad,
  onSortOrderAutoChange,
  onStageChange,
  onUpdateField,
}: {
  adaptationOptions: string[];
  boardOptions: Array<{ label: string; value: string }>;
  boardsLoading: boolean;
  canSubmit: boolean;
  createError: string | null;
  eventSelectOptions: Array<{ label: string; value: string }>;
  formState: CreateEmailFormState;
  htmlInspection: EmailHTMLInspection | undefined;
  htmlInspectionError: boolean;
  htmlInspectionPending: boolean;
  htmlInspectionWarnings: string[];
  isCreating: boolean;
  languageOptions: string[];
  selectedBoard: string;
  selectedEventGroup: EventGroupOption | undefined;
  selectedSortOrder: number;
  selectedStage: string;
  sortOrderAuto: boolean;
  stageOptions: string[];
  trimmedHTML: string;
  variantOptions: string[];
  onBoardChange: (value: string | null) => void;
  onCreate: () => void;
  onEventGroupChange: (value: string | null) => void;
  onHTMLFileLoad: (file: File | null) => void;
  onSortOrderAutoChange: (isAuto: boolean) => void;
  onStageChange: (value: string | null) => void;
  onUpdateField: <Key extends keyof CreateEmailFormState>(
    key: Key,
    value: CreateEmailFormState[Key]
  ) => void;
}) {
  return (
    <Stack className={styles.editorContent} gap="md">
      <Stack gap={4}>
        <Title order={4}>New email</Title>
        <Text c="dimmed" size="sm">
          Upload finished HTML or paste it below.
        </Text>
      </Stack>

      {createError ? (
        <Alert color="red" title="Failed to create email">
          {createError}
        </Alert>
      ) : null}

      <Group grow>
        <Select
          allowDeselect={false}
          data={boardOptions}
          disabled={boardsLoading}
          label="Board"
          placeholder={boardsLoading ? "Loading boards" : "Select board"}
          required
          value={selectedBoard || null}
          onChange={onBoardChange}
        />
        <Select
          allowDeselect={false}
          data={stageOptions}
          disabled={!selectedBoard || stageOptions.length === 0}
          label="Stage"
          placeholder={selectedBoard ? "Select stage" : "Select a board first"}
          required
          value={selectedStage}
          onChange={onStageChange}
        />
      </Group>
      {selectedBoard && stageOptions.length === 0 ? (
        <Alert color="yellow" title="No stages">
          This board has no stages yet, so a new email cannot be placed on it.
        </Alert>
      ) : null}

      <Select
        allowDeselect={false}
        data={eventSelectOptions}
        description={
          selectedEventGroup
            ? "New language/version will be placed into this existing card."
            : "A separate event card will be created in this stage."
        }
        disabled={!selectedBoard || !selectedStage}
        label="Event"
        placeholder="Choose event"
        value={formState.eventGroupKey || newEventGroupValue}
        onChange={onEventGroupChange}
      />

      <TextInput
        label="Title"
        required
        value={formState.title}
        onChange={(event) => {
          onUpdateField("title", event.currentTarget.value);
          if (formState.eventGroupKey !== newEventGroupValue) {
            onUpdateField("eventGroupKey", newEventGroupValue);
          }
        }}
      />
      <TextInput
        label="Subject"
        value={formState.subject}
        onChange={(event) => onUpdateField("subject", event.currentTarget.value)}
      />
      <Textarea
        autosize
        label="Preheader"
        minRows={2}
        value={formState.preheader}
        onChange={(event) => onUpdateField("preheader", event.currentTarget.value)}
      />

      <Group grow>
        <TextInput
          label="Send timing"
          value={formState.sendTiming}
          onChange={(event) => onUpdateField("sendTiming", event.currentTarget.value)}
        />
        <Autocomplete
          data={languageOptions}
          label="Language"
          placeholder="en"
          value={formState.language}
          onChange={(value) => onUpdateField("language", value)}
        />
      </Group>

      <Group grow>
        <Autocomplete
          data={variantOptions}
          label="Version"
          placeholder="v1"
          value={formState.variant}
          onChange={(value) => onUpdateField("variant", value)}
        />
        <Autocomplete
          data={adaptationOptions}
          label="Adaptation"
          placeholder="Default"
          value={formState.adaptationLabel}
          onChange={(value) => onUpdateField("adaptationLabel", value)}
        />
      </Group>

      <Group grow>
        <NumberInput
          description={
            sortOrderAuto
              ? "Suggested from selected board/stage."
              : "Manual order."
          }
          label="Sort order"
          value={selectedSortOrder}
          onChange={(value) => {
            onSortOrderAutoChange(false);
            onUpdateField("sortOrder", typeof value === "number" ? value : 0);
          }}
        />
      </Group>

      <Group justify="space-between" align="center">
        <Stack gap={2}>
          <Text fw={600} size="sm">
            HTML
          </Text>
          <Text c="dimmed" size="xs">
            Existing review/edit markers are preserved.
          </Text>
        </Stack>
        <FileButton accept=".html,text/html" onChange={onHTMLFileLoad}>
          {(props) => (
            <Button
              {...props}
              leftSection={<UploadSimpleIcon aria-hidden="true" size={16} />}
              size="xs"
              variant="light"
            >
              Upload HTML
            </Button>
          )}
        </FileButton>
      </Group>
      <Textarea
        autosize
        minRows={12}
        placeholder="<html>...</html>"
        value={formState.originalHTML}
        onChange={(event) => onUpdateField("originalHTML", event.currentTarget.value)}
      />
      {trimmedHTML ? (
        <EmailHTMLInspectionStatus
          htmlInspection={htmlInspection}
          htmlInspectionError={htmlInspectionError}
          htmlInspectionPending={htmlInspectionPending}
          htmlInspectionWarnings={htmlInspectionWarnings}
        />
      ) : null}

      <Group justify="flex-end">
        <Button component={Link} to="/" variant="subtle">
          Cancel
        </Button>
        <Button
          disabled={!canSubmit}
          leftSection={<PlusIcon aria-hidden="true" size={16} />}
          loading={isCreating}
          onClick={onCreate}
        >
          Create email
        </Button>
      </Group>
    </Stack>
  );
}

function EmailHTMLInspectionStatus({
  htmlInspection,
  htmlInspectionError,
  htmlInspectionPending,
  htmlInspectionWarnings,
}: {
  htmlInspection: EmailHTMLInspection | undefined;
  htmlInspectionError: boolean;
  htmlInspectionPending: boolean;
  htmlInspectionWarnings: string[];
}) {
  return (
    <Alert
      color={
        htmlInspectionError || htmlInspectionWarnings.length > 0
          ? "yellow"
          : "green"
      }
      title="Backend HTML inspection"
    >
      <Stack gap={6}>
        {htmlInspectionPending ? (
          <Text size="sm">Inspecting HTML...</Text>
        ) : htmlInspectionError ? (
          <Text size="sm">
            HTML could not be inspected. Check the markup and editable field
            markers.
          </Text>
        ) : htmlInspection ? (
          <>
            <Group gap={6} wrap="wrap">
              <Badge
                color={htmlInspection.review_block_count > 0 ? "green" : "yellow"}
                variant="light"
              >
                {htmlInspection.review_block_count} review blocks
              </Badge>
              <Badge
                color={htmlInspection.editable_field_count > 0 ? "green" : "yellow"}
                variant="light"
              >
                {htmlInspection.editable_field_count} editable fields
              </Badge>
              {htmlInspection.original_review_block_count === 0 ? (
                <Badge color="blue" variant="light">
                  generated review blocks
                </Badge>
              ) : null}
            </Group>
            {htmlInspectionWarnings.length > 0 ? (
              <Stack gap={2}>
                {htmlInspectionWarnings.map((warning) => (
                  <Text key={warning} size="sm">
                    {warning}
                  </Text>
                ))}
              </Stack>
            ) : (
              <Text size="sm">Review and editable markers look usable.</Text>
            )}
            {htmlInspection.editable_fields.length > 0 ? (
              <Group gap={6} wrap="wrap">
                {htmlInspection.editable_fields.slice(0, 8).map((field) => (
                  <Badge key={field.key} color="gray" variant="light">
                    {field.key}: {field.type}
                  </Badge>
                ))}
                {htmlInspection.editable_fields.length > 8 ? (
                  <Badge color="gray" variant="light">
                    +{htmlInspection.editable_fields.length - 8} more
                  </Badge>
                ) : null}
              </Group>
            ) : null}
          </>
        ) : null}
      </Stack>
    </Alert>
  );
}
