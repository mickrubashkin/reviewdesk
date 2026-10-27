import { useMediaQuery } from "@mantine/hooks";
import { notifications } from "@mantine/notifications";
import { useNavigate } from "react-router-dom";

import { useQueryClient } from "@tanstack/react-query";

import { Alert, Button, Group, Loader, Stack, Text } from "@mantine/core";
import { ArrowsClockwiseIcon, SparkleIcon } from "@phosphor-icons/react";

import { useEffect, useMemo, useRef } from "react";

import { ApiError } from "../../../shared/api";
import { useBlockCatalog } from "../../blocks/api";
import { fieldLimits, layoutItemId } from "../../blocks/layout";
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
  formatStageName,
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
  useLinkedComment,
} from "./hooks";

import {
  ActivityPanel,
  BlocksPanel,
  ChecksPanel,
  HandoffPanel,
  RegionField,
  PlanningPanel,
  ReviewPreviewSkeleton,
  VersionHistoryPanel,
} from "./panels";

import { canEditEmailContent } from "../../emails/permissions";
import { ReviewHeader } from "./ReviewHeader";
import { ArchivedEmailNotice } from "./header/ArchivedEmailNotice";
import { ViewportSwitch } from "./header/ViewportSwitch";
import { ReviewLayout } from "./ReviewLayout";
import { ReferenceSection } from "../../boards/BoardHome/ReferenceSection";
import {
  ArchiveEmailModal,
  DuplicateEmailModal,
  SourceHTMLModal,
} from "./ReviewModals";
import type { EmailReviewProps, ReviewContentTab } from "./EmailReview.types";
import {
  aiBlockTargets,
  buildCommentBlockOptions,
  buildEditableFieldsPayload,
  formatReviewBlockLabel,
} from "./EmailReview.helpers";

export function EmailReview({ currentUser, emailId }: EmailReviewProps) {
  const currentUserEmail = currentUser.email;
  const currentUserRole = currentUser.role;
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const activeCommentTimeoutRef = useRef<number | null>(null);
  const inlineEditPreviewNeedsFrameRef = useRef(false);
  const isCompactReview = useMediaQuery("(max-width: 64em)");

  const layout = useEmailReviewLayout();
  const {
    actionMenuOpened,
    activeCommentId,
    activeContentTab,
    activePanelTab,
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
    commentStatusFilter,
    currentUser,
    duplicateModalOpened,
    emailId,
  });

  const {
    canManageEmail,
    activityQuery,
    adminUsersQuery,
    approvalActivity,
    approvalBlockedCount,
    approvalBlockedMessage,
    availableAdaptations,
    hasRegions,
    availableVariants,
    boardEmails,
    boardGroupCount,
    boardGroupPosition,
    boardsQuery,
    changedBlockTargets,
    comments,
    commentsQuery,
    commentTargets,
    currentListEmail,
    currentStage,
    duplicateEmailsQuery,
    email,
    emailGroup,
    emailQuery,
    filteredComments,
    hasAdaptationOptions,
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
  // Mirrors the API rule: the author or a manager of the board can resolve a comment.
  const canResolveComment = (comment: EmailComment) =>
    canManageEmail ||
    comment.author_email?.toLowerCase() === currentUserEmail.toLowerCase();

  const mutations = useEmailReviewMutations({
    approvalBlockedMessage,
    emailId,
    sequence: email?.sequence,
  });

  const {
    archiveEmailMutation,
    restoreEmailMutation,
    createCommentMutation,
    createReplyMutation,
    duplicateEmailMutation,
    planningFieldsMutation,
    resolveCommentMutation,
    restoreVersionMutation,
    reviewStatusMutation,
    saveEditableFieldsMutation,
    saveLayoutMutation,
  } = mutations;
  const blockCatalogQuery = useBlockCatalog(Boolean(email?.layout));

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
  // While the AI tab of the side panel is open, the blocks its
  // recommendations are about are outlined with their numbers (the preview
  // draws outlines only next to the side panel, not in the compact layout).
  const isAITabOpen = activePanelTab === "ai";
  const previewBlockTargets = useMemo(
    () => [
      ...changedBlockTargets,
      ...(isAITabOpen && displayedAnalysis ? aiBlockTargets(displayedAnalysis) : []),
    ],
    [changedBlockTargets, displayedAnalysis, isAITabOpen]
  );
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

  const handleSelectComment = (
    comment: EmailComment,
    contentTab: ReviewContentTab = "comments"
  ) => {
    setActivePanelTab("comments");
    setActiveContentTab(contentTab);
    setActiveCommentId(comment.id);

    if (activeCommentTimeoutRef.current !== null) {
      window.clearTimeout(activeCommentTimeoutRef.current);
    }
    activeCommentTimeoutRef.current = window.setTimeout(() => {
      setActiveCommentId(null);
      activeCommentTimeoutRef.current = null;
    }, 2400);
  };

  useLinkedComment(commentsQuery.data, handleSelectComment, () =>
    setCommentStatusFilter("all")
  );

  if (emailQuery.isError) {
    return (
      <Stack className={styles.centerState} align="center" justify="center" p="md">
        <Alert color="red" title="Failed to load review">
          The email may not exist, or the API server may be unreachable.
        </Alert>
      </Stack>
    );
  }

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
        title: "Not ready yet",
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

  const handleSelectCommentIds = (commentIds: string[]) => {
    const comment = (commentsQuery.data ?? []).find(
      (commentItem) => commentItem.id === commentIds[0]
    );
    if (comment) {
      handleSelectComment(comment);
    }
  };

  // An archived email is history: read-only until an admin restores it.
  const isArchived = Boolean(email?.archived_at);
  const canEditContent = email && !isArchived
    ? canEditEmailContent(currentUser, email)
    : false;
  const canManageCurrent = canManageEmail && !isArchived;

  const isApplyingInlineEdit =
    saveEditableFieldsMutation.isPending || isInlineEditPreviewRefreshing;

  const previewContent = (
    email ? (
      <MailPreview
        activeCommentId={activeCommentId}
        canEditContent={canEditContent}
        canEditHTML={currentUserRole === "super_admin" && !email.layout}
        changedBlockTargets={previewBlockTargets}
        commentTargets={commentTargets}
        createCommentError={createCommentMutation.isError}
        email={email}
        enableReviewSelectionComposer={!isArchived}
        fieldLimits={fieldLimits(email.layout, blockCatalogQuery.data)}
        inlineEditErrorMessage={saveEditableFieldsMutation.error instanceof ApiError
          ? saveEditableFieldsMutation.error.message
          : undefined}
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
    <Stack gap="sm">
      <Group justify="flex-end">
        <Button
          disabled={!email || isAnalyzingCurrentEmail}
          leftSection={<ArrowsClockwiseIcon aria-hidden="true" size={14} />}
          size="compact-xs"
          variant="subtle"
          onClick={handleReanalyze}
        >
          Generate new analysis
        </Button>
      </Group>
      <AnalysisPanel
        analysis={displayedAnalysis}
        blockLabel={formatReviewBlockLabel}
        error={streamError}
        isStreaming={streamStatus === "streaming"}
        preview={streamPreview}
        showError={streamStatus === "error"}
      />
    </Stack>
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
      // On a phone the comments and the email are separate tabs: a tap on a
      // comment shows its place in the email.
      onSelectComment={(comment) =>
        handleSelectComment(comment, isCompactReview ? "email" : "comments")
      }
      readOnly={isArchived}
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
      canManage={canManageCurrent}
      email={email}
      isLoadingUsers={adminUsersQuery.isLoading}
      isSaving={planningFieldsMutation.isPending}
      key={email?.id ?? "loading"}
      onSubmit={handlePlanningFieldsSubmit}
    />
  );

  const handoffContent = (
    <HandoffPanel
      approvalActivity={approvalActivity}
      canMarkLive={currentUserRole === "super_admin" && !isArchived}
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
      canManage={canManageCurrent}
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

  // An email built from blocks has no HTML of its own to edit: it comes
  // from the block library. Its blocks are changed on the EN email.
  const blocksContent = email?.layout ? (
    <BlocksPanel
      canEdit={canManageCurrent && !email.translation_of && blockCatalogQuery.isSuccess}
      catalog={blockCatalogQuery.data ?? []}
      isSaving={saveLayoutMutation.isPending}
      isTranslation={Boolean(email.translation_of)}
      key={`${email.id}:${email.layout.map(layoutItemId).join(",")}`}
      layout={email.layout}
      onSave={(layout) => saveLayoutMutation.mutate(layout)}
    />
  ) : null;

  const detailsContent = (
    <Stack gap="lg">
      <ChecksPanel checks={email?.checks ?? []} />
      {email ? <RegionField canEdit={canManageCurrent} email={email} key={`region:${email.id}`} /> : null}
      {email ? (
        <ReferenceSection
          candidates={boardEmails}
          canEdit={canEditContent}
          canManage={canManageCurrent}
          email={email}
          key={`reference:${email.id}`}
        />
      ) : null}
      {planningContent}
      {blocksContent}
      {handoffContent}
    </Stack>
  );

  const historyPanelContent = (
    <Stack gap="lg">
      {historyContent}
      {activityContent}
    </Stack>
  );

  return (
    <div className={styles.page}>
      <ReviewHeader
        actionMenuOpened={actionMenuOpened}
        approvalBlockedCount={approvalBlockedCount}
        approvalBlockedMessage={approvalBlockedMessage}
        archiveEmailIsPending={archiveEmailMutation.isPending}
        availableAdaptations={availableAdaptations}
        hasRegions={hasRegions}
        availableVariants={availableVariants}
        canManageEmail={canManageCurrent}
        boardGroupCount={boardGroupCount}
        boardGroupPosition={boardGroupPosition}
        boardName={
          boardsQuery.data?.find((board) => board.key === email?.sequence)?.name
        }
        currentListEmail={currentListEmail}
        currentStageKey={currentStage?.stage ?? (isArchived ? email?.stage ?? undefined : undefined)}
        // The board list holds active emails only, so an archived one names
        // its stage itself.
        currentStageTitle={currentStage?.title ?? (isArchived ? formatStageName(email?.stage ?? "") : undefined)}
        groupVersions={emailGroup?.versions ?? []}
        email={email}
        hasAdaptationOptions={hasAdaptationOptions}
        hasVariantOptions={hasVariantOptions}
        isCompactReview={isCompactReview}
        isSuperAdmin={currentUserRole === "super_admin"}
        languageVersions={languageVersions}
        nextBoardEmail={nextBoardEmail}
        nextEmailWithOpenComments={nextEmailWithOpenComments}
        previousBoardEmail={previousBoardEmail}
        renderedHTML={renderedEmailQuery.data?.html ?? ""}
        reviewStatusIsPending={reviewStatusMutation.isPending}
        selectedAdaptation={selectedAdaptation}
        selectedVariant={selectedVariant}
        onActionMenuChange={setActionMenuOpened}
        onAdaptationSelect={handleAdaptationSelect}
        onArchiveClick={() => setArchiveModalOpened(true)}
        onDuplicateClick={() => {
          duplicateEmailMutation.reset();
          setDuplicateModalOpened(true);
        }}
        onNavigateToReview={navigateToReview}
        onReviewStatusChange={handleReviewStatusChange}
        onVariantSelect={handleVariantClick}
      />

      {email?.archived_at ? (
        <ArchivedEmailNotice
          archivedAt={email.archived_at}
          archivedBy={email.archived_by_email ?? null}
          canRestore={canManageEmail}
          isRestoring={restoreEmailMutation.isPending}
          onRestore={() => restoreEmailMutation.mutate(email.id)}
        />
      ) : null}

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
        analysisContent={analysisContent}
        commentsContent={commentsContent}
        contentRef={contentRef}
        detailsContent={detailsContent}
        historyContent={historyPanelContent}
        isCompactReview={isCompactReview}
        isResizing={isResizing}
        openCommentCount={openCommentCount}
        previewContent={previewContent}
        previewToolbar={<ViewportSwitch viewport={viewport} onChange={setViewport} />}
        rightPanelPercent={rightPanelPercent}
        onActiveContentTabChange={setActiveContentTab}
        onActivePanelTabChange={setActivePanelTab}
        onResizeEnd={layout.handleResizeStart}
        onResizeMove={layout.handleResizeStart}
        onResizeStart={layout.handleResizeStart}
      />
    </div>
  );
}

export default EmailReview;
