import {
  Alert,
  Button,
  Loader,
  Stack,
  Text,
  Timeline,
} from "@mantine/core";
import { ArrowRightIcon } from "@phosphor-icons/react";
import {
  type FormEvent,
  type RefCallback,
  useEffect,
  useRef,
  useState,
} from "react";

import type { ReviewTextSelection } from "../../emails/MailPreview.types";
import type {
  EmailComment,
  EmailCommentSeverity,
  EmailListItem,
} from "../../emails/types";

import type { CommentBlockOption, CommentStatusFilter } from "./EmailReview.types";
import { CommentComposer } from "./comments/CommentComposer";
import { CommentFilterBar } from "./comments/CommentFilterBar";
import { CommentItem } from "./comments/CommentItem";

import styles from "./EmailReview.module.css";

export function CommentsPanel({
  activeCommentId,
  blockOptions,
  comments,
  createCommentError,
  filteredComments,
  filter,
  hoveredCommentId,
  isCreatingComment,
  isError,
  isLoading,
  isResolving,
  canResolveComment,
  nextEmailWithOpenComments,
  onCreateComment,
  onFilterChange,
  onHoverComment,
  onNextEmailWithOpenComments,
  onReply,
  onResolve,
  onSelectComment,
  replyingCommentId,
  staleCommentIds,
}: {
  activeCommentId: string | null;
  blockOptions: CommentBlockOption[];
  comments: EmailComment[];
  createCommentError: boolean;
  filteredComments: EmailComment[];
  filter: CommentStatusFilter;
  hoveredCommentId: string | null;
  isCreatingComment: boolean;
  isError: boolean;
  isLoading: boolean;
  isResolving: boolean;
  canResolveComment: (comment: EmailComment) => boolean;
  nextEmailWithOpenComments: EmailListItem | null;
  onCreateComment: (
    selection: ReviewTextSelection,
    body: string,
    severity: EmailCommentSeverity
  ) => void;
  onFilterChange: (filter: CommentStatusFilter) => void;
  onHoverComment: (comment: EmailComment | null) => void;
  onNextEmailWithOpenComments: () => void;
  onReply: (commentId: string, body: string) => void;
  onResolve: (commentId: string) => void;
  onSelectComment: (comment: EmailComment) => void;
  replyingCommentId: string | null;
  staleCommentIds: Set<string>;
}) {
  const commentItemRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const wasCreatingCommentRef = useRef(false);
  const [isComposerOpen, setIsComposerOpen] = useState(false);
  const [selectedBlockValue, setSelectedBlockValue] = useState<string | null>(
    blockOptions[0]?.value ?? null
  );
  const [draftComment, setDraftComment] = useState("");
  const [draftSeverity, setDraftSeverity] =
    useState<EmailCommentSeverity>("issue");
  const selectedBlock =
    blockOptions.find((option) => option.value === selectedBlockValue) ??
    blockOptions[0] ??
    null;
  const trimmedDraftComment = draftComment.trim();

  useEffect(() => {
    if (wasCreatingCommentRef.current && !isCreatingComment && !createCommentError) {
      setDraftComment("");
      setDraftSeverity("issue");
      setIsComposerOpen(false);
    }

    wasCreatingCommentRef.current = isCreatingComment;
  }, [createCommentError, isCreatingComment]);

  useEffect(() => {
    if (!activeCommentId) {
      return;
    }

    const animationFrame = window.requestAnimationFrame(() => {
      commentItemRefs.current.get(activeCommentId)?.scrollIntoView({
        behavior: "smooth",
        block: "nearest",
      });
    });

    return () => window.cancelAnimationFrame(animationFrame);
  }, [activeCommentId, filteredComments]);

  const handlePanelCommentSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedBlock || !trimmedDraftComment) {
      return;
    }

    onCreateComment(
      {
        endOffset: selectedBlock.selectedText.length,
        reviewBlock: selectedBlock.reviewBlock,
        selectedText: selectedBlock.selectedText,
        startOffset: 0,
      },
      trimmedDraftComment,
      draftSeverity
    );
  };

  const registerCommentItem =
    (commentId: string): RefCallback<HTMLDivElement> =>
      (node) => {
        if (node) {
          commentItemRefs.current.set(commentId, node);
          return;
        }

        commentItemRefs.current.delete(commentId);
      };

  if (isLoading) {
    return (
      <Stack className={styles.emptyState} align="center" justify="center">
        <Loader size="sm" />
        <Text c="dimmed" size="sm">
          Loading comments
        </Text>
      </Stack>
    );
  }

  if (isError) {
    return (
      <Alert color="red" title="Failed to load comments">
        Comments are unavailable right now.
      </Alert>
    );
  }

  return (
    <Stack gap="sm">
      {comments.length > 0 ? (
        <CommentFilterBar
          comments={comments}
          filter={filter}
          onAddCommentClick={() => setIsComposerOpen((opened) => !opened)}
          onFilterChange={onFilterChange}
        />
      ) : null}

      {isComposerOpen ? (
        <CommentComposer
          blockOptions={blockOptions}
          createCommentError={createCommentError}
          draftComment={draftComment}
          draftSeverity={draftSeverity}
          isCreatingComment={isCreatingComment}
          onCancel={() => setIsComposerOpen(false)}
          onChangeDraftComment={setDraftComment}
          onChangeDraftSeverity={setDraftSeverity}
          onChangeSelectedBlockValue={setSelectedBlockValue}
          onSubmit={handlePanelCommentSubmit}
          selectedBlock={selectedBlock}
        />
      ) : null}

      {filteredComments.length > 0 ? (
        <Timeline active={filteredComments.length} bulletSize={24} lineWidth={2}>
          {filteredComments.map((comment) => (
            <CommentItem
              canResolve={canResolveComment(comment)}
              isActive={comment.id === activeCommentId}
              isHovered={comment.id === hoveredCommentId}
              comment={comment}
              isReplying={comment.id === replyingCommentId}
              isResolving={isResolving}
              isStaleAfterEdit={staleCommentIds.has(comment.id)}
              itemRef={registerCommentItem(comment.id)}
              key={comment.id}
              onHover={onHoverComment}
              onReply={onReply}
              onResolve={onResolve}
              onSelect={onSelectComment}
            />
          ))}
        </Timeline>
      ) : comments.length > 0 ? (
        <Stack
          className={styles.emptyState}
          align="center"
          justify="center"
          gap="xs"
        >
          <Text fw={600}>No open comments</Text>
          <Text c="dimmed" ta="center" size="sm">
            Resolved comments are hidden. Switch to All to review them.
          </Text>
          {nextEmailWithOpenComments ? (
            <Button
              rightSection={<ArrowRightIcon aria-hidden="true" size={15} />}
              size="xs"
              variant="light"
              onClick={onNextEmailWithOpenComments}
            >
              Next with open comments
            </Button>
          ) : null}
        </Stack>
      ) : !isComposerOpen ? (
        <Stack
          className={styles.emptyState}
          align="center"
          justify="center"
          gap="xs"
        >
          <Text fw={600}>No comments yet</Text>
          <Text c="dimmed" ta="center" size="sm">
            Select text or click a reviewable block in the email preview to add
            the first comment.
          </Text>
          <Button
            disabled={blockOptions.length === 0}
            size="xs"
            variant="light"
            onClick={() => setIsComposerOpen(true)}
          >
            Add comment
          </Button>
        </Stack>
      ) : null}
    </Stack>
  );
}
