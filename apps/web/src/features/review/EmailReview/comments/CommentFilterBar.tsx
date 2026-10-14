import { Badge, Button, Group, SegmentedControl, Stack } from "@mantine/core";
import { ChatTextIcon } from "@phosphor-icons/react";

import type { EmailComment } from "../../../emails/types";
import type { CommentStatusFilter } from "../EmailReview.types";
import styles from "../EmailReview.module.css";

export function CommentFilterBar({
  comments,
  filter,
  onAddCommentClick,
  onFilterChange,
}: {
  comments: EmailComment[];
  filter: CommentStatusFilter;
  onAddCommentClick: () => void;
  onFilterChange: (filter: CommentStatusFilter) => void;
}) {
  const openCount = comments.filter(
    (comment) => comment.status === "open"
  ).length;
  const blockingCount = comments.filter(
    (comment) => comment.status === "open" && comment.severity === "blocking"
  ).length;
  const resolvedCount = comments.filter(
    (comment) => comment.status === "resolved"
  ).length;

  return (
    <Stack className={styles.commentFilterBar} gap="xs">
      <Group gap={6} wrap="nowrap">
        <Badge color="yellow" size="sm" variant="light">
          Open {openCount}
        </Badge>
        {blockingCount > 0 ? (
          <Badge color="red" size="sm" variant="light">
            Blocking {blockingCount}
          </Badge>
        ) : null}
        <Badge color="green" size="sm" variant="light">
          Resolved {resolvedCount}
        </Badge>
        <Badge color="gray" size="sm" variant="light">
          All {comments.length}
        </Badge>
      </Group>

      <Group justify="space-between" gap="xs" wrap="nowrap">
        <Button
          leftSection={<ChatTextIcon aria-hidden="true" size={14} />}
          size="xs"
          variant="light"
          onClick={onAddCommentClick}
        >
          Add comment
        </Button>
        <SegmentedControl
          aria-label="Comment status filter"
          className={styles.commentFilterControl}
          data={[
            { label: "Open", value: "open" },
            { label: "All", value: "all" },
          ]}
          size="xs"
          value={filter}
          onChange={(value) => onFilterChange(value as CommentStatusFilter)}
        />
      </Group>
    </Stack>
  );
}
