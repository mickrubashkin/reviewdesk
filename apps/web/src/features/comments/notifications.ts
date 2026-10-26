import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { fetchJson } from "../../shared/api";

export type AppNotification = {
  id: string;
  kind: "mention";
  created_at: string;
  actor_email: string | null;
  email_id: string;
  email_title: string;
  email_language: string;
  email_variant: string;
  email_stage: string | null;
  board_key: string;
  comment_id: string | null;
  excerpt: string;
};

export const notificationsQueryKey = ["notifications"];

export function fetchNotifications(): Promise<AppNotification[]> {
  return fetchJson<AppNotification[]>("/api/notifications");
}

// markNotificationsRead marks the given notifications read, or all of them
// when ids is empty.
export function markNotificationsRead(ids: string[]): Promise<void> {
  return fetchJson<void>("/api/notifications/read", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  });
}

// useNotifications returns the current person's unread notifications. It
// polls like comments do, so a new mention shows up without a reload.
export function useNotifications() {
  return useQuery({
    queryKey: notificationsQueryKey,
    queryFn: fetchNotifications,
    refetchInterval: 30_000,
  });
}

export function useMarkNotificationsRead() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: markNotificationsRead,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: notificationsQueryKey }),
  });
}

// commentLink opens the email with the mentioned comment in focus.
export function commentLink(emailId: string, commentId: string | null) {
  const path = `/emails/${encodeURIComponent(emailId)}/review`;
  return commentId ? `${path}?comment=${encodeURIComponent(commentId)}` : path;
}
