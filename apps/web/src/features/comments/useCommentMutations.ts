import { useMutation, useQueryClient } from "@tanstack/react-query";

import { createCommentMessage, createEmailComment, resolveComment } from "../emails/api";
import type { ReviewTextSelection } from "../emails/MailPreview.types";
import type { EmailCommentSeverity } from "../emails/types";
import { mentionIdsForBody } from "./mentions";

// useCommentMutations creates, replies to and resolves comments on one email.
// Who a message mentions is read from its text.
export function useCommentMutations(emailId: string) {
  const queryClient = useQueryClient();
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["email-comments", emailId] });
    void queryClient.invalidateQueries({ queryKey: ["email-activity", emailId] });
    // The open comment count lives on the email lists and on the email itself;
    // its preview and versions stay as they are.
    void queryClient.invalidateQueries({
      queryKey: ["emails"],
      predicate: (query) =>
        query.queryKey.length <= 2 ||
        (query.queryKey[1] === emailId && query.queryKey[2] === "review"),
    });
  };

  const createCommentMutation = useMutation({
    mutationFn: ({
      body,
      severity,
      selection,
    }: {
      body: string;
      severity: EmailCommentSeverity;
      selection: ReviewTextSelection;
    }) =>
      createEmailComment(emailId, {
        review_block: selection.reviewBlock,
        selected_text: selection.selectedText,
        start_offset: selection.startOffset,
        end_offset: selection.endOffset,
        body,
        severity,
        mention_user_ids: mentionIdsForBody(queryClient, body),
      }),
    onSuccess: refresh,
  });

  const resolveCommentMutation = useMutation({
    mutationFn: (commentId: string) => resolveComment(commentId),
    onSuccess: refresh,
  });

  const createReplyMutation = useMutation({
    mutationFn: ({ body, commentId }: { body: string; commentId: string }) =>
      createCommentMessage(commentId, {
        body,
        mention_user_ids: mentionIdsForBody(queryClient, body),
      }),
    onSuccess: refresh,
  });

  return { createCommentMutation, createReplyMutation, resolveCommentMutation };
}
