import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo } from "react";

import { fetchAdminUsers } from "../../../admin-users/api";
import { fetchBoards } from "../../../boards/api";
import {
  fetchEmailActivity,
  fetchEmailComments,
  fetchEmailDetail,
  fetchEmails,
  fetchEmailVersions,
  fetchRenderedEmail,
  fetchSharedEmailAnalysis,
  listEmailAreaApprovals,
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
  EmailDetail,
  EmailListItem,
  EmailVariant,
} from "../../../emails/types";
import type { CommentStatusFilter } from "../EmailReview.types";
import {
  buildApprovalBlockedMessage,
  buildChangedBlockTargets,
  buildReviewBlockFreshness,
  commentToTarget,
  findNeighborEmail,
  latestApprovalActivity,
} from "../EmailReview.helpers";

export function useEmailReviewData({
  canManageEmail,
  commentStatusFilter,
  duplicateModalOpened,
  emailId,
}: {
  canManageEmail: boolean;
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

  const areaApprovalsQuery = useQuery({
    queryKey: ["email-area-approvals", emailId],
    queryFn: () => listEmailAreaApprovals(emailId),
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
    enabled: duplicateModalOpened && canManageEmail,
  });

  const duplicateEmailsQuery = useQuery({
    queryKey: ["emails"],
    queryFn: () => fetchEmails(),
    enabled: duplicateModalOpened && canManageEmail,
  });

  const stageColumns = useMemo(
    () => buildStageColumns(emailsQuery.data ?? []),
    [emailsQuery.data]
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
  const availableAdaptations = emailGroup
    ? getAvailableAdaptations(emailGroup.versions, selectedVariant)
    : email
      ? [email]
      : [];

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
  const openBlockingCommentCount = comments.filter(
    (comment) => comment.status === "open" && comment.severity === "blocking"
  ).length;
  const effectiveOpenBlockingCommentCount = Math.max(
    openBlockingCommentCount,
    email?.open_blocking_comment_count ?? 0
  );
  const incompleteRequiredApprovalCount = (areaApprovalsQuery.data ?? []).filter(
    (approval) => approval.required && approval.status !== "approved"
  ).length;
  const approvalBlockedCount =
    effectiveOpenBlockingCommentCount + incompleteRequiredApprovalCount;
  const approvalBlockedMessage = buildApprovalBlockedMessage(
    effectiveOpenBlockingCommentCount,
    incompleteRequiredApprovalCount
  );

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
              open_blocking_comment_count: openBlockingCommentCount,
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
                  open_blocking_comment_count: openBlockingCommentCount,
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
              open_blocking_comment_count: openBlockingCommentCount,
            }
          : currentEmail
    );
  }, [
    commentsQuery.data,
    email?.sequence,
    emailId,
    openBlockingCommentCount,
    openCommentCount,
    queryClient,
  ]);

  return {
    emailQuery,
    email,
    commentsQuery,
    comments,
    filteredComments,
    openCommentCount,
    openBlockingCommentCount,
    approvalBlockedCount,
    approvalBlockedMessage,
    activityQuery,
    areaApprovalsQuery,
    renderedEmailQuery,
    versionsQuery,
    sharedAnalysisQuery,
    adminUsersQuery,
    boardsQuery,
    duplicateEmailsQuery,
    stageColumns,
    emailGroup,
    nextEmailWithOpenComments,
    selectedVariant,
    selectedAdaptation,
    languageVersions,
    availableVariants,
    availableAdaptations,
    hasLanguageOptions: languageVersions.length > 1,
    hasVariantOptions: availableVariants.length > 1,
    hasAdaptationOptions: availableAdaptations.length > 1,
    previousBoardEmail,
    nextBoardEmail,
    currentStage,
    commentTargets,
    changedBlockTargets,
    reviewBlockFreshness,
    approvalActivity,
  };
}
