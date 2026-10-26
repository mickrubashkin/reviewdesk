import { useEffect } from "react";
import {
  ActionIcon,
  Anchor,
  Button,
  Group,
  Skeleton,
  Text,
  Tooltip,
} from "@mantine/core";
import { Link } from "react-router-dom";

import {
  ArrowLeftIcon,
  ArrowRightIcon,
  ChatTextIcon,
  HouseIcon,
} from "@phosphor-icons/react";

import type { EmailDetail, EmailListItem, EmailVariant } from "../../emails/types";

import { formatEmailTitle } from "./EmailReview.helpers";
import { ProductionBadge } from "../../emails/ProductionBadge";
import { UploadBadge } from "../../emails/UploadBadge";

import styles from "./EmailReview.module.css";
import { ReviewStatusControl } from "./header/ReviewStatusControl";
import { ReviewMoreMenu } from "./header/ReviewMoreMenu";
import { LanguagePills, type LanguagePillVersion } from "./header/LanguagePills";
import { AdaptationSelect, VariantSelect } from "./header/VariantSelectors";

type ReviewHeaderProps = {
  actionMenuOpened: boolean;
  approvalBlockedCount: number;
  approvalBlockedMessage: string;
  archiveEmailIsPending: boolean;
  availableAdaptations: Array<{ adaptation_key: string; adaptation_label: string }>;
  hasRegions: boolean;
  availableVariants: EmailVariant[];
  boardGroupCount: number;
  boardGroupPosition: number | null;
  boardName: string | undefined;
  canManageEmail: boolean;
  currentListEmail: EmailListItem | undefined;
  currentStageKey: string | undefined;
  currentStageTitle: string | undefined;
  groupVersions: EmailListItem[];
  email: EmailDetail | undefined;
  hasAdaptationOptions: boolean;
  hasVariantOptions: boolean;
  isSuperAdmin: boolean;
  renderedHTML: string;
  isCompactReview: boolean;
  languageVersions: LanguagePillVersion[];
  nextBoardEmail: EmailListItem | undefined;
  nextEmailWithOpenComments: EmailListItem | null;
  previousBoardEmail: EmailListItem | undefined;
  reviewStatusIsPending: boolean;
  selectedAdaptation: string;
  selectedVariant: EmailVariant;
  onActionMenuChange: (opened: boolean) => void;
  onAdaptationSelect: (adaptationKey: string) => void;
  onArchiveClick: () => void;
  onDuplicateClick: () => void;
  onNavigateToReview: (emailId: string) => void;
  onReviewStatusChange: (value: string | null) => void;
  onVariantSelect: (variant: EmailVariant) => void;
};

export function ReviewHeader({
  actionMenuOpened,
  approvalBlockedCount,
  approvalBlockedMessage,
  archiveEmailIsPending,
  availableAdaptations,
  hasRegions,
  availableVariants,
  boardGroupCount,
  boardGroupPosition,
  boardName,
  canManageEmail,
  currentListEmail,
  currentStageKey,
  currentStageTitle,
  email,
  groupVersions,
  hasAdaptationOptions,
  hasVariantOptions,
  isSuperAdmin,
  renderedHTML,
  isCompactReview,
  languageVersions,
  nextBoardEmail,
  nextEmailWithOpenComments,
  previousBoardEmail,
  reviewStatusIsPending,
  selectedAdaptation,
  selectedVariant,
  onActionMenuChange,
  onAdaptationSelect,
  onArchiveClick,
  onDuplicateClick,
  onNavigateToReview,
  onReviewStatusChange,
  onVariantSelect,
}: ReviewHeaderProps) {
  const boardPath = email?.sequence
    ? `/boards/${encodeURIComponent(email.sequence)}`
    : "/";
  const goToPrevious = () => {
    if (previousBoardEmail) {
      onNavigateToReview(previousBoardEmail.id);
    }
  };
  const goToNext = () => {
    if (nextBoardEmail) {
      onNavigateToReview(nextBoardEmail.id);
    }
  };

  // ← and → step through the board, except while typing or with a modifier.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (
        event.defaultPrevented ||
        event.altKey ||
        event.ctrlKey ||
        event.metaKey ||
        event.shiftKey ||
        (event.key !== "ArrowLeft" && event.key !== "ArrowRight")
      ) {
        return;
      }
      const target = event.target;
      if (
        target instanceof HTMLElement &&
        (target.isContentEditable ||
          target.closest("input, textarea, select, [role='textbox'], [role='menu'], [role='dialog']"))
      ) {
        return;
      }
      if (event.key === "ArrowLeft") {
        goToPrevious();
      } else {
        goToNext();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  return (
    <header className={styles.header}>
      <div className={styles.headerInner}>
        <Group className={styles.headerMain} gap="sm" wrap="nowrap">
          <Group className={styles.boardNavigation} gap={4} wrap="nowrap">
            <Tooltip label="Back to board">
              <ActionIcon
                aria-label="Back to board"
                className={styles.headerIconButton}
                component={Link}
                radius="md"
                size="lg"
                to={
                  email && currentStageKey
                    ? `${boardPath}?stage=${encodeURIComponent(currentStageKey)}`
                    : boardPath
                }
                variant="subtle"
              >
                <HouseIcon aria-hidden="true" size={17} />
              </ActionIcon>
            </Tooltip>
            <Tooltip
              disabled={!previousBoardEmail}
              label={previousBoardEmail ? formatEmailTitle(previousBoardEmail.title) : ""}
            >
              <ActionIcon
                aria-keyshortcuts="ArrowLeft"
                aria-label="Previous email on board"
                className={styles.headerIconButton}
                disabled={!previousBoardEmail}
                onClick={goToPrevious}
                radius="md"
                size="lg"
                variant="subtle"
              >
                <ArrowLeftIcon aria-hidden="true" size={17} />
              </ActionIcon>
            </Tooltip>
            {boardGroupPosition ? (
              <Text className={styles.boardCounter} c="dimmed" size="xs">
                {boardGroupPosition} / {boardGroupCount}
              </Text>
            ) : null}
            <Tooltip
              disabled={!nextBoardEmail}
              label={nextBoardEmail ? formatEmailTitle(nextBoardEmail.title) : ""}
            >
              <ActionIcon
                aria-keyshortcuts="ArrowRight"
                aria-label="Next email on board"
                className={styles.headerIconButton}
                disabled={!nextBoardEmail}
                onClick={goToNext}
                radius="md"
                size="lg"
                variant="subtle"
              >
                <ArrowRightIcon aria-hidden="true" size={17} />
              </ActionIcon>
            </Tooltip>
            {!isCompactReview ? (
              <Button
                disabled={!nextEmailWithOpenComments}
                leftSection={<ChatTextIcon aria-hidden="true" size={14} />}
                size="compact-xs"
                variant="light"
                onClick={() => {
                  if (nextEmailWithOpenComments) {
                    onNavigateToReview(nextEmailWithOpenComments.id);
                  }
                }}
              >
                Next with open comments
              </Button>
            ) : null}
          </Group>

          <Group className={styles.breadcrumbs} gap={6} wrap="nowrap">
            {email ? (
              <Anchor
                className={styles.breadcrumbText}
                component={Link}
                size="sm"
                to={boardPath}
              >
                {boardName ?? email.sequence}
              </Anchor>
            ) : (
              <Skeleton className={styles.breadcrumbText} h={14} w={70} />
            )}
            <Text c="dimmed" size="sm">
              ›
            </Text>
            {email && currentStageTitle ? (
              <Anchor
                className={styles.breadcrumbText}
                component={Link}
                size="sm"
                to={
                  currentStageKey
                    ? `${boardPath}?stage=${encodeURIComponent(currentStageKey)}`
                    : boardPath
                }
              >
                {currentStageTitle}
              </Anchor>
            ) : (
              <Skeleton className={styles.breadcrumbText} h={14} w={84} />
            )}
            <Text c="dimmed" size="sm">
              ›
            </Text>
            {email ? (
              <Text className={styles.titleText} size="sm" fw={650}>
                {formatEmailTitle(email.title)}
              </Text>
            ) : (
              <Skeleton className={styles.titleText} h={14} w={120} />
            )}
          </Group>

          {!isCompactReview ? (
            <>
              <div className={styles.actionDivider} aria-hidden="true" />

              <Group className={styles.actionGroup} gap="xs" wrap="nowrap">
                <LanguagePills
                  selectedEmailId={email?.id ?? ""}
                  versions={languageVersions}
                  onSelect={onNavigateToReview}
                />
                {hasVariantOptions ? (
                  <VariantSelect
                    availableVariants={availableVariants}
                    selectedVariant={selectedVariant}
                    onSelect={onVariantSelect}
                  />
                ) : null}
                {hasAdaptationOptions ? (
                  <AdaptationSelect
                    adaptations={availableAdaptations}
                    hasRegions={hasRegions}
                    selectedAdaptation={selectedAdaptation}
                    onSelect={onAdaptationSelect}
                  />
                ) : null}
              </Group>
            </>
          ) : null}
        </Group>

        <Group className={styles.headerActions} gap="xs" wrap="nowrap">
          {email ? (
            <Group gap={6} wrap="nowrap">
              <ReviewStatusControl
                approvalBlockedCount={approvalBlockedCount}
                approvalBlockedMessage={approvalBlockedMessage}
                canManage={canManageEmail}
                isUpdating={reviewStatusIsPending}
                status={email.review_status}
                onChange={onReviewStatusChange}
              />
              {!isCompactReview && currentListEmail ? (
                <>
                  <ProductionBadge email={currentListEmail} versions={groupVersions} />
                  <UploadBadge email={currentListEmail} />
                </>
              ) : null}
            </Group>
          ) : null}
          <ReviewMoreMenu
            archiveIsPending={archiveEmailIsPending}
            canManage={canManageEmail}
            currentListEmail={currentListEmail}
            email={email}
            isCompactReview={isCompactReview}
            isSuperAdmin={isSuperAdmin}
            nextEmailWithOpenComments={nextEmailWithOpenComments}
            opened={actionMenuOpened}
            renderedHTML={renderedHTML}
            selectors={
              <>
                <LanguagePills
                  selectedEmailId={email?.id ?? ""}
                  versions={languageVersions}
                  onSelect={onNavigateToReview}
                />
                {hasVariantOptions ? (
                  <VariantSelect
                    availableVariants={availableVariants}
                    selectedVariant={selectedVariant}
                    onSelect={onVariantSelect}
                  />
                ) : null}
                {hasAdaptationOptions ? (
                  <AdaptationSelect
                    adaptations={availableAdaptations}
                    hasRegions={hasRegions}
                    selectedAdaptation={selectedAdaptation}
                    onSelect={onAdaptationSelect}
                  />
                ) : null}
              </>
            }
            onArchiveClick={onArchiveClick}
            onDuplicateClick={onDuplicateClick}
            onNavigateToReview={onNavigateToReview}
            onOpenedChange={onActionMenuChange}
          />
        </Group>
      </div>
    </header>
  );
}
