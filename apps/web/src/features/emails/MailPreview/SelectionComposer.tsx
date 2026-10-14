import {
  ActionIcon,
  Button,
  Group,
  Popover,
  SegmentedControl,
  Stack,
  Text,
  Textarea,
  TextInput,
  Tooltip,
} from "@mantine/core";
import {
  ChatTextIcon,
  CodeIcon,
  PencilSimpleIcon,
} from "@phosphor-icons/react";

import type { Dispatch, SetStateAction } from "react";

import type { BitrixVariable } from "../../bitrix-variables/api";
import type { EmailCommentSeverity } from "../types";
import { shouldUseInlineTextarea } from "./MailPreview.helpers";
import type { InlineEditInput, SelectionMenuState } from "./MailPreview";
import { VariableInsertMenu } from "./VariableInsertMenu";
import styles from "../EmailPreviewDrawer.module.css";

type SelectionComposerProps = {
  canEditContent: boolean;
  canEditHTML: boolean;
  createCommentError: boolean;
  draftComment: string;
  draftCommentSeverity: EmailCommentSeverity;
  draftInlineEdit: Record<string, string | number>;
  inlineEditError: boolean;
  isApplyingInlineEdit: boolean;
  isCreatingComment: boolean;
  selectionMenu: SelectionMenuState | null;
  trimmedDraft: string;
  variables?: BitrixVariable[];
  onApplyInlineEdit: () => void;
  onClose: () => void;
  onDraftCommentChange: (value: string) => void;
  onDraftCommentSeverityChange: (value: EmailCommentSeverity) => void;
  onDraftInlineEditChange: Dispatch<SetStateAction<Record<string, string | number>>>;
  onEditSourceHTML?: () => void;
  onOpenCommentComposer: () => void;
  onOpenInlineEditor: () => void;
  onSubmitComment: () => void;
};

export function SelectionComposer({
  canEditContent,
  canEditHTML,
  createCommentError,
  draftComment,
  draftCommentSeverity,
  draftInlineEdit,
  inlineEditError,
  isApplyingInlineEdit,
  isCreatingComment,
  selectionMenu,
  trimmedDraft,
  variables = [],
  onApplyInlineEdit,
  onClose,
  onDraftCommentChange,
  onDraftCommentSeverityChange,
  onDraftInlineEditChange,
  onEditSourceHTML,
  onOpenCommentComposer,
  onOpenInlineEditor,
  onSubmitComment,
}: SelectionComposerProps) {
  const variableMenu = (input: InlineEditInput) => {
    if (input.type === "number" || variables.length === 0) {
      return undefined;
    }
    return (
      <VariableInsertMenu
        disabled={isApplyingInlineEdit}
        variables={variables}
        onInsert={(expression) =>
          onDraftInlineEditChange((current) => {
            const value = String(current[input.key] ?? input.value);
            return {
              ...current,
              [input.key]:
                input.type === "text" && value.trim() !== ""
                  ? `${value.trimEnd()} ${expression}`
                  : expression,
            };
          })
        }
      />
    );
  };

  return (
    <Popover
      opened={selectionMenu !== null}
      position="top"
      shadow="md"
      styles={{
        dropdown: {
          maxWidth: 340,
          width: 340,
        },
      }}
      withArrow
    >
      <Popover.Target>
        <div
          className={styles.selectionMenuAnchor}
          style={{
            left: selectionMenu?.x ?? 0,
            top: selectionMenu?.y ?? 0,
          }}
        />
      </Popover.Target>
      <Popover.Dropdown className={styles.selectionMenuDropdown}>
        {selectionMenu?.mode === "comment" ? (
          <Stack gap="xs">
            <Text c="dimmed" size="xs">
              {selectionMenu.reviewBlock}
            </Text>
            <Text className={styles.selectionMenuQuote} size="sm" lineClamp={3}>
              {selectionMenu.selectedText}
            </Text>
            <Textarea
              autosize
              data-autofocus
              disabled={isCreatingComment}
              minRows={3}
              placeholder="Add a comment"
              value={draftComment}
              onChange={(event) => onDraftCommentChange(event.currentTarget.value)}
            />
            <SegmentedControl
              data={[
                { label: "Suggestion", value: "suggestion" },
                { label: "Issue", value: "issue" },
                { label: "Blocking", value: "blocking" },
              ]}
              disabled={isCreatingComment}
              size="xs"
              value={draftCommentSeverity}
              onChange={(value) =>
                onDraftCommentSeverityChange(value as EmailCommentSeverity)
              }
            />
            {createCommentError ? (
              <Text c="red" size="xs">
                Failed to create comment.
              </Text>
            ) : null}
            <Group justify="flex-end" gap="xs">
              <Button
                disabled={isCreatingComment}
                size="xs"
                variant="subtle"
                onClick={onClose}
              >
                Cancel
              </Button>
              <Button
                disabled={!trimmedDraft}
                loading={isCreatingComment}
                size="xs"
                onClick={onSubmitComment}
              >
                Add comment
              </Button>
            </Group>
          </Stack>
        ) : selectionMenu?.mode === "edit" && selectionMenu.editableTarget ? (
          <Stack gap="xs">
            <Text fw={700} size="sm">
              {selectionMenu.editableTarget.label}
            </Text>
            {selectionMenu.editableTarget.inputs.map((input, index) =>
              input.type === "text" && shouldUseInlineTextarea(input) ? (
                <Textarea
                  autosize
                  data-autofocus={index === 0 || undefined}
                  disabled={isApplyingInlineEdit}
                  key={input.key}
                  label={input.label}
                  minRows={3}
                  rightSection={variableMenu(input)}
                  value={String(draftInlineEdit[input.key] ?? input.value)}
                  onChange={(event) => {
                    const value = event.currentTarget.value;
                    onDraftInlineEditChange((current) => ({
                      ...current,
                      [input.key]: value,
                    }));
                  }}
                />
              ) : (
                <TextInput
                  data-autofocus={index === 0 || undefined}
                  disabled={isApplyingInlineEdit}
                  key={input.key}
                  label={input.label}
                  rightSection={variableMenu(input)}
                  type={input.type === "number" ? "number" : "text"}
                  value={String(draftInlineEdit[input.key] ?? input.value)}
                  onChange={(event) => {
                    const value = event.currentTarget.value;
                    onDraftInlineEditChange((current) => ({
                      ...current,
                      [input.key]:
                        input.type === "number"
                          ? Number(value) || 0
                          : value,
                    }));
                  }}
                />
              )
            )}
            {inlineEditError ? (
              <Text c="red" size="xs">
                Failed to save edit.
              </Text>
            ) : null}
            <Group justify="flex-end" gap="xs">
              <Button
                disabled={isApplyingInlineEdit}
                size="xs"
                variant="subtle"
                onClick={onClose}
              >
                Cancel
              </Button>
              <Button
                loading={isApplyingInlineEdit}
                size="xs"
                onClick={onApplyInlineEdit}
              >
                Apply
              </Button>
            </Group>
          </Stack>
        ) : (
          <Stack gap="xs">
            <Text c="dimmed" size="xs">
              {selectionMenu?.reviewBlock}
            </Text>
            <Text className={styles.selectionMenuQuote} size="sm" lineClamp={3}>
              {selectionMenu?.selectedText}
            </Text>
            <Group gap="xs" justify="center">
              <Tooltip label="Comment">
                <ActionIcon
                  aria-label="Comment"
                  size="lg"
                  variant="light"
                  onClick={onOpenCommentComposer}
                >
                  <ChatTextIcon aria-hidden="true" size={18} />
                </ActionIcon>
              </Tooltip>
              {canEditContent && selectionMenu?.editableTarget ? (
                <Tooltip
                  label={
                    isApplyingInlineEdit
                      ? "Preview is refreshing"
                      : "Edit"
                  }
                >
                  <ActionIcon
                    aria-label="Edit"
                    disabled={isApplyingInlineEdit}
                    size="lg"
                    variant="light"
                    onClick={onOpenInlineEditor}
                  >
                    <PencilSimpleIcon aria-hidden="true" size={18} />
                  </ActionIcon>
                </Tooltip>
              ) : null}
              {canEditHTML ? (
                <Tooltip
                  label={
                    isApplyingInlineEdit
                      ? "Preview is refreshing"
                      : "Edit template HTML"
                  }
                >
                  <ActionIcon
                    aria-label="Edit template HTML"
                    disabled={isApplyingInlineEdit}
                    size="lg"
                    variant="light"
                    onClick={() => {
                      onClose();
                      onEditSourceHTML?.();
                    }}
                  >
                    <CodeIcon aria-hidden="true" size={18} />
                  </ActionIcon>
                </Tooltip>
              ) : null}
            </Group>
          </Stack>
        )}
      </Popover.Dropdown>
    </Popover>
  );
}
