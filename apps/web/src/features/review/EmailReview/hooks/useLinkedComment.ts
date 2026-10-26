import { useEffect, useRef } from "react";
import { useSearchParams } from "react-router-dom";

import { useMarkNotificationsRead, useNotifications } from "../../../comments/notifications";
import type { EmailComment } from "../../../emails/types";

// useLinkedComment handles links like /emails/{id}/review?comment={id} (from
// My tasks or a mention email): once comments load it focuses that thread,
// and marks the person's notifications about it read.
export function useLinkedComment(
  comments: EmailComment[] | undefined,
  onFocus: (comment: EmailComment) => void,
  onShowResolved: () => void
) {
  const [searchParams] = useSearchParams();
  const commentId = searchParams.get("comment");
  const focusedRef = useRef<string | null>(null);
  const notificationsQuery = useNotifications();
  const markRead = useMarkNotificationsRead();

  useEffect(() => {
    if (!commentId || focusedRef.current === commentId) {
      return;
    }
    const comment = comments?.find((item) => item.id === commentId);
    if (!comment) {
      return;
    }
    focusedRef.current = commentId;
    if (comment.status !== "open") {
      onShowResolved();
    }
    onFocus(comment);
  }, [commentId, comments, onFocus, onShowResolved]);

  const unreadIds = (notificationsQuery.data ?? [])
    .filter((notification) => commentId && notification.comment_id === commentId)
    .map((notification) => notification.id);
  const unreadKey = unreadIds.join(",");
  const { isPending, mutate } = markRead;
  useEffect(() => {
    if (unreadKey && !isPending) {
      mutate(unreadKey.split(","));
    }
  }, [isPending, mutate, unreadKey]);
}
