import {
  type KeyboardEvent,
  type MouseEvent,
} from "react";
import {
  Badge,
  Card,
  Group,
  Stack,
  Text,
} from "@mantine/core";
import { useNavigate } from "react-router-dom";

import type {
  EmailReviewStatus,
  EmailVariant,
  EmailVersionGroup,
} from "../types";
import {
  getAvailableVariants,
  getAvailableAdaptations,
  getDefaultVersion,
  getSelectedAdaptation,
  getSelectedVariant,
  getVersionForVariant,
  getVersionsForVariantAndAdaptation,
} from "../stages";
import { isApprovedEmailReviewStatus } from "../reviewStatus";
import { EmailCardMenu } from "./EmailCardMenu";
import { formatEmailTitle } from "./EmailCard.helpers";
import { ProductionBadge } from "../ProductionBadge";
import { OverflowTooltipText } from "./OverflowTooltipText";
import styles from "./EmailCard.module.css";

type EmailCardProps = {
  emailGroup: EmailVersionGroup;
  selectedEmailId: string | undefined;
  onOpen: (groupKey: string, emailId: string) => void;
  onReviewStatusChange: (emailId: string, reviewStatus: EmailReviewStatus) => void;
  onSelectVersion: (groupKey: string, emailId: string) => void;
};

export function EmailCard({
  emailGroup,
  selectedEmailId,
  onOpen,
  onReviewStatusChange,
  onSelectVersion,
}: EmailCardProps) {
  const navigate = useNavigate();
  const selectedEmail =
    emailGroup.versions.find((email) => email.id === selectedEmailId) ??
    getDefaultVersion(emailGroup.versions);
  const selectedVariant = getSelectedVariant(
    emailGroup.versions,
    selectedEmail.id
  );
  const selectedAdaptation = getSelectedAdaptation(
    emailGroup.versions,
    selectedEmail.id
  );
  const variantVersions = getVersionsForVariantAndAdaptation(
    emailGroup.versions,
    selectedVariant,
    selectedAdaptation
  );
  const availableVariants = getAvailableVariants(emailGroup.versions);
  const availableAdaptations = getAvailableAdaptations(
    emailGroup.versions,
    selectedVariant
  );
  const openCommentCount = emailGroup.versions.reduce(
    (count, email) => count + (email.open_comment_count ?? 0),
    0
  );
  const openBlockingCommentCount = emailGroup.versions.reduce(
    (count, email) => count + (email.open_blocking_comment_count ?? 0),
    0
  );
  const selectedEmailApprovalBlocked =
    !isApprovedEmailReviewStatus(selectedEmail.review_status) &&
    (selectedEmail.open_blocking_comment_count ?? 0) > 0;
  const commentedVersions = emailGroup.versions.filter(
    (email) => (email.open_comment_count ?? 0) > 0
  );

  const handleVersionClick = (
    event: MouseEvent<HTMLButtonElement>,
    emailId: string
  ) => {
    event.stopPropagation();
    onSelectVersion(emailGroup.key, emailId);
  };
  const handleCommentVersionClick = (
    event: MouseEvent<HTMLButtonElement>,
    emailId: string
  ) => {
    event.stopPropagation();
    navigate(`/emails/${encodeURIComponent(emailId)}/review`);
  };

  const handleVariantSelect = (
    event: MouseEvent<HTMLButtonElement>,
    variant: EmailVariant
  ) => {
    event.stopPropagation();
    const nextEmail = getVersionForVariant(
      emailGroup.versions,
      variant,
      selectedEmail.language,
      selectedAdaptation
    );
    if (nextEmail) {
      onSelectVersion(emailGroup.key, nextEmail.id);
    }
  };
  const handleAdaptationSelect = (
    event: MouseEvent<HTMLButtonElement>,
    adaptationKey: string
  ) => {
    event.stopPropagation();
    const nextEmail = getVersionForVariant(
      emailGroup.versions,
      selectedVariant,
      selectedEmail.language,
      adaptationKey
    );
    if (nextEmail) {
      onSelectVersion(emailGroup.key, nextEmail.id);
    }
  };
  const handleReviewStatusSelect = (
    event: MouseEvent<HTMLButtonElement>,
    reviewStatus: EmailReviewStatus
  ) => {
    event.stopPropagation();
    if (isApprovedEmailReviewStatus(reviewStatus) && selectedEmailApprovalBlocked) {
      return;
    }
    if (reviewStatus !== selectedEmail.review_status) {
      onReviewStatusChange(selectedEmail.id, reviewStatus);
    }
  };
  const handleCardKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onOpen(emailGroup.key, selectedEmail.id);
    }
  };

  return (
    <Card
      className={`${styles.emailCardButton} ${styles.emailCard}`}
      withBorder
      padding="md"
      radius="md"
      role="button"
      tabIndex={0}
      onClick={() => {
        onOpen(emailGroup.key, selectedEmail.id);
      }}
      onKeyDown={handleCardKeyDown}
    >
      <Stack className={styles.emailCardContent} gap={0}>
        <Stack className={styles.emailCardBody} gap={6}>
          <Group justify="space-between" gap={8} align="flex-start" wrap="nowrap">
            <Text
              className={styles.emailCardTitle}
              fw={600}
              size="sm"
              lineClamp={2}
            >
              {formatEmailTitle(selectedEmail.title)}
            </Text>

            {openCommentCount > 0 ? (
              <Badge
                className={styles.openCommentCornerBadge}
                color={openBlockingCommentCount > 0 ? "red" : "yellow"}
                radius="xl"
                size="xs"
                variant="light"
              >
                {openCommentCount}
              </Badge>
            ) : null}

            <EmailCardMenu
              availableAdaptations={availableAdaptations}
              availableVariants={availableVariants}
              commentedVersions={commentedVersions}
              openBlockingCommentCount={openBlockingCommentCount}
              openCommentCount={openCommentCount}
              selectedAdaptation={selectedAdaptation}
              selectedEmail={selectedEmail}
              selectedEmailApprovalBlocked={selectedEmailApprovalBlocked}
              selectedVariant={selectedVariant}
              variantVersions={variantVersions}
              onAdaptationSelect={handleAdaptationSelect}
              onCommentVersionClick={handleCommentVersionClick}
              onReviewStatusSelect={handleReviewStatusSelect}
              onVariantSelect={handleVariantSelect}
              onVersionClick={handleVersionClick}
            />
          </Group>

          <Stack className={styles.emailCardCopy} gap={10}>
            {selectedEmail.subject ? (
              <OverflowTooltipText
                text={selectedEmail.subject}
                size="sm"
                c="dimmed"
                lineClamp={2}
              />
            ) : null}

            {selectedEmail.preheader ? (
              <OverflowTooltipText
                text={selectedEmail.preheader}
                size="xs"
                c="dimmed"
                lineClamp={2}
              />
            ) : null}
          </Stack>
        </Stack>

        <Group className={styles.emailCardFooter} justify="space-between" wrap="nowrap">
          <Text size="xs" lineClamp={1}>
            {selectedEmail.send_timing ?? "No timing"}
            {selectedEmail.send_condition ? ` · ${selectedEmail.send_condition}` : ""}
          </Text>
          <ProductionBadge email={selectedEmail} versions={emailGroup.versions} />
        </Group>
      </Stack>
    </Card>
  );
}

export default EmailCard;
