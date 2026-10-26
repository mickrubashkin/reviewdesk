import { Button, Menu } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useState, type ReactNode } from "react";

import {
  ArchiveIcon,
  ArrowRightIcon,
  CaretDownIcon,
  CopyIcon,
  CopySimpleIcon,
  DownloadSimpleIcon,
  UploadSimpleIcon,
} from "@phosphor-icons/react";

import { BitrixCopyModal } from "../../../emails/BitrixCopyModal";
import { downloadRenderedHTML } from "../../../emails/exportHtml";
import type { EmailDetail, EmailListItem } from "../../../emails/types";

import styles from "../EmailReview.module.css";

type ReviewMoreMenuProps = {
  archiveIsPending: boolean;
  canManage: boolean;
  currentListEmail: EmailListItem | undefined;
  email: EmailDetail | undefined;
  isCompactReview: boolean;
  isSuperAdmin: boolean;
  nextEmailWithOpenComments: EmailListItem | null;
  opened: boolean;
  renderedHTML: string;
  // Compact layout only: language, version and adaptation selectors.
  selectors: ReactNode;
  onArchiveClick: () => void;
  onDuplicateClick: () => void;
  onNavigateToReview: (emailId: string) => void;
  onOpenedChange: (opened: boolean) => void;
};

export function ReviewMoreMenu({
  archiveIsPending,
  canManage,
  currentListEmail,
  email,
  isCompactReview,
  isSuperAdmin,
  nextEmailWithOpenComments,
  opened,
  renderedHTML,
  selectors,
  onArchiveClick,
  onDuplicateClick,
  onNavigateToReview,
  onOpenedChange,
}: ReviewMoreMenuProps) {
  const [bitrixCopyOpened, setBitrixCopyOpened] = useState(false);
  const uploadEmail = currentListEmail ?? email ?? null;
  const copyLink = () => {
    if (!email) {
      return;
    }
    const url = `${window.location.origin}/emails/${encodeURIComponent(email.id)}/review`;
    void navigator.clipboard.writeText(url).then(
      () => notifications.show({ color: "green", message: "Link copied.", title: "Copied" }),
      () => notifications.show({ color: "red", message: url, title: "Could not copy" })
    );
  };

  return (
    <>
      <Menu
        opened={opened}
        position="bottom-end"
        width={isCompactReview ? 260 : 240}
        withinPortal
        onChange={onOpenedChange}
      >
        <Menu.Target>
          <Button
            aria-label="Open actions menu"
            className={styles.moreMenuButton}
            data-expanded={opened || undefined}
            rightSection={<CaretDownIcon aria-hidden="true" size={12} />}
            size="xs"
            variant="subtle"
          >
            More
          </Button>
        </Menu.Target>
        <Menu.Dropdown>
          {isCompactReview ? (
            <>
              <Menu.Label>Email version</Menu.Label>
              <div className={styles.menuControls}>{selectors}</div>
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
              <Menu.Divider />
            </>
          ) : null}
          <Menu.Item
            disabled={!email || !renderedHTML}
            leftSection={<DownloadSimpleIcon aria-hidden="true" size={15} />}
            onClick={() => email && downloadRenderedHTML(email, renderedHTML)}
          >
            Download HTML
          </Menu.Item>
          <Menu.Item
            disabled={!email}
            leftSection={<CopySimpleIcon aria-hidden="true" size={15} />}
            onClick={copyLink}
          >
            Copy link
          </Menu.Item>
          {canManage ? (
            <>
              <Menu.Item
                disabled={!email}
                leftSection={<CopyIcon aria-hidden="true" size={15} />}
                onClick={onDuplicateClick}
              >
                Duplicate as...
              </Menu.Item>
              <Menu.Item
                color="red"
                disabled={!email}
                leftSection={<ArchiveIcon aria-hidden="true" size={15} />}
                onClick={onArchiveClick}
              >
                {archiveIsPending ? "Archiving" : "Archive email"}
              </Menu.Item>
            </>
          ) : null}
          {isSuperAdmin ? (
            <Menu.Item
              disabled={!uploadEmail}
              leftSection={<UploadSimpleIcon aria-hidden="true" size={15} />}
              onClick={() => setBitrixCopyOpened(true)}
            >
              Copy for Bitrix24...
            </Menu.Item>
          ) : null}
        </Menu.Dropdown>
      </Menu>
      <BitrixCopyModal
        email={uploadEmail}
        opened={bitrixCopyOpened}
        onClose={() => setBitrixCopyOpened(false)}
      />
    </>
  );
}
