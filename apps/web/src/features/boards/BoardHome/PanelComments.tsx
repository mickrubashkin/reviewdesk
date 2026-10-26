import { Button, Group, Loader, Stack, Text } from "@mantine/core";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";

import { commentLink } from "../../comments/notifications";
import { useCommentMutations } from "../../comments/useCommentMutations";
import { fetchEmailComments } from "../../emails/api";
import type { EmailCommentSeverity, EmailDetail, EmailListItem } from "../../emails/types";
import { CommentComposer } from "../../review/EmailReview/comments/CommentComposer";
import { buildCommentBlockOptions } from "../../review/EmailReview/EmailReview.helpers";
import { PanelThread } from "./PanelThread";

// PanelComments is the Comments tab of the board side panel. It lets people read, reply to, resolve and add comments from the
// board's side panel. A new comment goes on a whole review block; for a
// comment on exact words, open the email and select them in the preview.
export function PanelComments({
  canManage,
  currentUserEmail,
  detail,
  email,
}: {
  canManage: boolean;
  currentUserEmail: string;
  detail: EmailDetail | undefined;
  email: EmailListItem;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const commentsQuery = useQuery({
    queryKey: ["email-comments", email.id],
    queryFn: () => fetchEmailComments(email.id),
    refetchInterval: 5000,
    refetchIntervalInBackground: false,
  });
  const { createCommentMutation, createReplyMutation, resolveCommentMutation } = useCommentMutations(email.id);
  const [showResolved, setShowResolved] = useState(false);
  const [composerOpen, setComposerOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const [severity, setSeverity] = useState<EmailCommentSeverity>("issue");
  const [blockValue, setBlockValue] = useState<string | null>(null);

  const blockOptions = buildCommentBlockOptions(detail);
  const selectedBlock = blockOptions.find((option) => option.value === blockValue) ?? blockOptions[0] ?? null;
  const comments = commentsQuery.data ?? [];
  const resolvedCount = comments.filter((comment) => comment.status !== "open").length;
  const visible = showResolved ? comments : comments.filter((comment) => comment.status === "open");

  // The board shows open comment counts, so refresh it when they change.
  const refreshBoard = () => void queryClient.invalidateQueries({ queryKey: ["emails", email.sequence], exact: true });

  const handleCreate = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedBlock || !draft.trim()) {
      return;
    }
    createCommentMutation.mutate(
      {
        body: draft.trim(),
        severity,
        selection: {
          endOffset: selectedBlock.selectedText.length,
          reviewBlock: selectedBlock.reviewBlock,
          selectedText: selectedBlock.selectedText,
          startOffset: 0,
        },
      },
      {
        onSuccess: () => {
          setDraft("");
          setSeverity("issue");
          setComposerOpen(false);
          refreshBoard();
        },
      }
    );
  };

  return (
    <Stack gap="xs">
      <Group justify="space-between" wrap="nowrap">
        <Group gap={6}>{commentsQuery.isLoading ? <Loader size="xs" /> : null}</Group>
        <Group gap={4}>
          {resolvedCount > 0 ? (
            <Button size="compact-xs" variant="subtle" onClick={() => setShowResolved((value) => !value)}>
              {showResolved ? "Hide resolved" : `Show resolved (${resolvedCount})`}
            </Button>
          ) : null}
          {!composerOpen ? (
            <Button
              disabled={blockOptions.length === 0}
              size="compact-xs"
              variant="light"
              onClick={() => setComposerOpen(true)}
            >
              Add comment
            </Button>
          ) : null}
        </Group>
      </Group>

      {commentsQuery.isError ? (
        <Text c="red" size="xs">
          Failed to load comments.
        </Text>
      ) : null}

      {composerOpen ? (
        <CommentComposer
          blockOptions={blockOptions}
          createCommentError={createCommentMutation.isError}
          draftComment={draft}
          draftSeverity={severity}
          isCreatingComment={createCommentMutation.isPending}
          selectedBlock={selectedBlock}
          onCancel={() => setComposerOpen(false)}
          onChangeDraftComment={setDraft}
          onChangeDraftSeverity={setSeverity}
          onChangeSelectedBlockValue={setBlockValue}
          onSubmit={handleCreate}
        />
      ) : null}

      {visible.length > 0 ? (
        <Stack gap="xs">
          {visible.map((comment) => (
            <PanelThread
              key={comment.id}
              canResolve={canManage || comment.author_email?.toLowerCase() === currentUserEmail.toLowerCase()}
              comment={comment}
              isReplying={createReplyMutation.isPending && createReplyMutation.variables?.commentId === comment.id}
              isResolving={resolveCommentMutation.isPending && resolveCommentMutation.variables === comment.id}
              onOpen={() => navigate(commentLink(email.id, comment.id))}
              onReply={(body) => createReplyMutation.mutate({ body, commentId: comment.id })}
              onResolve={() => resolveCommentMutation.mutate(comment.id, { onSuccess: refreshBoard })}
            />
          ))}
        </Stack>
      ) : !commentsQuery.isLoading && !composerOpen ? (
        <Text c="dimmed" size="xs">
          {comments.length > 0 ? "No open comments." : "No comments yet."} Write @ and a name in a comment to notify
          someone.
        </Text>
      ) : null}
    </Stack>
  );
}
