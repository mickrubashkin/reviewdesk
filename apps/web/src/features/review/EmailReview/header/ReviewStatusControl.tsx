import { Badge, Select } from "@mantine/core";

import type { EmailReviewStatus } from "../../../emails/types";
import {
  emailReviewStatusColor,
  emailReviewStatusOptions,
  formatEmailReviewStatus,
  isApprovedEmailReviewStatus,
} from "../../../emails/reviewStatus";
import styles from "../EmailReview.module.css";

export function ReviewStatusControl({
  approvalBlockedCount,
  approvalBlockedMessage,
  canManage,
  isUpdating,
  onChange,
  status,
}: {
  approvalBlockedCount: number;
  approvalBlockedMessage: string;
  canManage: boolean;
  isUpdating: boolean;
  onChange: (value: string | null) => void;
  status: EmailReviewStatus;
}) {
  if (!canManage) {
    return (
      <Badge
        color={emailReviewStatusColor(status)}
        radius="sm"
        variant="light"
      >
        {formatEmailReviewStatus(status)}
      </Badge>
    );
  }

  const approvalBlocked =
    approvalBlockedCount > 0 && !isApprovedEmailReviewStatus(status);
  return (
    <Select
      allowDeselect={false}
      aria-label={approvalBlocked ? approvalBlockedMessage : "Review status"}
      className={styles.reviewStatusSelect}
      classNames={{ input: styles.headerSelectInput }}
      data={emailReviewStatusOptions.map((option) => ({
        ...option,
        disabled: isApprovedEmailReviewStatus(option.value) && approvalBlocked,
      }))}
      disabled={isUpdating}
      size="xs"
      value={status}
      onChange={onChange}
    />
  );
}
