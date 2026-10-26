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

import { useRef, type Dispatch, type SetStateAction } from "react";

import type { BitrixVariable } from "../../bitrix-variables/api";
import type { SharedText } from "../../shared-texts/api";
import type { EmailCommentSeverity } from "../types";
import { shouldUseInlineTextarea } from "./MailPreview.helpers";
import type { InlineEditInput, SelectionMenuState } from "./MailPreview";
import { MarkdownToolbar } from "./MarkdownToolbar";
import { VariableInsertMenu } from "./VariableInsertMenu";
import { MentionTextarea } from "../../comments/MentionTextarea";
import styles from "../EmailPreviewDrawer.module.css";

type SelectionComposerProps = {
  canEditContent: boolean;
  canEditHTML: boolean;
  createCommentError: boolean;
  draftComment: string;
  draftCommentSeverity: EmailCommentSeverity;
  draftInlineEdit: Record<string, string | number>;
  draftShared: Record<string, boolean>;
  fieldLimits: Record<string, number>;
  inlineEditError: boolean;
  inlineEditErrorMessage?: string;
  isApplyingInlineEdit: boolean;
  isCreatingComment: boolean;
  language?: string;
  selectionMenu: SelectionMenuState | null;
  sharedTexts: Record<string, SharedText>;
  trimmedDraft: string;
  variables?: BitrixVariable[];
  onApplyInlineEdit: () => void;
  onClose: () => void;
  onDraftCommentChange: (value: string) => void;
  onDraftCommentSeverityChange: (value: EmailCommentSeverity) => void;
  onDraftInlineEditChange: Dispatch<SetStateAction<Record<string, string | number>>>;
  onDraftSharedChange: Dispatch<SetStateAction<Record<string, boolean>>>;
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
  draftShared,
  fieldLimits,
  inlineEditError,
  inlineEditErrorMessage,
  isApplyingInlineEdit,
  isCreatingComment,
  language = "",
  selectionMenu,
  sharedTexts,
  trimmedDraft,
  variables = [],
  onApplyInlineEdit,
  onClose,
  onDraftCommentChange,
  onDraftCommentSeverityChange,
  onDraftInlineEditChange,
  onDraftSharedChange,
  onEditSourceHTML,
  onOpenCommentComposer,
  onOpenInlineEditor,
  onSubmitComment,
}: SelectionComposerProps) {
  const textareas = useRef<Record<string, HTMLTextAreaElement | null>>({});
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
            <MentionTextarea
              autosize
              data-autofocus
              disabled={isCreatingComment}
              minRows={3}
              placeholder="Add a comment, @ to mention"
              value={draftComment}
              withinPortal={false}
              onChange={onDraftCommentChange}
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
            {selectionMenu.editableTarget.inputs.map((input, index) => {
              const sharedText = input.field ? sharedTexts[input.key] : undefined;
              const linkable = sharedText !== undefined && sharedText.type === input.field?.type;
              const isShared = draftShared[input.key] ?? Boolean(input.field?.shared);
              const value = isShared
                ? (sharedText?.value ?? String(input.value))
                : String(draftInlineEdit[input.key] ?? input.value);
              const disabled = isApplyingInlineEdit || isShared;
              const rightSection = isShared ? undefined : variableMenu(input);
              const limit = input.type === "text" && !isShared ? fieldLimits[input.key] : undefined;
              const counter = limit ? (
                <Text c={value.length > limit ? "red" : "dimmed"} component="span" size="xs">
                  {value.length} / {limit}
                </Text>
              ) : undefined;

              return (
                <Stack gap={4} key={input.key}>
                  {input.formattable && !isShared ? (
                    <MarkdownToolbar
                      disabled={disabled}
                      getTextarea={() => textareas.current[input.key] ?? null}
                      value={value}
                      onChange={(nextValue) =>
                        onDraftInlineEditChange((current) => ({ ...current, [input.key]: nextValue }))
                      }
                    />
                  ) : null}
                  {input.type === "text" && shouldUseInlineTextarea(input) ? (
                    <Textarea
                      ref={(element) => {
                        textareas.current[input.key] = element;
                      }}
                      autosize
                      data-autofocus={(index === 0 && !isShared) || undefined}
                      description={counter}
                      disabled={disabled}
                      inputWrapperOrder={["label", "input", "description"]}
                      label={input.label}
                      minRows={3}
                      rightSection={rightSection}
                      value={value}
                      onChange={(event) => {
                        const nextValue = event.currentTarget.value;
                        onDraftInlineEditChange((current) => ({
                          ...current,
                          [input.key]: nextValue,
                        }));
                      }}
                    />
                  ) : (
                    <TextInput
                      data-autofocus={(index === 0 && !isShared) || undefined}
                      description={counter}
                      disabled={disabled}
                      inputWrapperOrder={["label", "input", "description"]}
                      label={input.label}
                      rightSection={rightSection}
                      type={input.type === "number" ? "number" : "text"}
                      value={value}
                      onChange={(event) => {
                        const nextValue = event.currentTarget.value;
                        onDraftInlineEditChange((current) => ({
                          ...current,
                          [input.key]:
                            input.type === "number"
                              ? Number(nextValue) || 0
                              : nextValue,
                        }));
                      }}
                    />
                  )}
                  {isShared ? (
                    <Stack align="flex-start" gap={2}>
                      <Text c="dimmed" size="xs">
                        Shared text for {language.toUpperCase() || "this language"}
                        {sharedText?.board ? " on this board" : ""}
                        {sharedText ? `, used in ${sharedText.linked_emails} emails` : ""}. Edit
                        it in Settings → Shared texts.
                      </Text>
                      <Button
                        disabled={isApplyingInlineEdit}
                        px={0}
                        size="compact-xs"
                        variant="transparent"
                        onClick={() => {
                          onDraftInlineEditChange((current) => ({ ...current, [input.key]: value }));
                          onDraftSharedChange((current) => ({ ...current, [input.key]: false }));
                        }}
                      >
                        Use own text
                      </Button>
                    </Stack>
                  ) : linkable ? (
                    <Stack align="flex-start" gap={2}>
                      <Text c="dimmed" size="xs">
                        {value === sharedText.value
                          ? "Same as the shared text, but not linked to it."
                          : "Differs from the shared text."}
                      </Text>
                      <Button
                        disabled={isApplyingInlineEdit}
                        px={0}
                        size="compact-xs"
                        variant="transparent"
                        onClick={() =>
                          onDraftSharedChange((current) => ({ ...current, [input.key]: true }))
                        }
                      >
                        Use shared text
                      </Button>
                    </Stack>
                  ) : null}
                </Stack>
              );
            })}
            {inlineEditError ? (
              <Text c="red" size="xs">
                {inlineEditErrorMessage || "Failed to save edit."}
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
