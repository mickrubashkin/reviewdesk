import { Badge, Button, Group, Stack, Text, Textarea, Timeline } from "@mantine/core";
import { ChatTextIcon, CheckIcon } from "@phosphor-icons/react";
import { useState, type FormEvent, type KeyboardEvent, type RefCallback } from "react";

import type { EmailComment } from "../../../emails/types";
import {
  commentSeverityColor,
  formatCommentDate,
  formatCommentSeverity,
  getReviewTargetLabel,
} from "../EmailReview.helpers";
import styles from "../EmailReview.module.css";

export function CommentItem({
  canResolve,
  comment,
  isActive,
  isHovered,
  isReplying,
  isResolving,
  isStaleAfterEdit,
  itemRef,
  onHover,
  onReply,
  onResolve,
  onSelect,
}: {
  canResolve: boolean;
  comment: EmailComment;
  isActive: boolean;
  isHovered: boolean;
  isReplying: boolean;
  isResolving: boolean;
  isStaleAfterEdit: boolean;
  itemRef: RefCallback<HTMLDivElement>;
  onHover: (comment: EmailComment | null) => void;
  onReply: (commentId: string, body: string) => void;
  onResolve: (commentId: string) => void;
  onSelect: (comment: EmailComment) => void;
}) {
  const isResolved = comment.status === "resolved";
  const [isReplyComposerOpen, setIsReplyComposerOpen] = useState(false);
  const [replyDraft, setReplyDraft] = useState("");
  const trimmedReplyDraft = replyDraft.trim();
  const messages =
    (comment.messages?.length ?? 0) > 0
      ? comment.messages
      : [
          {
            author_email: comment.author_email,
            body: comment.body,
            comment_id: comment.id,
            created_at: comment.created_at,
            id: `${comment.id}-legacy-body`,
            updated_at: comment.created_at,
            user_id: comment.user_id,
          },
        ];
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onSelect(comment);
    }
  };
  const handleReplySubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    event.stopPropagation();
    if (!trimmedReplyDraft) {
      return;
    }

    onReply(comment.id, trimmedReplyDraft);
    setReplyDraft("");
    setIsReplyComposerOpen(false);
  };

  return (
    <Timeline.Item
      bullet={
        isResolved ? (
          <CheckIcon aria-hidden="true" size={13} weight="bold" />
        ) : (
          <ChatTextIcon aria-hidden="true" size={13} />
        )
      }
      color={isResolved ? "green" : commentSeverityColor(comment.severity)}
      title={
        <Group justify="space-between" gap="xs" wrap="nowrap">
          <Stack gap={2}>
            <Text fw={700} size="xs">
              {comment.author_email ?? "Unknown author"}
            </Text>
            <Text c="dimmed" size="xs">
              {getReviewTargetLabel(comment)}
            </Text>
          </Stack>
          <Badge
            color={isResolved ? "green" : commentSeverityColor(comment.severity)}
            size="sm"
            variant="light"
          >
            {isResolved ? "resolved" : formatCommentSeverity(comment.severity)}
          </Badge>
        </Group>
      }
    >
      <Stack
        className={styles.commentItem}
        data-active={isActive || undefined}
        data-hovered={isHovered || undefined}
        gap={8}
        ref={itemRef}
        role="button"
        tabIndex={0}
        onClick={() => onSelect(comment)}
        onKeyDown={handleKeyDown}
        onMouseEnter={() => onHover(comment)}
        onMouseLeave={() => onHover(null)}
      >
        <Text className={styles.commentQuote} size="sm">
          {comment.selected_text}
        </Text>
        {isStaleAfterEdit ? (
          <Badge color="orange" size="sm" variant="light">
            Edited after this comment
          </Badge>
        ) : null}

        <Stack className={styles.commentMessages} gap={8}>
          {messages.map((message) => (
            <Stack className={styles.commentMessage} gap={3} key={message.id}>
              <Group gap={6} wrap="nowrap">
                <Text fw={650} size="xs">
                  {message.author_email ?? "Unknown author"}
                </Text>
                <Text c="dimmed" size="xs">
                  {formatCommentDate(message.created_at)}
                </Text>
              </Group>
              <Text size="sm">{message.body}</Text>
            </Stack>
          ))}
        </Stack>

        {isReplyComposerOpen && !isResolved ? (
          <form
            className={styles.commentReplyForm}
            onClick={(event) => event.stopPropagation()}
            onSubmit={handleReplySubmit}
          >
            <Textarea
              autosize
              disabled={isReplying}
              minRows={2}
              placeholder="Reply"
              size="xs"
              value={replyDraft}
              onChange={(event) => setReplyDraft(event.currentTarget.value)}
              onClick={(event) => event.stopPropagation()}
              onKeyDown={(event) => event.stopPropagation()}
            />
            <Group justify="flex-end" gap="xs">
              <Button
                disabled={isReplying}
                size="compact-xs"
                variant="subtle"
                onClick={(event) => {
                  event.stopPropagation();
                  setReplyDraft("");
                  setIsReplyComposerOpen(false);
                }}
              >
                Cancel
              </Button>
              <Button
                disabled={!trimmedReplyDraft}
                loading={isReplying}
                size="compact-xs"
                type="submit"
                onClick={(event) => event.stopPropagation()}
              >
                Send
              </Button>
            </Group>
          </form>
        ) : null}

        <Group justify="space-between" gap="xs">
          <Text c="dimmed" size="xs">
            {formatCommentDate(comment.created_at)}
          </Text>
          {!isResolved ? (
            <Group gap={4}>
              <Button
                disabled={isReplying}
                size="compact-xs"
                variant="subtle"
                onClick={(event) => {
                  event.stopPropagation();
                  setIsReplyComposerOpen(true);
                }}
              >
                Reply
              </Button>
              {canResolve ? (
                <Button
                  loading={isResolving}
                  size="compact-xs"
                  variant="subtle"
                  onClick={(event) => {
                    event.stopPropagation();
                    onResolve(comment.id);
                  }}
                >
                  Resolve
                </Button>
              ) : null}
            </Group>
          ) : null}
        </Group>
      </Stack>
    </Timeline.Item>
  );
}
