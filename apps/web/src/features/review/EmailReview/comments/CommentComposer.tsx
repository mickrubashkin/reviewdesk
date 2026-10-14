import { Button, Group, SegmentedControl, Select, Stack, Text, Textarea } from "@mantine/core";
import type { FormEvent } from "react";

import type { EmailCommentSeverity } from "../../../emails/types";
import type { CommentBlockOption } from "../EmailReview.types";
import styles from "../EmailReview.module.css";

export function CommentComposer({
  blockOptions,
  createCommentError,
  draftComment,
  draftSeverity,
  isCreatingComment,
  onCancel,
  onChangeDraftComment,
  onChangeDraftSeverity,
  onChangeSelectedBlockValue,
  onSubmit,
  selectedBlock,
}: {
  blockOptions: CommentBlockOption[];
  createCommentError: boolean;
  draftComment: string;
  draftSeverity: EmailCommentSeverity;
  isCreatingComment: boolean;
  onCancel: () => void;
  onChangeDraftComment: (value: string) => void;
  onChangeDraftSeverity: (severity: EmailCommentSeverity) => void;
  onChangeSelectedBlockValue: (value: string | null) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  selectedBlock: CommentBlockOption | null;
}) {
  const trimmedDraftComment = draftComment.trim();

  return (
    <form className={styles.panelCommentComposer} onSubmit={onSubmit}>
      <Stack gap="xs">
        <Select
          data={blockOptions.map((option) => ({
            label: option.label,
            value: option.value,
          }))}
          disabled={isCreatingComment || blockOptions.length === 0}
          label="Review block"
          placeholder="Select a block"
          value={selectedBlock?.value ?? null}
          onChange={onChangeSelectedBlockValue}
        />
        {selectedBlock ? (
          <Text className={styles.commentQuote} lineClamp={3} size="sm">
            {selectedBlock.selectedText}
          </Text>
        ) : (
          <Text c="dimmed" size="sm">
            No review blocks found. Select text in the preview instead.
          </Text>
        )}
        <Textarea
          autosize
          disabled={isCreatingComment || !selectedBlock}
          minRows={3}
          placeholder="Add a comment"
          value={draftComment}
          onChange={(event) => onChangeDraftComment(event.currentTarget.value)}
        />
        <SegmentedControl
          data={[
            { label: "Suggestion", value: "suggestion" },
            { label: "Issue", value: "issue" },
            { label: "Blocking", value: "blocking" },
          ]}
          disabled={isCreatingComment}
          size="xs"
          value={draftSeverity}
          onChange={(value) =>
            onChangeDraftSeverity(value as EmailCommentSeverity)
          }
        />
        {createCommentError ? (
          <Text c="red" size="xs">
            Failed to create comment.
          </Text>
        ) : null}
        <Text c="dimmed" size="xs">
          For precise text comments, select text or click a block in the email
          preview.
        </Text>
        <Group justify="flex-end" gap="xs">
          <Button
            disabled={isCreatingComment}
            size="xs"
            variant="subtle"
            onClick={onCancel}
          >
            Cancel
          </Button>
          <Button
            disabled={!selectedBlock || !trimmedDraftComment}
            loading={isCreatingComment}
            size="xs"
            type="submit"
          >
            Add comment
          </Button>
        </Group>
      </Stack>
    </form>
  );
}
