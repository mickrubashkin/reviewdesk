import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Stack,
  Text,
  Textarea,
  Tooltip,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { CopyIcon, DownloadSimpleIcon } from "@phosphor-icons/react";

import { copyRenderedHTML, downloadRenderedHTML } from "../../../emails/exportHtml";
import {
  emailReviewStatusColor,
  formatEmailReviewStatus,
  isProductionApprovedEmailReviewStatus,
} from "../../../emails/reviewStatus";
import type {
  EmailActivityItem,
  EmailAreaApproval,
  EmailDetail,
} from "../../../emails/types";
import {
  areaApprovalStatusColor,
  formatAreaApprovalStatus,
  formatCommentDate,
} from "../EmailReview.helpers";
import { MarkLiveButton, UnmarkLiveButton } from "../../../portal/MarkLive";
import styles from "../EmailReview.module.css";

async function copyPlainText(value: string, label: string) {
  try {
    await navigator.clipboard.writeText(value);
    notifications.show({
      color: "green",
      message: label + " copied to clipboard.",
      title: "Copied",
    });
  } catch {
    notifications.show({
      color: "red",
      message: "Browser blocked clipboard access.",
      title: "Copy failed",
    });
  }
}

function HandoffRow({
  copyable = false,
  label,
  value,
}: {
  copyable?: boolean;
  label: string;
  value: string;
}) {
  const displayValue = value.trim() || "Not set";

  return (
    <Group className={styles.handoffRow} gap="xs" justify="space-between" wrap="nowrap">
      <Text c="dimmed" size="sm">
        {label}
      </Text>
      <Group gap={4} justify="flex-end" wrap="nowrap">
        <Text className={styles.handoffValue} fw={600} size="sm">
          {displayValue}
        </Text>
        {copyable && value.trim() ? (
          <Tooltip label={`Copy ${label.toLowerCase()}`}>
            <ActionIcon
              aria-label={`Copy ${label.toLowerCase()}`}
              size="sm"
              variant="subtle"
              onClick={() => void copyPlainText(value, label)}
            >
              <CopyIcon aria-hidden="true" size={14} />
            </ActionIcon>
          </Tooltip>
        ) : null}
      </Group>
    </Group>
  );
}

function formatByteSize(value: string) {
  const bytes = new Blob([value]).size;
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  return `${(bytes / 1024).toFixed(1)} KB`;
}

export function HandoffPanel({
  approvalActivity,
  areaApprovals,
  canMarkLive = false,
  email,
  isLoadingRenderedHTML,
  openBlockingCommentCount,
  openCommentCount,
  renderedHTML,
  renderedHTMLError,
  onLiveChanged,
}: {
  approvalActivity: EmailActivityItem | null;
  areaApprovals: EmailAreaApproval[];
  canMarkLive?: boolean;
  onLiveChanged?: () => void;
  email: EmailDetail | undefined;
  isLoadingRenderedHTML: boolean;
  openBlockingCommentCount: number;
  openCommentCount: number;
  renderedHTML: string;
  renderedHTMLError: boolean;
}) {
  if (!email) {
    return (
      <Stack className={styles.emptyState} align="center" justify="center">
        <Loader size="sm" />
      </Stack>
    );
  }

  const isProductionApproved = isProductionApprovedEmailReviewStatus(
    email.review_status
  );
  const incompleteRequiredApprovals = areaApprovals.filter(
    (approval) => approval.required && approval.status !== "approved"
  );
  const staleApprovals = areaApprovals.filter(
    (approval) => approval.status === "stale"
  );
  const approvalLabel = approvalActivity
    ? `${approvalActivity.actor_email ?? "System"} on ${formatCommentDate(approvalActivity.created_at)}`
    : "Approved";

  return (
    <Stack gap="sm">
      {email.live_marked_at ? (
        <Alert color="green" title="Live on production" variant="light">
          <Group justify="space-between" wrap="nowrap">
            <Text size="sm">
              Marked by {email.live_marked_by} on {formatCommentDate(email.live_marked_at)}
              {email.live_note ? ` · ${email.live_note}` : ""}
            </Text>
            {canMarkLive ? <UnmarkLiveButton emailId={email.id} onDone={() => onLiveChanged?.()} /> : null}
          </Group>
        </Alert>
      ) : canMarkLive ? (
        <Group justify="space-between">
          <Text c="dimmed" size="sm">
            Sent outside CRM robots (e.g. from the admin panel)?
          </Text>
          <MarkLiveButton emailId={email.id} onDone={() => onLiveChanged?.()} />
        </Group>
      ) : null}

      {!isProductionApproved ? (
        <Alert color="gray" title="Handoff not ready" variant="light">
          Mark this email production approved to prepare handoff.
        </Alert>
      ) : null}

      {openBlockingCommentCount > 0 ? (
        <Alert color="red" title="Open blocking comments" variant="light">
          Resolve blocking comments before production handoff.
        </Alert>
      ) : null}

      {incompleteRequiredApprovals.length > 0 ? (
        <Alert color="red" title="Required approvals incomplete" variant="light">
          Complete required area approvals before production handoff.
        </Alert>
      ) : null}

      {staleApprovals.length > 0 ? (
        <Alert color="orange" title="Stale area approvals" variant="light">
          Re-approve {staleApprovals.map((approval) => approval.name).join(", ")} after
          the latest content change.
        </Alert>
      ) : null}

      <Stack className={styles.handoffSection} gap="xs">
        <Group justify="space-between" gap="xs" wrap="nowrap">
          <Text fw={700} size="sm">
            Package
          </Text>
          <Badge
            color={emailReviewStatusColor(email.review_status)}
            radius="sm"
            variant="light"
          >
            {formatEmailReviewStatus(email.review_status)}
          </Badge>
        </Group>
        <HandoffRow label="Title" value={email.title} />
        <HandoffRow label="Subject" value={email.subject ?? ""} copyable />
        <HandoffRow label="Preheader" value={email.preheader ?? ""} copyable />
        <HandoffRow label="Language" value={email.language.toUpperCase()} />
        <HandoffRow label="Version" value={email.variant} />
        <HandoffRow label="Adaptation" value={email.adaptation_label} />
        <HandoffRow label="Send timing" value={email.send_timing ?? ""} />
        <HandoffRow label="Send condition" value={email.send_condition ?? ""} />
      </Stack>

      <Stack className={styles.handoffSection} gap="xs">
        <Text fw={700} size="sm">
          Approval
        </Text>
        <HandoffRow label="Status" value={formatEmailReviewStatus(email.review_status)} />
        {isProductionApproved ? <HandoffRow label="Approved by" value={approvalLabel} /> : null}
        <HandoffRow
          label="Open comments"
          value={`${openCommentCount}${openBlockingCommentCount > 0 ? ` (${openBlockingCommentCount} blocking)` : ""}`}
        />
      </Stack>

      {areaApprovals.length > 0 ? (
        <Stack className={styles.handoffSection} gap="xs">
          <Text fw={700} size="sm">
            Area approvals
          </Text>
          {areaApprovals.map((approval) => (
            <Group
              className={styles.handoffRow}
              gap="xs"
              justify="space-between"
              key={approval.board_approval_area_id}
              wrap="nowrap"
            >
              <Group gap={6} wrap="nowrap">
                <Text c="dimmed" size="sm">
                  {approval.name}
                </Text>
                {approval.required ? (
                  <Badge color="blue" radius="sm" size="xs" variant="light">
                    Required
                  </Badge>
                ) : null}
              </Group>
              <Badge
                color={areaApprovalStatusColor(approval.status)}
                radius="sm"
                size="sm"
                variant="light"
              >
                {formatAreaApprovalStatus(approval.status)}
              </Badge>
            </Group>
          ))}
        </Stack>
      ) : null}

      {email.implementation_notes ? (
        <Stack className={styles.handoffSection} gap={6}>
          <Text fw={700} size="sm">
            Implementation notes
          </Text>
          <Text c="dimmed" size="sm" style={{ whiteSpace: "pre-wrap" }}>
            {email.implementation_notes}
          </Text>
        </Stack>
      ) : null}

      <Stack className={styles.handoffSection} gap="xs">
        <Group justify="space-between" gap="xs" wrap="nowrap">
          <Text fw={700} size="sm">
            Final HTML
          </Text>
          {renderedHTML ? (
            <Text c="dimmed" size="xs">
              {formatByteSize(renderedHTML)}
            </Text>
          ) : null}
        </Group>

        {renderedHTMLError ? (
          <Alert color="red" title="Failed to render HTML" variant="light">
            Try refreshing the page or check the email source.
          </Alert>
        ) : (
          <>
            <Textarea
              autosize
              disabled={!isProductionApproved || isLoadingRenderedHTML}
              maxRows={8}
              minRows={5}
              readOnly
              value={
                isLoadingRenderedHTML
                  ? "Loading rendered HTML..."
                  : renderedHTML
              }
            />
            <Group gap="xs" grow>
              <Button
                disabled={!isProductionApproved || !renderedHTML}
                leftSection={<CopyIcon aria-hidden="true" size={15} />}
                size="xs"
                variant="light"
                onClick={() => void copyRenderedHTML(renderedHTML)}
              >
                Copy HTML
              </Button>
              <Button
                disabled={!isProductionApproved || !renderedHTML}
                leftSection={<DownloadSimpleIcon aria-hidden="true" size={15} />}
                size="xs"
                variant="light"
                onClick={() => downloadRenderedHTML(email, renderedHTML)}
              >
                Download
              </Button>
            </Group>
          </>
        )}
      </Stack>
    </Stack>
  );
}
