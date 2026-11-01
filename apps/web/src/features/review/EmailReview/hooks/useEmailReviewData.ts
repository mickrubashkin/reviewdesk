import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo } from "react";

import { fetchAdminUsers } from "../../../admin-users/api";
import { fetchBoards, useBoardRegions } from "../../../boards/api";
import { versionName } from "../../../boards/regions";
import {
  fetchEmailActivity,
  fetchEmailComments,
  fetchEmailDetail,
  fetchEmails,
  fetchEmailVersions,
  fetchRenderedEmail,
  fetchSharedEmailAnalysis,
} from "../../../emails/api";
import {
  buildStageColumns,
  getAvailableAdaptations,
  getAvailableVariants,
  getSelectedAdaptation,
  getSelectedVariant,
  getVersionsForVariantAndAdaptation,
} from "../../../emails/stages";
import type {
  AuthUser,
  EmailDetail,
  EmailListItem,
  EmailVariant,
} from "../../../emails/types";
import type { CommentStatusFilter } from "../EmailReview.types";
import { canManageBoard } from "../../../emails/permissions";
import { getProductionStatus } from "../../../emails/productionStatus";
import { withFirstSendEstimates } from "../../../emails/firstSend";
import {
  buildApprovalBlockedMessage,
  buildChangedBlockTargets,
  buildReviewBlockFreshness,
  commentToTarget,
  findNeighborEmail,
  latestApprovalActivity,
} from "../EmailReview.helpers";

export function useEmailReviewData({
  commentStatusFilter,
  currentUser,
  duplicateModalOpened,
  emailId,
}: {
  currentUser: AuthUser;
  commentStatusFilter: CommentStatusFilter;
  duplicateModalOpened: boolean;
  emailId: string;
}) {
  const queryClient = useQueryClient();

  const emailQuery = useQuery({
    queryKey: ["emails", emailId, "review"],
    queryFn: () => fetchEmailDetail(emailId),
    enabled: emailId.trim() !== "",
  });
  const email = emailQuery.data;
  // Admin rights on this email's board (admins: every board).
  const canManageEmail = canManageBoard(currentUser, email?.sequence);

  const commentsQuery = useQuery({
    queryKey: ["email-comments", emailId],
    queryFn: () => fetchEmailComments(emailId),
    enabled: emailId.trim() !== "",
    refetchInterval: 5000,
    refetchIntervalInBackground: false,
  });

  const activityQuery = useQuery({
    queryKey: ["email-activity", emailId],
    queryFn: () => fetchEmailActivity(emailId),
    enabled: emailId.trim() !== "",
  });

  const renderedEmailQuery = useQuery({
    queryKey: ["emails", emailId, "rendered"],
    queryFn: () => fetchRenderedEmail(emailId),
    enabled: emailId.trim() !== "",
  });

  const versionsQuery = useQuery({
    queryKey: ["emails", emailId, "versions"],
    queryFn: () => fetchEmailVersions(emailId),
    enabled: emailId.trim() !== "" && canManageEmail,
  });

  const emailsQuery = useQuery({
    queryKey: ["emails", email?.sequence ?? "all"],
    queryFn: () => fetchEmails(email?.sequence),
    enabled: Boolean(email?.sequence),
  });

  const sharedAnalysisQuery = useQuery({
    queryKey: ["emails", emailId, "ai-analysis"],
    queryFn: () => fetchSharedEmailAnalysis(emailId),
    enabled: emailId.trim() !== "",
  });

  const adminUsersQuery = useQuery({
    queryKey: ["admin", "users"],
    queryFn: fetchAdminUsers,
    enabled: canManageEmail,
  });

  const boardsQuery = useQuery({
    queryKey: ["boards"],
    queryFn: fetchBoards,
  });

  const duplicateEmailsQuery = useQuery({
    queryKey: ["emails"],
    queryFn: () => fetchEmails(),
    enabled: duplicateModalOpened && canManageEmail,
  });

  // Live state comes from the board's last portal sync, as on the board.
  const portalSyncedAt = boardsQuery.data?.find(
    (board) => board.key === email?.sequence
  )?.portal_synced_at;
  const boardEmails = useMemo(
    () =>
      withFirstSendEstimates(
        (emailsQuery.data ?? []).map((listEmail) => ({
          ...listEmail,
          production_status: getProductionStatus(listEmail, portalSyncedAt),
        }))
      ),
    [emailsQuery.data, portalSyncedAt]
  );
  const stageColumns = useMemo(
    () => buildStageColumns(boardEmails),
    [boardEmails]
  );

  const emailGroup = useMemo(() => {
    return stageColumns
      .flatMap((column) => column.emailGroups)
      .find((group) => group.versions.some((version) => version.id === emailId));
  }, [emailId, stageColumns]);

  const nextEmailWithOpenComments = useMemo(() => {
    const orderedEmails = stageColumns.flatMap(
      (column) => column.emailGroups.flatMap((group) => group.versions)
    );
    const currentEmailIndex = orderedEmails.findIndex(
      (orderedEmail) => orderedEmail.id === emailId
    );

    if (currentEmailIndex === -1) {
      return null;
    }

    return (
      orderedEmails
        .slice(currentEmailIndex + 1)
        .find((orderedEmail) => (orderedEmail.open_comment_count ?? 0) > 0) ??
      null
    );
  }, [emailId, stageColumns]);

  const selectedVariant = emailGroup
    ? getSelectedVariant(emailGroup.versions, emailId)
    : email?.variant ?? "v1";
  const selectedAdaptation = emailGroup
    ? getSelectedAdaptation(emailGroup.versions, emailId)
    : email?.adaptation_key ?? "default";
  const selectedVariantVersions = emailGroup
    ? getVersionsForVariantAndAdaptation(
        emailGroup.versions,
        selectedVariant,
        selectedAdaptation
      )
    : [];
  const languageVersions =
    selectedVariantVersions.length > 0 || !email
      ? selectedVariantVersions
      : [{ id: email.id, language: email.language }];
  const availableVariants = emailGroup
    ? getAvailableVariants(emailGroup.versions)
    : email
      ? [email.variant as EmailVariant]
      : [];
  // On a board with regions the versions are named after them.
  const regions = useBoardRegions(email?.sequence).data ?? [];
  const availableAdaptations = (
    emailGroup
      ? getAvailableAdaptations(emailGroup.versions, selectedVariant)
      : email
        ? [email]
        : []
  ).map((adaptation) => ({
    adaptation_key: adaptation.adaptation_key,
    adaptation_label: versionName(adaptation, email?.language ?? "", regions),
  }));

  const boardEmailGroups = useMemo(
    () => stageColumns.flatMap((column) => column.emailGroups),
    [stageColumns]
  );
  const currentBoardGroupIndex = emailGroup
    ? boardEmailGroups.findIndex((group) => group.key === emailGroup.key)
    : -1;

  const previousBoardEmail = findNeighborEmail(
    [...boardEmailGroups.slice(0, Math.max(currentBoardGroupIndex, 0))].reverse(),
    selectedVariant,
    email?.language,
    selectedAdaptation
  );
  const nextBoardEmail = findNeighborEmail(
    boardEmailGroups.slice(currentBoardGroupIndex + 1),
    selectedVariant,
    email?.language,
    selectedAdaptation
  );
  const currentListEmail = emailGroup?.versions.find(
    (version) => version.id === emailId
  );
  const currentStage = stageColumns.find((column) =>
    column.emailGroups.some((group) => group.key === emailGroup?.key)
  );

  const comments = useMemo(
    () => commentsQuery.data ?? [],
    [commentsQuery.data]
  );
  const openCommentCount = comments.filter(
    (comment) => comment.status === "open"
  ).length;
  const checks = email?.checks ?? [];
  const approvalBlockedCount = checks.length;
  const approvalBlockedMessage = buildApprovalBlockedMessage(checks);

  const filteredComments = useMemo(
    () =>
      commentStatusFilter === "open"
        ? comments.filter((comment) => comment.status === "open")
        : comments,
    [commentStatusFilter, comments]
  );

  const reviewBlockFreshness = useMemo(
    () => buildReviewBlockFreshness(comments, activityQuery.data ?? []),
    [activityQuery.data, comments]
  );
  const commentTargets = useMemo(
    () =>
      filteredComments.map((comment) =>
        commentToTarget(
          comment,
          reviewBlockFreshness.changedAfterCommentCommentIds.has(comment.id)
        )
      ),
    [filteredComments, reviewBlockFreshness.changedAfterCommentCommentIds]
  );
  const changedBlockTargets = useMemo(
    () => buildChangedBlockTargets(reviewBlockFreshness),
    [reviewBlockFreshness]
  );

  const approvalActivity = useMemo(
    () => latestApprovalActivity(activityQuery.data ?? []),
    [activityQuery.data]
  );

  // Sync comment counts to React Query caches
  useEffect(() => {
    if (!commentsQuery.data) {
      return;
    }

    queryClient.setQueryData<EmailListItem[]>(["emails"], (currentEmails) => {
      if (!currentEmails) {
        return currentEmails;
      }

      return currentEmails.map((currentEmail) =>
        currentEmail.id === emailId
          ? {
              ...currentEmail,
              open_comment_count: openCommentCount,
            }
          : currentEmail
      );
    });
    if (email?.sequence) {
      queryClient.setQueryData<EmailListItem[]>(
        ["emails", email.sequence],
        (currentEmails) => {
          if (!currentEmails) {
            return currentEmails;
          }

          return currentEmails.map((currentEmail) =>
            currentEmail.id === emailId
              ? {
                  ...currentEmail,
                  open_comment_count: openCommentCount,
                }
              : currentEmail
          );
        }
      );
    }

    queryClient.setQueryData<EmailDetail>(
      ["emails", emailId, "review"],
      (currentEmail) =>
        currentEmail
          ? {
              ...currentEmail,
              open_comment_count: openCommentCount,
            }
          : currentEmail
    );
  }, [
    commentsQuery.data,
    email?.sequence,
    emailId,
    openCommentCount,
    queryClient,
  ]);

  return {
    canManageEmail,
    emailQuery,
    email,
    commentsQuery,
    comments,
    filteredComments,
    openCommentCount,
    approvalBlockedCount,
    approvalBlockedMessage,
    activityQuery,
    renderedEmailQuery,
    versionsQuery,
    sharedAnalysisQuery,
    adminUsersQuery,
    boardsQuery,
    duplicateEmailsQuery,
    boardEmails,
    stageColumns,
    emailGroup,
    nextEmailWithOpenComments,
    selectedVariant,
    selectedAdaptation,
    languageVersions,
    availableVariants,
    availableAdaptations,
    hasRegions: regions.length > 0,
    hasVariantOptions: availableVariants.length > 1,
    hasAdaptationOptions: availableAdaptations.length > 1,
    previousBoardEmail,
    nextBoardEmail,
    boardGroupCount: boardEmailGroups.length,
    boardGroupPosition:
      currentBoardGroupIndex >= 0 ? currentBoardGroupIndex + 1 : null,
    currentListEmail,
    currentStage,
    commentTargets,
    changedBlockTargets,
    reviewBlockFreshness,
    approvalActivity,
  };
}
