import {
  ActionIcon,
  Button,
  Group,
  Menu,
  Skeleton,
  Text,
} from "@mantine/core";

import {
  ArchiveIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  ArrowsClockwiseIcon,
  CaretDownIcon,
  ChecksIcon,
  ClockCounterClockwiseIcon,
  EnvelopeSimpleIcon,
  HandshakeIcon,
  HouseIcon,
  ListChecksIcon,
  SparkleIcon,
  StackPlusIcon,
} from "@phosphor-icons/react";

import type { EmailDetail, EmailListItem, EmailVariant } from "../../emails/types";

import type { ReviewUtilityPanel, ReviewViewport } from "./EmailReview.types";
import { formatEmailTitle } from "./EmailReview.helpers";

import styles from "./EmailReview.module.css";
import { ReviewStatusControl } from "./header/ReviewStatusControl";
import {
  AdaptationSelect,
  LanguageSelect,
  ReadOnlyVersionValue,
  VariantSelect,
} from "./header/VariantSelectors";
import { ViewportSwitch } from "./header/ViewportSwitch";

type ReviewHeaderProps = {
  actionMenuOpened: boolean;
  activeUtilityPanel: ReviewUtilityPanel | null;
  approvalBlockedCount: number;
  approvalBlockedMessage: string;
  archiveEmailIsPending: boolean;
  availableAdaptations: Array<{ adaptation_key: string; adaptation_label: string }>;
  availableVariants: EmailVariant[];
  canManageEmail: boolean;
  currentStageTitle: string | undefined;
  email: EmailDetail | undefined;
  hasAdaptationOptions: boolean;
  hasLanguageOptions: boolean;
  hasVariantOptions: boolean;
  isAnalyzingCurrentEmail: boolean;
  isCompactReview: boolean;
  languageVersions: Array<{ id: string; language: string }>;
  nextBoardEmail: EmailListItem | undefined;
  nextEmailWithOpenComments: EmailListItem | null;
  previousBoardEmail: EmailListItem | undefined;
  reviewStatusIsPending: boolean;
  selectedAdaptation: string;
  selectedVariant: EmailVariant;
  viewport: ReviewViewport;
  onActionMenuChange: (opened: boolean) => void;
  onAdaptationSelect: (adaptationKey: string) => void;
  onAnalyze: () => void;
  onArchiveClick: () => void;
  onBackToBoard: () => void;
  onDuplicateClick: () => void;
  onEmailPanelSelect: () => void;
  onNavigateToReview: (emailId: string) => void;
  onReanalyze: () => void;
  onReviewStatusChange: (value: string | null) => void;
  onUtilityPanelSelect: (panel: ReviewUtilityPanel) => void;
  onVariantSelect: (variant: EmailVariant) => void;
  onViewportChange: (viewport: ReviewViewport) => void;
};

export function ReviewHeader({
  actionMenuOpened,
  activeUtilityPanel,
  approvalBlockedCount,
  approvalBlockedMessage,
  archiveEmailIsPending,
  availableAdaptations,
  availableVariants,
  canManageEmail,
  currentStageTitle,
  email,
  hasAdaptationOptions,
  hasLanguageOptions,
  hasVariantOptions,
  isAnalyzingCurrentEmail,
  isCompactReview,
  languageVersions,
  nextBoardEmail,
  nextEmailWithOpenComments,
  previousBoardEmail,
  reviewStatusIsPending,
  selectedAdaptation,
  selectedVariant,
  viewport,
  onActionMenuChange,
  onAdaptationSelect,
  onAnalyze,
  onArchiveClick,
  onBackToBoard,
  onDuplicateClick,
  onEmailPanelSelect,
  onNavigateToReview,
  onReanalyze,
  onReviewStatusChange,
  onUtilityPanelSelect,
  onVariantSelect,
  onViewportChange,
}: ReviewHeaderProps) {
  return (
    <header className={styles.header}>
      <div className={styles.headerInner}>
        <Group className={styles.headerMain} gap="sm" wrap="nowrap">
          <ActionIcon
            aria-label="Back to board"
            className={styles.headerIconButton}
            onClick={onBackToBoard}
            radius="md"
            size="lg"
            variant="subtle"
          >
            <HouseIcon aria-hidden="true" size={18} />
          </ActionIcon>

          <Group className={styles.boardNavigation} gap={4} wrap="nowrap">
            <ActionIcon
              aria-label="Previous email on board"
              className={styles.headerIconButton}
              disabled={!previousBoardEmail}
              onClick={() => {
                if (previousBoardEmail) {
                  onNavigateToReview(previousBoardEmail.id);
                }
              }}
              radius="md"
              size="lg"
              variant="subtle"
            >
              <ArrowLeftIcon aria-hidden="true" size={17} />
            </ActionIcon>
            <ActionIcon
              aria-label="Next email on board"
              className={styles.headerIconButton}
              disabled={!nextBoardEmail}
              onClick={() => {
                if (nextBoardEmail) {
                  onNavigateToReview(nextBoardEmail.id);
                }
              }}
              radius="md"
              size="lg"
              variant="subtle"
            >
              <ArrowRightIcon aria-hidden="true" size={17} />
            </ActionIcon>
          </Group>

          <Group className={styles.breadcrumbs} gap={6} wrap="nowrap">
            {currentStageTitle ? (
              <Text className={styles.breadcrumbText} size="sm" fw={650}>
                {currentStageTitle}
              </Text>
            ) : (
              <Skeleton className={styles.breadcrumbText} h={14} w={84} />
            )}
            <Text c="dimmed" size="sm">
              /
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
                <ViewportSwitch viewport={viewport} onChange={onViewportChange} />
              </Group>

              <div className={styles.actionDivider} aria-hidden="true" />

              <Group className={styles.actionGroup} gap="xs" wrap="nowrap">
                {hasLanguageOptions ? (
                  <LanguageSelect
                    selectedEmailId={email?.id ?? ""}
                    versions={languageVersions}
                    onSelect={onNavigateToReview}
                  />
                ) : (
                  <ReadOnlyVersionValue
                    label="Email language"
                    value={email?.language.toUpperCase() ?? ""}
                  />
                )}

                {hasVariantOptions ? (
                  <VariantSelect
                    availableVariants={availableVariants}
                    selectedVariant={selectedVariant}
                    onSelect={onVariantSelect}
                  />
                ) : (
                  <ReadOnlyVersionValue
                    label="Email version"
                    value={selectedVariant}
                  />
                )}
                {hasAdaptationOptions ? (
                  <AdaptationSelect
                    adaptations={availableAdaptations}
                    selectedAdaptation={selectedAdaptation}
                    onSelect={onAdaptationSelect}
                  />
                ) : (
                  <ReadOnlyVersionValue
                    label="Email adaptation"
                    value={email?.adaptation_label ?? ""}
                  />
                )}
              </Group>
            </>
          ) : null}
        </Group>

        <Group className={styles.headerActions} gap="xs" wrap="nowrap">
          {isCompactReview ? (
            <Menu
              opened={actionMenuOpened}
              position="bottom-end"
              width={260}
              withinPortal
              onChange={onActionMenuChange}
            >
              <Menu.Target>
                <Button
                  aria-label="Open actions menu"
                  className={styles.moreMenuButton}
                  data-expanded={actionMenuOpened || undefined}
                  rightSection={<CaretDownIcon aria-hidden="true" size={12} />}
                  size="xs"
                  variant="subtle"
                >
                  More
                </Button>
              </Menu.Target>
              <Menu.Dropdown>
                {email ? (
                  <>
                    <Menu.Label>Review status</Menu.Label>
                    <div className={styles.menuControls}>
                      <ReviewStatusControl
                        approvalBlockedCount={approvalBlockedCount}
                        approvalBlockedMessage={approvalBlockedMessage}
                        canManage={canManageEmail}
                        isUpdating={reviewStatusIsPending}
                        status={email.review_status}
                        onChange={onReviewStatusChange}
                      />
                    </div>
                    <Menu.Divider />
                  </>
                ) : null}
                <Menu.Label>Email version</Menu.Label>
                <div className={styles.menuControls}>
                  {hasLanguageOptions ? (
                    <LanguageSelect
                      selectedEmailId={email?.id ?? ""}
                      versions={languageVersions}
                      onSelect={onNavigateToReview}
                    />
                  ) : (
                    <ReadOnlyVersionValue
                      label="Email language"
                      value={email?.language.toUpperCase() ?? ""}
                    />
                  )}

                  {hasVariantOptions ? (
                    <VariantSelect
                      availableVariants={availableVariants}
                      selectedVariant={selectedVariant}
                      onSelect={onVariantSelect}
                    />
                  ) : (
                    <ReadOnlyVersionValue
                      label="Email version"
                      value={selectedVariant}
                    />
                  )}
                  {hasAdaptationOptions ? (
                    <AdaptationSelect
                      adaptations={availableAdaptations}
                      selectedAdaptation={selectedAdaptation}
                      onSelect={onAdaptationSelect}
                    />
                  ) : (
                    <ReadOnlyVersionValue
                      label="Email adaptation"
                      value={email?.adaptation_label ?? ""}
                    />
                  )}
                </div>
                <Menu.Divider />

                <Menu.Label>Sections</Menu.Label>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={!activeUtilityPanel || undefined}
                  leftSection={<EnvelopeSimpleIcon aria-hidden="true" size={15} />}
                  onClick={onEmailPanelSelect}
                >
                  Email
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "planning" || undefined}
                  leftSection={<ListChecksIcon aria-hidden="true" size={15} />}
                  onClick={() => onUtilityPanelSelect("planning")}
                >
                  Plan
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "approvals" || undefined}
                  leftSection={<ChecksIcon aria-hidden="true" size={15} />}
                  onClick={() => onUtilityPanelSelect("approvals")}
                >
                  Approvals
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "handoff" || undefined}
                  leftSection={<HandshakeIcon aria-hidden="true" size={15} />}
                  onClick={() => onUtilityPanelSelect("handoff")}
                >
                  Handoff
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "history" || undefined}
                  leftSection={
                    <ClockCounterClockwiseIcon aria-hidden="true" size={15} />
                  }
                  onClick={() => onUtilityPanelSelect("history")}
                >
                  History
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "activity" || undefined}
                  leftSection={
                    <ClockCounterClockwiseIcon aria-hidden="true" size={15} />
                  }
                  onClick={() => onUtilityPanelSelect("activity")}
                >
                  Activity
                </Menu.Item>
                <Menu.Divider />

                {canManageEmail ? (
                  <>
                    <Menu.Item
                      disabled={!email}
                      leftSection={
                        <StackPlusIcon aria-hidden="true" size={15} />
                      }
                      onClick={onDuplicateClick}
                    >
                      Duplicate as...
                    </Menu.Item>
                    <Menu.Item
                      color="red"
                      disabled={!email}
                      leftSection={
                        <ArchiveIcon aria-hidden="true" size={15} />
                      }
                      onClick={() => onArchiveClick()}
                    >
                      {archiveEmailIsPending
                        ? "Archiving"
                        : "Archive email"}
                    </Menu.Item>
                    <Menu.Divider />
                  </>
                ) : null}
                <Menu.Item
                  disabled={!email}
                  leftSection={<SparkleIcon aria-hidden="true" size={15} />}
                  onClick={onAnalyze}
                >
                  {isAnalyzingCurrentEmail ? "Analyzing" : "View shared AI analysis"}
                </Menu.Item>
                <Menu.Item
                  disabled={!email || isAnalyzingCurrentEmail}
                  leftSection={
                    <ArrowsClockwiseIcon aria-hidden="true" size={15} />
                  }
                  onClick={onReanalyze}
                >
                  Generate new shared AI analysis
                </Menu.Item>
                <Menu.Divider />
                <Menu.Item
                  disabled={!nextEmailWithOpenComments}
                  leftSection={<ArrowRightIcon aria-hidden="true" size={15} />}
                  onClick={() => {
                    if (nextEmailWithOpenComments) {
                      onNavigateToReview(nextEmailWithOpenComments.id);
                    }
                  }}
                >
                  Next with open comments
                </Menu.Item>
              </Menu.Dropdown>
            </Menu>
          ) : (
            <Menu
              opened={actionMenuOpened}
              position="bottom-end"
              width={240}
              withinPortal
              onChange={onActionMenuChange}
            >
              <Menu.Target>
                <Button
                  aria-label="Open actions menu"
                  className={styles.moreMenuButton}
                  data-expanded={actionMenuOpened || undefined}
                  rightSection={<CaretDownIcon aria-hidden="true" size={12} />}
                  size="xs"
                  variant="subtle"
                >
                  More
                </Button>
              </Menu.Target>
              <Menu.Dropdown>
                {email ? (
                  <>
                    <Menu.Label>Review status</Menu.Label>
                    <div className={styles.menuControls}>
                      <ReviewStatusControl
                        approvalBlockedCount={approvalBlockedCount}
                        approvalBlockedMessage={approvalBlockedMessage}
                        canManage={canManageEmail}
                        isUpdating={reviewStatusIsPending}
                        status={email.review_status}
                        onChange={onReviewStatusChange}
                      />
                    </div>
                    <Menu.Divider />
                  </>
                ) : null}
                <Menu.Label>Sections</Menu.Label>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={!activeUtilityPanel || undefined}
                  leftSection={<EnvelopeSimpleIcon aria-hidden="true" size={15} />}
                  onClick={onEmailPanelSelect}
                >
                  Email
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "planning" || undefined}
                  leftSection={<ListChecksIcon aria-hidden="true" size={15} />}
                  onClick={() => onUtilityPanelSelect("planning")}
                >
                  Plan
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "approvals" || undefined}
                  leftSection={<ChecksIcon aria-hidden="true" size={15} />}
                  onClick={() => onUtilityPanelSelect("approvals")}
                >
                  Approvals
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "handoff" || undefined}
                  leftSection={<HandshakeIcon aria-hidden="true" size={15} />}
                  onClick={() => onUtilityPanelSelect("handoff")}
                >
                  Handoff
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "history" || undefined}
                  leftSection={
                    <ClockCounterClockwiseIcon aria-hidden="true" size={15} />
                  }
                  onClick={() => onUtilityPanelSelect("history")}
                >
                  History
                </Menu.Item>
                <Menu.Item
                  className={styles.sectionMenuItem}
                  data-active={activeUtilityPanel === "activity" || undefined}
                  leftSection={
                    <ClockCounterClockwiseIcon aria-hidden="true" size={15} />
                  }
                  onClick={() => onUtilityPanelSelect("activity")}
                >
                  Activity
                </Menu.Item>
                <Menu.Divider />
                <Menu.Label>Actions</Menu.Label>
                {canManageEmail ? (
                  <>
                    <Menu.Item
                      disabled={!email}
                      leftSection={
                        <StackPlusIcon aria-hidden="true" size={15} />
                      }
                      onClick={onDuplicateClick}
                    >
                      Duplicate as...
                    </Menu.Item>
                    <Menu.Item
                      color="red"
                      disabled={!email}
                      leftSection={
                        <ArchiveIcon aria-hidden="true" size={15} />
                      }
                      onClick={() => onArchiveClick()}
                    >
                      {archiveEmailIsPending
                        ? "Archiving"
                        : "Archive email"}
                    </Menu.Item>
                    <Menu.Divider />
                  </>
                ) : null}
                <Menu.Item
                  disabled={!email}
                  leftSection={<SparkleIcon aria-hidden="true" size={15} />}
                  onClick={onAnalyze}
                >
                  {isAnalyzingCurrentEmail
                    ? "Analyzing"
                    : "View shared AI analysis"}
                </Menu.Item>
                <Menu.Item
                  disabled={!email || isAnalyzingCurrentEmail}
                  leftSection={
                    <ArrowsClockwiseIcon aria-hidden="true" size={15} />
                  }
                  onClick={onReanalyze}
                >
                  Generate new shared AI analysis
                </Menu.Item>
                <Menu.Divider />
                <Menu.Item
                  disabled={!nextEmailWithOpenComments}
                  leftSection={<ArrowRightIcon aria-hidden="true" size={15} />}
                  onClick={() => {
                    if (nextEmailWithOpenComments) {
                      onNavigateToReview(nextEmailWithOpenComments.id);
                    }
                  }}
                >
                  Next with open comments
                </Menu.Item>
              </Menu.Dropdown>
            </Menu>
          )}
        </Group>
      </div>
    </header>
  );
}
