import { useMediaQuery } from "@mantine/hooks";
import { notifications } from "@mantine/notifications";
import { useNavigate } from "react-router-dom";

import { useQueryClient } from "@tanstack/react-query";

import { Alert, Button, Loader, Stack, Text } from "@mantine/core";
import { SparkleIcon } from "@phosphor-icons/react";

import { useEffect, useRef } from "react";

import { AnalysisPanel } from "../../emails/AnalysisPanel";

import {
  fetchEmailComments,
  fetchEmailDetail,
} from "../../emails/api";

import { MailPreview } from "../../emails/MailPreview";
import type {
  InlineEditUpdate,
  ReviewTextSelection,
} from "../../emails/MailPreview.types";

import {
  isApprovedEmailReviewStatus,
} from "../../emails/reviewStatus";

import {
  getVersionForVariant,
} from "../../emails/stages";

import { buildStreamPreview } from "../../emails/streamPreview";

import type {
  EmailComment,
  EmailCommentSeverity,
  EmailReviewStatus,
  EmailVariant,
  UpdateEmailPlanningFieldsPayload,
} from "../../emails/types";

import { useEmailAnalysisStream } from "../../emails/useEmailAnalysisStream";

import { CommentsPanel } from "./CommentsPanel";
import styles from "./EmailReview.module.css";

import {
  useEmailReviewData,
  useEmailReviewLayout,
  useEmailReviewMutations,
} from "./hooks";

import {
  ActivityPanel,
  AreaApprovalsPanel,
  HandoffPanel,
  PlanningPanel,
  ReviewPreviewSkeleton,
  VersionHistoryPanel,
} from "./panels";

import { ReviewHeader } from "./ReviewHeader";
import { ReviewLayout } from "./ReviewLayout";
import {
  ArchiveEmailModal,
  DuplicateEmailModal,
  SourceHTMLModal,
} from "./ReviewModals";
import type {
  EmailReviewProps,
  ReviewUtilityPanel,
} from "./EmailReview.types";
import {
  buildCommentBlockOptions,
  buildEditableFieldsPayload,
} from "./EmailReview.helpers";

export function EmailReview({
  currentUserEmail,
  currentUserRole,
  emailId,
}: EmailReviewProps) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const activeCommentTimeoutRef = useRef<number | null>(null);
  const inlineEditPreviewNeedsFrameRef = useRef(false);
  const isCompactReview = useMediaQuery("(max-width: 64em)");
  const canManageEmail =
    currentUserRole === "admin" || currentUserRole === "super_admin";
  // Mirrors the API rule: only the author or an admin can resolve a comment.
  const canResolveComment = (comment: EmailComment) =>
    canManageEmail ||
    comment.author_email?.toLowerCase() === currentUserEmail.toLowerCase();

  const layout = useEmailReviewLayout();
  const {
    actionMenuOpened,
    activeCommentId,
    activeContentTab,
    activePanelTab,
    activeUtilityPanel,
    archiveModalOpened,
    commentStatusFilter,
    contentRef,
    duplicateModalOpened,
    hoveredCommentId,
    isInlineEditPreviewRefreshing,
    isResizing,
    rightPanelPercent,
    setActionMenuOpened,
    setActiveCommentId,
    setActiveContentTab,
    setActivePanelTab,
    setActiveUtilityPanel,
    setArchiveModalOpened,
    setCommentStatusFilter,
    setDuplicateModalOpened,
    setHoveredCommentId,
    setIsInlineEditPreviewRefreshing,
    setSourceHTMLDraft,
    setSourceHTMLModalOpened,
    setViewport,
    sourceHTMLDraft,
    sourceHTMLModalOpened,
    viewport,
  } = layout;

  const data = useEmailReviewData({
    canManageEmail,
    commentStatusFilter,
    duplicateModalOpened,
    emailId,
  });

  const {
    activityQuery,
    adminUsersQuery,
    approvalActivity,
    approvalBlockedCount,
    approvalBlockedMessage,
    areaApprovalsQuery,
    availableAdaptations,
    availableVariants,
    boardsQuery,
    changedBlockTargets,
    comments,
    commentsQuery,
    commentTargets,
    currentStage,
    duplicateEmailsQuery,
    email,
    emailGroup,
    emailQuery,
    filteredComments,
    hasAdaptationOptions,
    hasLanguageOptions,
    hasVariantOptions,
    languageVersions,
    nextBoardEmail,
    nextEmailWithOpenComments,
    openBlockingCommentCount,
    openCommentCount,
    previousBoardEmail,
    renderedEmailQuery,
    reviewBlockFreshness,
    selectedAdaptation,
    selectedVariant,
    sharedAnalysisQuery,
    versionsQuery,
  } = data;

  const mutations = useEmailReviewMutations({
    approvalBlockedMessage,
    emailId,
    sequence: email?.sequence,
  });

  const {
    archiveEmailMutation,
    areaApprovalMutation,
    createCommentMutation,
    createReplyMutation,
    duplicateEmailMutation,
    planningFieldsMutation,
    resolveCommentMutation,
    restoreVersionMutation,
    reviewStatusMutation,
    saveEditableFieldsMutation,
  } = mutations;

  const {
    analyze,
    reanalyze,
    streamAnalysis,
    streamEmailId,
    streamError,
    streamStatus,
    streamText,
  } = useEmailAnalysisStream(email, true, (analysis) => {
    queryClient.setQueryData(["emails", emailId, "ai-analysis"], analysis);
    void queryClient.invalidateQueries({ queryKey: ["email-activity", emailId] });
  });

  const isCurrentStream = streamEmailId === email?.id;
  useEffect(() => {
    if (isCurrentStream && (streamStatus === "done" || streamStatus === "error")) {
      void queryClient.invalidateQueries({ queryKey: ["email-activity", emailId] });
    }
  }, [emailId, isCurrentStream, queryClient, streamStatus]);

  const displayedAnalysis =
    streamAnalysis && isCurrentStream
      ? streamAnalysis
      : sharedAnalysisQuery.data ?? null;
  const streamPreview = buildStreamPreview(streamText);
  const shouldShowAnalysisPanel =
    (isCurrentStream && streamStatus !== "idle") || Boolean(displayedAnalysis);
  const isAnalyzingCurrentEmail = streamStatus === "streaming" && isCurrentStream;

  useEffect(
    () => () => {
      if (activeCommentTimeoutRef.current !== null) {
        window.clearTimeout(activeCommentTimeoutRef.current);
      }
    },
    []
  );

  useEffect(() => {
    for (const neighbor of [previousBoardEmail, nextBoardEmail]) {
      if (!neighbor) {
        continue;
      }

      void queryClient.prefetchQuery({
        queryKey: ["emails", neighbor.id, "review"],
        queryFn: () => fetchEmailDetail(neighbor.id),
      });
      void queryClient.prefetchQuery({
        queryKey: ["email-comments", neighbor.id],
        queryFn: () => fetchEmailComments(neighbor.id),
      });
    }
  }, [nextBoardEmail, previousBoardEmail, queryClient]);

  if (emailQuery.isError) {
    return (
      <Stack className={styles.centerState} align="center" justify="center" p="md">
        <Alert color="red" title="Failed to load review">
          The email may not exist, or the API server may be unreachable.
        </Alert>
      </Stack>
    );
  }

  const handleBackToBoard = () => {
    navigate(email?.sequence ? `/boards/${encodeURIComponent(email.sequence)}` : "/");
  };

  const navigateToReview = (nextEmailId: string) => {
    if (nextEmailId !== email?.id) {
      navigate(`/emails/${encodeURIComponent(nextEmailId)}/review`);
    }
  };

  const handleVariantClick = (variant: EmailVariant) => {
    if (!emailGroup || !email) {
      return;
    }

    const nextEmail = getVersionForVariant(
      emailGroup.versions,
      variant,
      email.language,
      selectedAdaptation
    );
    if (nextEmail) {
      navigateToReview(nextEmail.id);
    }
  };

  const handleAdaptationSelect = (adaptationKey: string) => {
    if (!emailGroup || !email) {
      return;
    }
    const nextEmail = getVersionForVariant(
      emailGroup.versions,
      selectedVariant,
      email.language,
      adaptationKey
    );
    if (nextEmail) {
      navigateToReview(nextEmail.id);
    }
  };

  const handleAnalyze = () => {
    setActivePanelTab("ai");
    setActiveContentTab("ai");
    analyze();
    void queryClient.invalidateQueries({ queryKey: ["email-activity", emailId] });
  };

  const handleReanalyze = () => {
    setActivePanelTab("ai");
    setActiveContentTab("ai");
    reanalyze();
    void queryClient.invalidateQueries({ queryKey: ["email-activity", emailId] });
  };

  const handleReviewStatusChange = (value: string | null) => {
    if (!email || !value || value === email.review_status) {
      return;
    }
    if (isApprovedEmailReviewStatus(value) && approvalBlockedCount > 0) {
      notifications.show({
        color: "red",
        message: approvalBlockedMessage,
        title: "Approval blocked",
      });
      return;
    }

    reviewStatusMutation.mutate({
      nextStatus: value as EmailReviewStatus,
      targetEmailId: email.id,
    });
  };

  const handlePlanningFieldsSubmit = (
    payload: UpdateEmailPlanningFieldsPayload
  ) => {
    if (!email) {
      return;
    }

    planningFieldsMutation.mutate({
      payload,
      targetEmailId: email.id,
    });
  };

  const handleCreateReviewComment = (
    selection: ReviewTextSelection,
    body: string,
    severity: EmailCommentSeverity
  ) => {
    createCommentMutation.mutate({ body, severity, selection });
  };

  const handleApplyInlineEdit = (update: InlineEditUpdate) => {
    if (!email || isInlineEditPreviewRefreshing || saveEditableFieldsMutation.isPending) {
      return;
    }

    inlineEditPreviewNeedsFrameRef.current =
      Object.keys(update.editableFields ?? {}).length > 0;
    saveEditableFieldsMutation.mutate(buildEditableFieldsPayload(email, update));
  };

  const openSourceHTMLModal = () => {
    if (!email || isInlineEditPreviewRefreshing || saveEditableFieldsMutation.isPending) {
      return;
    }

    setSourceHTMLDraft(email.original_html);
    setSourceHTMLModalOpened(true);
  };

  const saveSourceHTML = () => {
    if (!email || isInlineEditPreviewRefreshing || saveEditableFieldsMutation.isPending) {
      return;
    }

    inlineEditPreviewNeedsFrameRef.current = true;
    saveEditableFieldsMutation.mutate(
      buildEditableFieldsPayload(email, {}, sourceHTMLDraft)
    );
  };

  const handleHoverComment = (comment: EmailComment | null) => {
    setHoveredCommentId(comment?.id ?? null);
  };

  const handleHoverCommentIds = (commentIds: string[] | null) => {
    setHoveredCommentId(commentIds?.[0] ?? null);
  };

  const handleSelectComment = (comment: EmailComment) => {
    setActivePanelTab("comments");
    setActiveContentTab("comments");
    setActiveCommentId(comment.id);

    if (activeCommentTimeoutRef.current !== null) {
      window.clearTimeout(activeCommentTimeoutRef.current);
    }
    activeCommentTimeoutRef.current = window.setTimeout(() => {
      setActiveCommentId(null);
      activeCommentTimeoutRef.current = null;
    }, 2400);
  };

  const handleSelectCommentIds = (commentIds: string[]) => {
    const comment = (commentsQuery.data ?? []).find(
      (commentItem) => commentItem.id === commentIds[0]
    );
    if (comment) {
      handleSelectComment(comment);
    }
  };

  const isApplyingInlineEdit =
    saveEditableFieldsMutation.isPending || isInlineEditPreviewRefreshing;

  const previewContent = (
    email ? (
      <MailPreview
        activeCommentId={activeCommentId}
        canEditContent={canManageEmail}
        canEditHTML={currentUserRole === "super_admin"}
        changedBlockTargets={changedBlockTargets}
        commentTargets={commentTargets}
        createCommentError={createCommentMutation.isError}
        email={email}
        enableReviewSelectionComposer
        hoveredCommentId={hoveredCommentId}
        inlineEditError={saveEditableFieldsMutation.isError}
        isApplyingInlineEdit={isApplyingInlineEdit}
        isCreatingComment={createCommentMutation.isPending}
        isScanning={isAnalyzingCurrentEmail}
        onApplyInlineEdit={handleApplyInlineEdit}
        onCommentBadgeClick={handleSelectCommentIds}
        onCommentBadgeHover={handleHoverCommentIds}
        onCreateReviewComment={handleCreateReviewComment}
        onEditSourceHTML={openSourceHTMLModal}
        onInlineEditPreviewReady={() => {
          inlineEditPreviewNeedsFrameRef.current = false;
          if (!saveEditableFieldsMutation.isPending) {
            setIsInlineEditPreviewRefreshing(false);
          }
        }}
        viewport={isCompactReview ? "mobile" : viewport}
      />
    ) : (
      <ReviewPreviewSkeleton />
    )
  );

  const analysisContent = shouldShowAnalysisPanel ? (
    <AnalysisPanel
      analysis={displayedAnalysis}
      error={streamError}
      isStreaming={streamStatus === "streaming"}
      preview={streamPreview}
      showError={streamStatus === "error"}
    />
  ) : (
    <Stack
      className={styles.emptyState}
      align="center"
      justify="center"
      gap="xs"
    >
      {sharedAnalysisQuery.isLoading ? (
        <>
          <Loader size="sm" />
          <Text c="dimmed" size="sm">
            Loading shared AI analysis
          </Text>
        </>
      ) : (
        <>
          <Text fw={600}>No AI analysis yet</Text>
          <Text c="dimmed" ta="center" size="sm">
            Generate a shared review for this email.
          </Text>
          <Button
            disabled={!email}
            leftSection={<SparkleIcon aria-hidden="true" size={15} />}
            loading={isAnalyzingCurrentEmail}
            size="xs"
            variant="light"
            onClick={handleAnalyze}
          >
            Generate AI analysis
          </Button>
        </>
      )}
    </Stack>
  );

  const commentBlockOptions = buildCommentBlockOptions(email);
  const commentsContent = (
    <CommentsPanel
      activeCommentId={activeCommentId}
      blockOptions={commentBlockOptions}
      canResolveComment={canResolveComment}
      comments={comments}
      createCommentError={createCommentMutation.isError}
      filteredComments={filteredComments}
      filter={commentStatusFilter}
      hoveredCommentId={hoveredCommentId}
      staleCommentIds={reviewBlockFreshness.changedAfterCommentCommentIds}
      isCreatingComment={createCommentMutation.isPending}
      isError={commentsQuery.isError}
      isLoading={commentsQuery.isLoading}
      isResolving={resolveCommentMutation.isPending}
      nextEmailWithOpenComments={nextEmailWithOpenComments}
      onCreateComment={handleCreateReviewComment}
      onFilterChange={setCommentStatusFilter}
      onHoverComment={handleHoverComment}
      onNextEmailWithOpenComments={() => {
        if (nextEmailWithOpenComments) {
          navigateToReview(nextEmailWithOpenComments.id);
        }
      }}
      onReply={(commentId, body) =>
        createReplyMutation.mutate({ body, commentId })
      }
      onResolve={(commentId) =>
        resolveCommentMutation.mutate(commentId)
      }
      onSelectComment={handleSelectComment}
      replyingCommentId={
        createReplyMutation.isPending
          ? createReplyMutation.variables?.commentId
          : null
      }
    />
  );

  const planningContent = (
    <PlanningPanel
      adminUsers={adminUsersQuery.data ?? []}
      canManage={canManageEmail}
      email={email}
      isLoadingUsers={adminUsersQuery.isLoading}
      isSaving={planningFieldsMutation.isPending}
      key={email?.id ?? "loading"}
      onSubmit={handlePlanningFieldsSubmit}
    />
  );

  const approvalsContent = (
    <AreaApprovalsPanel
      approvals={areaApprovalsQuery.data ?? []}
      canManage={canManageEmail}
      isError={areaApprovalsQuery.isError}
      isLoading={areaApprovalsQuery.isLoading}
      pendingArea={
        areaApprovalMutation.isPending
          ? areaApprovalMutation.variables?.area ?? null
          : null
      }
      onSubmit={(area, status, decisionNote) =>
        areaApprovalMutation.mutate({ area, decisionNote, status })
      }
    />
  );

  const handoffContent = (
    <HandoffPanel
      approvalActivity={approvalActivity}
      areaApprovals={areaApprovalsQuery.data ?? []}
      canMarkLive={currentUserRole === "super_admin"}
      onLiveChanged={() => void queryClient.invalidateQueries({ queryKey: ["emails", emailId] })}
      email={email}
      isLoadingRenderedHTML={renderedEmailQuery.isLoading}
      openBlockingCommentCount={openBlockingCommentCount}
      openCommentCount={openCommentCount}
      renderedHTML={renderedEmailQuery.data?.html ?? ""}
      renderedHTMLError={renderedEmailQuery.isError}
    />
  );

  const historyContent = (
    <VersionHistoryPanel
      canManage={canManageEmail}
      currentUserRole={currentUserRole}
      isLoading={versionsQuery.isLoading}
      isRestoring={restoreVersionMutation.isPending}
      restoreError={restoreVersionMutation.isError}
      versions={versionsQuery.data ?? []}
      onRestore={(versionId) => restoreVersionMutation.mutate(versionId)}
    />
  );

  const activityContent = (
    <ActivityPanel
      activities={activityQuery.data ?? []}
      isError={activityQuery.isError}
      isLoading={activityQuery.isLoading}
    />
  );

  const selectedUtilityPanel = activeUtilityPanel ?? "planning";
  const utilityContentByPanel: Record<ReviewUtilityPanel, React.ReactNode> = {
    activity: activityContent,
    approvals: approvalsContent,
    handoff: handoffContent,
    history: historyContent,
    planning: planningContent,
  };
  const utilityPanelContent = utilityContentByPanel[selectedUtilityPanel];

  const handleUtilityPanelSelect = (panel: ReviewUtilityPanel) => {
    setActiveUtilityPanel(panel);
    if (isCompactReview) {
      setActiveContentTab("more");
    }
  };

  const handleEmailPanelSelect = () => {
    setActiveUtilityPanel(null);
    setActiveContentTab("email");
  };

  return (
    <div className={styles.page}>
      <ReviewHeader
        actionMenuOpened={actionMenuOpened}
        activeUtilityPanel={activeUtilityPanel}
        approvalBlockedCount={approvalBlockedCount}
        approvalBlockedMessage={approvalBlockedMessage}
        archiveEmailIsPending={archiveEmailMutation.isPending}
        availableAdaptations={availableAdaptations}
        availableVariants={availableVariants}
        canManageEmail={canManageEmail}
        currentStageTitle={currentStage?.title}
        email={email}
        hasAdaptationOptions={hasAdaptationOptions}
        hasLanguageOptions={hasLanguageOptions}
        hasVariantOptions={hasVariantOptions}
        isAnalyzingCurrentEmail={isAnalyzingCurrentEmail}
        isCompactReview={isCompactReview}
        languageVersions={languageVersions}
        nextBoardEmail={nextBoardEmail}
        nextEmailWithOpenComments={nextEmailWithOpenComments}
        previousBoardEmail={previousBoardEmail}
        reviewStatusIsPending={reviewStatusMutation.isPending}
        selectedAdaptation={selectedAdaptation}
        selectedVariant={selectedVariant}
        viewport={viewport}
        onActionMenuChange={setActionMenuOpened}
        onAdaptationSelect={handleAdaptationSelect}
        onAnalyze={handleAnalyze}
        onArchiveClick={() => setArchiveModalOpened(true)}
        onBackToBoard={handleBackToBoard}
        onDuplicateClick={() => {
          duplicateEmailMutation.reset();
          setDuplicateModalOpened(true);
        }}
        onEmailPanelSelect={handleEmailPanelSelect}
        onNavigateToReview={navigateToReview}
        onReanalyze={handleReanalyze}
        onReviewStatusChange={handleReviewStatusChange}
        onUtilityPanelSelect={handleUtilityPanelSelect}
        onVariantSelect={handleVariantClick}
        onViewportChange={setViewport}
      />

      {duplicateModalOpened && email ? (
        <DuplicateEmailModal
          boards={boardsQuery.data ?? []}
          boardsLoading={boardsQuery.isLoading}
          email={email}
          emails={duplicateEmailsQuery.data ?? []}
          emailsLoading={duplicateEmailsQuery.isLoading}
          error={duplicateEmailMutation.error}
          isSubmitting={duplicateEmailMutation.isPending}
          onClose={() => {
            duplicateEmailMutation.reset();
            setDuplicateModalOpened(false);
          }}
          onResetError={() => duplicateEmailMutation.reset()}
          onSubmit={async (sourceEmailId, payload) => {
            await duplicateEmailMutation.mutateAsync({ sourceEmailId, payload });
            setDuplicateModalOpened(false);
          }}
        />
      ) : null}

      {email ? (
        <ArchiveEmailModal
          email={email}
          isSubmitting={archiveEmailMutation.isPending}
          opened={archiveModalOpened}
          onClose={() => setArchiveModalOpened(false)}
          onSubmit={async (archiveEmailId) => {
            await archiveEmailMutation.mutateAsync(archiveEmailId);
            setArchiveModalOpened(false);
          }}
        />
      ) : null}

      {email && currentUserRole === "super_admin" ? (
        <SourceHTMLModal
          draft={sourceHTMLDraft}
          isSubmitting={isApplyingInlineEdit}
          opened={sourceHTMLModalOpened}
          onChange={setSourceHTMLDraft}
          onClose={() => setSourceHTMLModalOpened(false)}
          onSubmit={saveSourceHTML}
        />
      ) : null}

      <ReviewLayout
        activeContentTab={activeContentTab}
        activePanelTab={activePanelTab}
        activeUtilityPanel={activeUtilityPanel}
        analysisContent={analysisContent}
        commentsContent={commentsContent}
        contentRef={contentRef}
        isCompactReview={isCompactReview}
        isResizing={isResizing}
        openCommentCount={openCommentCount}
        previewContent={previewContent}
        rightPanelPercent={rightPanelPercent}
        selectedUtilityPanel={selectedUtilityPanel}
        utilityContentByPanel={utilityContentByPanel}
        utilityPanelContent={utilityPanelContent}
        onActiveContentTabChange={setActiveContentTab}
        onActivePanelTabChange={setActivePanelTab}
        onResizeEnd={layout.handleResizeStart}
        onResizeMove={layout.handleResizeStart}
        onResizeStart={layout.handleResizeStart}
        onUtilityPanelChange={setActiveUtilityPanel}
        onUtilityPanelSelect={handleUtilityPanelSelect}
      />
    </div>
  );
}

export default EmailReview;
