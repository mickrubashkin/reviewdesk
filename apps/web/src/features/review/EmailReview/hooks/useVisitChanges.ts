import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo } from "react";

import { recordEmailVisit } from "../../../emails/api";
import type { AuthUser, EmailActivityItem, EmailListItem } from "../../../emails/types";
import { changesSinceVisit, visitSince } from "../../../emails/visit";

// useVisitChanges records this visit to the email, except while a CRM
// operator views as someone (read-only), and sums up what others changed in
// it since the visit before, or since marks started for a first visit.
export function useVisitChanges({
  activities,
  currentUser,
  emailId,
  listEmail,
}: {
  activities: EmailActivityItem[];
  currentUser: AuthUser;
  emailId: string;
  listEmail: EmailListItem | undefined;
}) {
  const queryClient = useQueryClient();
  const viewing = Boolean(currentUser.viewed_by);
  const board = listEmail?.sequence;

  useEffect(() => {
    if (viewing || !board) {
      return;
    }
    recordEmailVisit(emailId)
      .then(() => {
        // The board and My tasks drop the mark on this email.
        void queryClient.invalidateQueries({ queryKey: ["emails", board], exact: true });
        void queryClient.invalidateQueries({ queryKey: ["emails", "all-boards"], exact: true });
      })
      .catch(() => {
        // Not recorded: the next visit compares with an older one, which is harmless.
      });
  }, [board, emailId, queryClient, viewing]);

  const since =
    visitSince(listEmail?.my_viewed_at, listEmail?.my_previous_viewed_at) ?? currentUser.changes_tracked_since ?? null;
  return useMemo(
    () => changesSinceVisit(activities, since, currentUser.email),
    [activities, currentUser.email, since]
  );
}
