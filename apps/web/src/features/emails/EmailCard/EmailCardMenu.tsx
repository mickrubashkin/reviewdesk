import type { MouseEvent } from "react";
import { ActionIcon, Badge, Group, Menu, ScrollArea, Stack, Text } from "@mantine/core";
import { CheckIcon, DotsThreeVerticalIcon } from "@phosphor-icons/react";

import {
  emailReviewStatusOptions,
  formatEmailReviewStatus,
  isApprovedEmailReviewStatus,
} from "../reviewStatus";
import type { EmailListItem, EmailReviewStatus, EmailVariant } from "../types";

import {
  formatDueDate,
  formatVariantOptionLabel,
  getDueDateTone,
} from "./EmailCard.helpers";
import styles from "./EmailCard.module.css";

type EmailCardMenuProps = {
  availableAdaptations: Array<Pick<EmailListItem, "adaptation_key" | "adaptation_label">>;
  availableVariants: EmailVariant[];
  commentedVersions: EmailListItem[];
  openBlockingCommentCount: number;
  openCommentCount: number;
  selectedAdaptation: string;
  selectedEmail: EmailListItem;
  selectedEmailApprovalBlocked: boolean;
  selectedVariant: EmailVariant;
  variantVersions: EmailListItem[];
  onAdaptationSelect: (
    event: MouseEvent<HTMLButtonElement>,
    adaptationKey: string
  ) => void;
  onCommentVersionClick: (
    event: MouseEvent<HTMLButtonElement>,
    emailId: string
  ) => void;
  onReviewStatusSelect: (
    event: MouseEvent<HTMLButtonElement>,
    reviewStatus: EmailReviewStatus
  ) => void;
  onVariantSelect: (
    event: MouseEvent<HTMLButtonElement>,
    variant: EmailVariant
  ) => void;
  onVersionClick: (event: MouseEvent<HTMLButtonElement>, emailId: string) => void;
};

export function EmailCardMenu({
  availableAdaptations,
  availableVariants,
  commentedVersions,
  openBlockingCommentCount,
  openCommentCount,
  selectedAdaptation,
  selectedEmail,
  selectedEmailApprovalBlocked,
  selectedVariant,
  variantVersions,
  onAdaptationSelect,
  onCommentVersionClick,
  onReviewStatusSelect,
  onVariantSelect,
  onVersionClick,
}: EmailCardMenuProps) {
  const dueDateTone = selectedEmail.due_date
    ? getDueDateTone(selectedEmail.due_date)
    : undefined;

  return (
    <Menu
      classNames={{
        dropdown: styles.cardMenuDropdown,
        divider: styles.cardMenuDivider,
        item: styles.cardMenuItem,
        itemLabel: styles.cardMenuItemLabel,
        label: styles.cardMenuLabel,
      }}
      position="bottom-end"
      shadow="md"
      width={190}
      withinPortal
    >
      <Menu.Target>
        <ActionIcon
          aria-label="Email options"
          className={styles.cardMenuButton}
          size="sm"
          variant="subtle"
          onClick={(event) => event.stopPropagation()}
        >
          <DotsThreeVerticalIcon aria-hidden="true" size={18} weight="bold" />
        </ActionIcon>
      </Menu.Target>
      <Menu.Dropdown onClick={(event) => event.stopPropagation()}>
        <Menu.Sub position="right-start">
          <Menu.Sub.Target>
            <Menu.Sub.Item
              rightSection={
                <Text className={styles.cardMenuValue}>
                  {selectedEmail.language.toUpperCase()}
                </Text>
              }
            >
              Language
            </Menu.Sub.Item>
          </Menu.Sub.Target>
          <Menu.Sub.Dropdown className={styles.cardMenuSubDropdown}>
            {variantVersions.map((email) => (
              <Menu.Item
                key={email.id}
                rightSection={
                  email.id === selectedEmail.id ? (
                    <CheckIcon aria-hidden="true" size={14} weight="bold" />
                  ) : null
                }
                onClick={(event) => onVersionClick(event, email.id)}
              >
                {email.language.toUpperCase()}
              </Menu.Item>
            ))}
          </Menu.Sub.Dropdown>
        </Menu.Sub>

        <Menu.Sub position="right-start">
          <Menu.Sub.Target>
            <Menu.Sub.Item
              rightSection={
                <Text className={styles.cardMenuValue}>
                  {formatVariantOptionLabel(selectedVariant, availableVariants)}
                </Text>
              }
            >
              Version
            </Menu.Sub.Item>
          </Menu.Sub.Target>
          <Menu.Sub.Dropdown className={styles.cardMenuSubDropdown}>
            {availableVariants.map((variant) => (
              <Menu.Item
                key={variant}
                rightSection={
                  variant === selectedVariant ? (
                    <CheckIcon aria-hidden="true" size={14} weight="bold" />
                  ) : null
                }
                onClick={(event) => onVariantSelect(event, variant)}
              >
                {formatVariantOptionLabel(variant, availableVariants)}
              </Menu.Item>
            ))}
          </Menu.Sub.Dropdown>
        </Menu.Sub>

        <Menu.Sub position="right-start">
          <Menu.Sub.Target>
            <Menu.Sub.Item
              rightSection={
                <Text className={styles.cardMenuValue} lineClamp={1}>
                  {selectedEmail.adaptation_label}
                </Text>
              }
            >
              Adaptation
            </Menu.Sub.Item>
          </Menu.Sub.Target>
          <Menu.Sub.Dropdown className={styles.cardMenuSubDropdown}>
            {availableAdaptations.map((adaptation) => (
              <Menu.Item
                key={adaptation.adaptation_key}
                rightSection={
                  adaptation.adaptation_key === selectedAdaptation ? (
                    <CheckIcon aria-hidden="true" size={14} weight="bold" />
                  ) : null
                }
                onClick={(event) =>
                  onAdaptationSelect(event, adaptation.adaptation_key)
                }
              >
                {adaptation.adaptation_label}
              </Menu.Item>
            ))}
          </Menu.Sub.Dropdown>
        </Menu.Sub>

        <Menu.Sub position="right-start">
          <Menu.Sub.Target>
            <Menu.Sub.Item
              rightSection={
                <Text className={styles.cardMenuValue} lineClamp={1}>
                  {formatEmailReviewStatus(selectedEmail.review_status)}
                </Text>
              }
            >
              Status
            </Menu.Sub.Item>
          </Menu.Sub.Target>
          <Menu.Sub.Dropdown className={styles.cardMenuSubDropdown}>
            {emailReviewStatusOptions.map((option) => (
              <Menu.Item
                disabled={
                  isApprovedEmailReviewStatus(option.value) &&
                  selectedEmailApprovalBlocked
                }
                key={option.value}
                rightSection={
                  option.value === selectedEmail.review_status ? (
                    <CheckIcon aria-hidden="true" size={14} weight="bold" />
                  ) : isApprovedEmailReviewStatus(option.value) &&
                    selectedEmailApprovalBlocked ? (
                    <Text c="red" size="xs">
                      Resolve blockers
                    </Text>
                  ) : null
                }
                onClick={(event) => onReviewStatusSelect(event, option.value)}
              >
                {formatEmailReviewStatus(option.value)}
              </Menu.Item>
            ))}
          </Menu.Sub.Dropdown>
        </Menu.Sub>

        <Menu.Divider />

        <Menu.Item
          rightSection={
            <Text className={styles.cardMenuValue} lineClamp={1}>
              {selectedEmail.owner_email ?? "Unassigned"}
            </Text>
          }
        >
          Owner
        </Menu.Item>
        <Menu.Item
          rightSection={
            <Text className={styles.cardMenuValue} lineClamp={1}>
              {selectedEmail.reviewer_email ?? "Unassigned"}
            </Text>
          }
        >
          Reviewer
        </Menu.Item>
        <Menu.Item
          rightSection={
            <Text
              className={styles.cardMenuValue}
              data-tone={dueDateTone}
              lineClamp={1}
            >
              {selectedEmail.due_date
                ? formatDueDate(selectedEmail.due_date)
                : "None"}
            </Text>
          }
        >
          Due date
        </Menu.Item>

        {commentedVersions.length > 0 ? (
          <Menu.Sub position="right-start">
            <Menu.Sub.Target>
              <Menu.Sub.Item
                rightSection={
                  <Badge
                    color={openBlockingCommentCount > 0 ? "red" : "yellow"}
                    size="xs"
                    variant="light"
                  >
                    {openBlockingCommentCount > 0
                      ? `${openBlockingCommentCount} blocking`
                      : openCommentCount}
                  </Badge>
                }
              >
                Open comments
              </Menu.Sub.Item>
            </Menu.Sub.Target>
            <Menu.Sub.Dropdown className={styles.cardMenuCommentsDropdown}>
              <ScrollArea.Autosize mah={156} type="auto">
                <Stack gap={0}>
                  {commentedVersions.map((email) => (
                    <button
                      className={styles.commentVersionRow}
                      key={email.id}
                      type="button"
                      onClick={(event) => onCommentVersionClick(event, email.id)}
                    >
                      <Group justify="space-between" gap={8} wrap="nowrap">
                        <Stack gap={1} className={styles.commentVersionText}>
                          <Text size="xs" fw={700} lineClamp={1}>
                            {email.language.toUpperCase()}{" "}
                            {formatVariantOptionLabel(
                              email.variant,
                              availableVariants
                            )}
                          </Text>
                          <Text
                            className={styles.commentVersionSubject}
                            c="dimmed"
                            lineClamp={1}
                          >
                            {email.subject ?? "No subject"}
                          </Text>
                        </Stack>

                        <Badge
                          className={styles.commentVersionBadge}
                          color={
                            (email.open_blocking_comment_count ?? 0) > 0
                              ? "red"
                              : "yellow"
                          }
                          size="xs"
                          variant="light"
                        >
                          {(email.open_blocking_comment_count ?? 0) > 0
                            ? `${email.open_blocking_comment_count} blocking`
                            : email.open_comment_count}
                        </Badge>
                      </Group>
                    </button>
                  ))}
                </Stack>
              </ScrollArea.Autosize>
            </Menu.Sub.Dropdown>
          </Menu.Sub>
        ) : null}
      </Menu.Dropdown>
    </Menu>
  );
}
