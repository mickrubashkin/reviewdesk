import { useState, type MouseEvent } from "react";
import {
  ActionIcon,
  Badge,
  Button,
  Group,
  Menu,
  ScrollArea,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import {
  ArchiveIcon,
  ArrowSquareOutIcon,
  CheckIcon,
  CopyIcon,
  CopySimpleIcon,
  DotsThreeVerticalIcon,
  UploadSimpleIcon,
} from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";

import { fetchAdminUsers } from "../../admin-users/api";
import { markEmailUploaded, updateEmailPlanningFields } from "../api";
import { applyPlanningFieldsToCaches } from "../../review/EmailReview/EmailReview.helpers";

import {
  emailReviewStatusOptions,
  formatEmailReviewStatus,
  emailUploadState,
  isApprovedEmailReviewStatus,
} from "../reviewStatus";
import type {
  EmailListItem,
  EmailReviewStatus,
  EmailVariant,
  AuthUser,
} from "../types";
import { canManageBoard } from "../permissions";

import {
  formatDueDate,
  formatVariantOptionLabel,
  getDueDateTone,
} from "./EmailCard.helpers";
import { CardArchiveModal, CardDuplicateModal } from "./CardActionModals";
import styles from "./EmailCard.module.css";
import { useUserName } from "../../../shared/userNames";

type EmailCardMenuProps = {
  availableAdaptations: Array<Pick<EmailListItem, "adaptation_key" | "adaptation_label">>;
  // On a board with regions the adaptations are its regions.
  hasRegions: boolean;
  availableVariants: EmailVariant[];
  commentedVersions: EmailListItem[];
  currentUser: AuthUser;
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
  hasRegions,
  availableVariants,
  commentedVersions,
  currentUser,
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
  const userName = useUserName();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [modal, setModal] = useState<"duplicate" | "archive" | null>(null);
  const canManage = canManageBoard(currentUser, selectedEmail.sequence);
  const isSuperAdmin = currentUser.role === "super_admin";
  const adminUsersQuery = useQuery({
    queryKey: ["admin", "users"],
    queryFn: fetchAdminUsers,
    enabled: canManage,
  });
  const ownerCandidates = (adminUsersQuery.data ?? []).filter(
    (user) => canManageBoard(user, selectedEmail.sequence)
  );
  const planningMutation = useMutation({
    mutationFn: (changes: { owner_email?: string | null; due_date?: string | null }) =>
      updateEmailPlanningFields(selectedEmail.id, {
        owner_email: selectedEmail.owner_email,
        reviewer_email: selectedEmail.reviewer_email,
        due_date: selectedEmail.due_date,
        implementation_notes: selectedEmail.implementation_notes,
        send_timing: selectedEmail.send_timing,
        send_condition: selectedEmail.send_condition,
        adaptation_label: selectedEmail.adaptation_label,
        ...changes,
      }),
    onSuccess: (response) => {
      applyPlanningFieldsToCaches(
        queryClient,
        selectedEmail.id,
        response,
        selectedEmail.sequence
      );
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", selectedEmail.id],
      });
    },
    onError: () =>
      notifications.show({
        color: "red",
        message: "Try again or check that you have admin access.",
        title: "Planning update failed",
      }),
  });
  const uploadState = emailUploadState(selectedEmail);
  const canMarkUploaded =
    uploadState === "to_upload" ||
    (uploadState === "changed_since_upload" &&
      selectedEmail.review_status === "ready");
  const uploadMutation = useMutation({
    mutationFn: (uploaded: boolean) => markEmailUploaded(selectedEmail.id, uploaded),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", selectedEmail.id],
      });
    },
    onError: (error) =>
      notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });
  const copyLink = () => {
    const url = `${window.location.origin}/emails/${encodeURIComponent(selectedEmail.id)}/review`;
    void navigator.clipboard.writeText(url).then(
      () => notifications.show({ color: "green", message: "Link copied.", title: "Copied" }),
      () => notifications.show({ color: "red", message: url, title: "Could not copy" })
    );
  };
  const hasLanguageOptions = variantVersions.length > 1;
  const hasVariantOptions = availableVariants.length > 1;
  const hasAdaptationOptions = availableAdaptations.length > 1;
  const hasShowSection =
    hasLanguageOptions ||
    hasVariantOptions ||
    hasAdaptationOptions ||
    commentedVersions.length > 0;
  const dueDateTone = selectedEmail.due_date
    ? getDueDateTone(selectedEmail.due_date)
    : undefined;

  return (
    <>
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
      width={220}
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
        {hasShowSection ? (
          <>
            <Menu.Label>Show</Menu.Label>
            {hasLanguageOptions ? (
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
            ) : null}
            {hasVariantOptions ? (
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
            ) : null}
            {hasAdaptationOptions ? (
        <Menu.Sub position="right-start">
          <Menu.Sub.Target>
            <Menu.Sub.Item
              rightSection={
                <Text className={styles.cardMenuValue} lineClamp={1}>
                  {availableAdaptations.find((a) => a.adaptation_key === selectedAdaptation)?.adaptation_label ??
                    selectedEmail.adaptation_label}
                </Text>
              }
            >
              {hasRegions ? "Region" : "Adaptation"}
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
            ) : null}
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
          </>
        ) : null}
        <Menu.Label>Properties</Menu.Label>
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

        {canManage ? (
          <Menu.Sub position="right-start">
            <Menu.Sub.Target>
              <Menu.Sub.Item
                rightSection={
                  <Text className={styles.cardMenuValue} lineClamp={1}>
                    {userName(selectedEmail.owner_email, "Unassigned")}
                  </Text>
                }
              >
                Owner
              </Menu.Sub.Item>
            </Menu.Sub.Target>
            <Menu.Sub.Dropdown className={styles.cardMenuSubDropdown}>
              <Menu.Item
                rightSection={
                  !selectedEmail.owner_email ? (
                    <CheckIcon aria-hidden="true" size={14} weight="bold" />
                  ) : null
                }
                onClick={() => planningMutation.mutate({ owner_email: null })}
              >
                Unassigned
              </Menu.Item>
              {ownerCandidates.map((user) => (
                <Menu.Item
                  key={user.email}
                  rightSection={
                    user.email === selectedEmail.owner_email ? (
                      <CheckIcon aria-hidden="true" size={14} weight="bold" />
                    ) : null
                  }
                  onClick={() => planningMutation.mutate({ owner_email: user.email })}
                >
                  {user.name || user.email}
                </Menu.Item>
              ))}
            </Menu.Sub.Dropdown>
          </Menu.Sub>
        ) : (
          <ReadOnlyRow
            label="Owner"
            value={userName(selectedEmail.owner_email, "Unassigned")}
          />
        )}
        {canManage ? (
          <Menu.Sub position="right-start">
            <Menu.Sub.Target>
              <Menu.Sub.Item
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
              </Menu.Sub.Item>
            </Menu.Sub.Target>
            <Menu.Sub.Dropdown className={styles.cardMenuSubDropdown}>
              <Stack gap={6} p={4}>
                <TextInput
                  aria-label="Due date"
                  size="xs"
                  type="date"
                  value={selectedEmail.due_date ?? ""}
                  onChange={(event) =>
                    planningMutation.mutate({
                      due_date: event.currentTarget.value || null,
                    })
                  }
                />
                {selectedEmail.due_date ? (
                  <Button
                    size="compact-xs"
                    variant="subtle"
                    onClick={() => planningMutation.mutate({ due_date: null })}
                  >
                    Clear
                  </Button>
                ) : null}
              </Stack>
            </Menu.Sub.Dropdown>
          </Menu.Sub>
        ) : (
          <ReadOnlyRow
            label="Due date"
            value={
              selectedEmail.due_date ? formatDueDate(selectedEmail.due_date) : "None"
            }
          />
        )}
        <ReadOnlyRow label="Live" value={liveLabel(selectedEmail)} />
        <ReadOnlyRow label="Bitrix24" value={uploadLabel(uploadState)} />

        <Menu.Label>Actions</Menu.Label>
        <Menu.Item
          leftSection={<ArrowSquareOutIcon aria-hidden="true" size={14} />}
          onClick={() => navigate(`/emails/${encodeURIComponent(selectedEmail.id)}/review`)}
        >
          Open
        </Menu.Item>
        <Menu.Item
          leftSection={<CopySimpleIcon aria-hidden="true" size={14} />}
          onClick={copyLink}
        >
          Copy link
        </Menu.Item>
        {canManage ? (
          <>
            <Menu.Item
              leftSection={<CopyIcon aria-hidden="true" size={14} />}
              onClick={() => setModal("duplicate")}
            >
              Duplicate as...
            </Menu.Item>
            <Menu.Item
              color="red"
              leftSection={<ArchiveIcon aria-hidden="true" size={14} />}
              onClick={() => setModal("archive")}
            >
              Archive
            </Menu.Item>
          </>
        ) : null}
        {isSuperAdmin && (canMarkUploaded || selectedEmail.uploaded_version != null) ? (
          <Menu.Item
            disabled={uploadMutation.isPending}
            leftSection={<UploadSimpleIcon aria-hidden="true" size={14} />}
            onClick={() => uploadMutation.mutate(canMarkUploaded)}
          >
            {canMarkUploaded ? "Mark uploaded" : "Clear upload mark"}
          </Menu.Item>
        ) : null}
      </Menu.Dropdown>
    </Menu>
    {modal ? (
      // Modals render in a portal, but React events still bubble to the card.
      <div
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => event.stopPropagation()}
      >
        {modal === "duplicate" ? (
          <CardDuplicateModal
            emailId={selectedEmail.id}
            onClose={() => setModal(null)}
          />
        ) : (
          <CardArchiveModal
            emailId={selectedEmail.id}
            onClose={() => setModal(null)}
          />
        )}
      </div>
    ) : null}
    </>
  );
}

function ReadOnlyRow({ label, value }: { label: string; value: string }) {
  return (
    <div className={styles.cardMenuReadonly}>
      <span>{label}</span>
      <Text className={styles.cardMenuValue} lineClamp={1}>
        {value}
      </Text>
    </div>
  );
}

function liveLabel(email: EmailListItem) {
  switch (email.production_status) {
    case "live":
      return `Live · ${email.portal_send_count}`;
    case "live_manual":
      return "Live (manual)";
    case "not_seen":
      return "Not seen";
    default:
      return "Unknown";
  }
}

function uploadLabel(state: ReturnType<typeof emailUploadState>) {
  switch (state) {
    case "to_upload":
      return "To upload";
    case "changed_since_upload":
      return "Re-upload";
    case "uploaded":
      return "Uploaded";
    default:
      return "Not ready";
  }
}
