import { Badge, Menu } from "@mantine/core";
import { CaretDownIcon, CheckIcon } from "@phosphor-icons/react";

import type { EmailReviewStatus } from "../../../emails/types";
import {
  emailReviewStatusColor,
  emailReviewStatusOptions,
  formatEmailReviewStatus,
  isApprovedEmailReviewStatus,
} from "../../../emails/reviewStatus";

// ReviewStatusControl is the status badge of the header; for admins it opens
// a menu to switch between Draft and Ready.
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
  const color = emailReviewStatusColor(status);
  if (!canManage) {
    return (
      <Badge color={color} radius="sm" size="lg" variant="light">
        {formatEmailReviewStatus(status)}
      </Badge>
    );
  }

  const approvalBlocked =
    approvalBlockedCount > 0 && !isApprovedEmailReviewStatus(status);
  return (
    <Menu disabled={isUpdating} position="bottom-start" withinPortal>
      <Menu.Target>
        <Badge
          aria-label={approvalBlocked ? approvalBlockedMessage : "Change status"}
          color={color}
          component="button"
          radius="sm"
          rightSection={<CaretDownIcon aria-hidden="true" size={11} />}
          size="lg"
          style={{ cursor: "pointer", opacity: isUpdating ? 0.6 : 1 }}
          type="button"
          variant="light"
        >
          {formatEmailReviewStatus(status)}
        </Badge>
      </Menu.Target>
      <Menu.Dropdown>
        {emailReviewStatusOptions.map((option) => {
          const blocked =
            isApprovedEmailReviewStatus(option.value) && approvalBlocked;
          return (
            <Menu.Item
              disabled={blocked}
              key={option.value}
              title={blocked ? approvalBlockedMessage : undefined}
              rightSection={
                option.value === status ? (
                  <CheckIcon aria-hidden="true" size={13} weight="bold" />
                ) : null
              }
              onClick={() => onChange(option.value)}
            >
              {option.label}
              {blocked ? " (blocked)" : ""}
            </Menu.Item>
          );
        })}
      </Menu.Dropdown>
    </Menu>
  );
}
