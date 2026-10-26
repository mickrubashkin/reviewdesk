import { Badge, Button, Group, Stack, Text, Tooltip } from "@mantine/core";
import { useState, type FormEvent } from "react";

import { MentionTextarea } from "../../comments/MentionTextarea";
import { MessageBody } from "../../comments/MessageBody";
import type { EmailComment, EmailCommentMessage } from "../../emails/types";
import {
  commentSeverityColor,
  formatCommentSeverity,
  getReviewTargetLabel,
} from "../../review/EmailReview/EmailReview.helpers";
import { useUserName } from "../../../shared/userNames";
import styles from "./EmailSidePanel.module.css";

// Threads longer than this show the first and the last message, with the
// ones in between behind "N more".
const collapseAfter = 3;

const shortDate = new Intl.DateTimeFormat(undefined, {
  day: "numeric",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
});

function formatShortDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : shortDate.format(date);
}

function sameAuthorSameDay(a: EmailCommentMessage, b: EmailCommentMessage) {
  return (
    (a.author_email ?? "").toLowerCase() === (b.author_email ?? "").toLowerCase() &&
    a.created_at.slice(0, 10) === b.created_at.slice(0, 10)
  );
}

// PanelThread is a compact comment thread for the board side panel: one
// header line, the commented text cut to one line, and the messages.
export function PanelThread({
  canResolve,
  comment,
  isReplying,
  isResolving,
  onOpen,
  onReply,
  onResolve,
}: {
  canResolve: boolean;
  comment: EmailComment;
  isReplying: boolean;
  isResolving: boolean;
  onOpen: () => void;
  onReply: (body: string) => void;
  onResolve: () => void;
}) {
  const userName = useUserName();
  const [expanded, setExpanded] = useState(false);
  const [replyOpen, setReplyOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const isResolved = comment.status !== "open";
  const messages = comment.messages ?? [];
  const hidden = !expanded && messages.length > collapseAfter ? messages.length - 2 : 0;
  const shown = hidden > 0 ? [messages[0], messages[messages.length - 1]] : messages;

  const handleReply = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!draft.trim()) {
      return;
    }
    onReply(draft.trim());
    setDraft("");
    setReplyOpen(false);
  };

  return (
    <div
      className={styles.thread}
      data-resolved={isResolved || undefined}
      style={{ ["--thread-color" as string]: `var(--mantine-color-${isResolved ? "green" : commentSeverityColor(comment.severity)}-5)` }}
    >
      <Group gap={6} justify="space-between" wrap="nowrap">
        <Text fw={600} size="xs" truncate>
          {getReviewTargetLabel(comment)}
        </Text>
        {isResolved ? (
          <Badge color="green" size="xs" variant="light">
            resolved
          </Badge>
        ) : comment.severity !== "issue" ? (
          <Badge color={commentSeverityColor(comment.severity)} size="xs" variant="light">
            {formatCommentSeverity(comment.severity)}
          </Badge>
        ) : null}
      </Group>
      <Tooltip label={comment.selected_text} maw={420} multiline openDelay={400} withinPortal>
        <Text c="dimmed" className={styles.threadQuote} lineClamp={1} size="xs">
          {comment.selected_text}
        </Text>
      </Tooltip>

      <Stack gap={4} mt={4}>
        {shown.map((message, index) => {
          const previous = index > 0 ? shown[index - 1] : null;
          const showHeader = !previous || hidden > 0 || !sameAuthorSameDay(previous, message);
          return (
            <div key={message.id}>
              {index === 1 && hidden > 0 ? (
                <Button mb={4} size="compact-xs" variant="subtle" onClick={() => setExpanded(true)}>
                  {hidden} more
                </Button>
              ) : null}
              {showHeader ? (
                <Text c="dimmed" size="xs">
                  <Text c="dark" component="span" fw={600} inherit>
                    {userName(message.author_email, "Unknown author")}
                  </Text>{" "}
                  · {formatShortDate(message.created_at)}
                </Text>
              ) : null}
              <MessageBody body={message.body} mentionEmails={message.mention_emails} />
            </div>
          );
        })}
      </Stack>

      {replyOpen && !isResolved ? (
        <form className={styles.threadReply} onSubmit={handleReply}>
          <MentionTextarea
            autosize
            data-autofocus
            disabled={isReplying}
            minRows={2}
            placeholder="Reply, @ to mention"
            size="xs"
            value={draft}
            onChange={setDraft}
          />
          <Group gap={4} justify="flex-end" mt={4}>
            <Button size="compact-xs" variant="subtle" onClick={() => setReplyOpen(false)}>
              Cancel
            </Button>
            <Button disabled={!draft.trim()} loading={isReplying} size="compact-xs" type="submit">
              Send
            </Button>
          </Group>
        </form>
      ) : null}

      <Group gap={2} justify="flex-end" mt={2}>
        <Button size="compact-xs" variant="subtle" onClick={onOpen}>
          Open in email
        </Button>
        {!isResolved && !replyOpen ? (
          <Button size="compact-xs" variant="subtle" onClick={() => setReplyOpen(true)}>
            Reply
          </Button>
        ) : null}
        {!isResolved && canResolve ? (
          <Button loading={isResolving} size="compact-xs" variant="subtle" onClick={onResolve}>
            Resolve
          </Button>
        ) : null}
      </Group>
    </div>
  );
}
