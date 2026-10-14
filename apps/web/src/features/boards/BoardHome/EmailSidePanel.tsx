import {
  ActionIcon,
  Badge,
  Button,
  Group,
  Loader,
  ScrollArea,
  SegmentedControl,
  Stack,
  Text,
  Title,
  Tooltip,
} from "@mantine/core";
import { ArrowSquareOutIcon, XIcon } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";

import { fetchEmailDetail } from "../../emails/api";
import { MailPreview } from "../../emails/MailPreview";
import { ProductionBadge } from "../../emails/ProductionBadge";
import { formatEmailReviewStatus } from "../../emails/reviewStatus";
import { formatStageName } from "../../emails/stages";
import type { EmailListItem } from "../../emails/types";
import { pickPrimary } from "./slotHelpers";
import { TranslationLinkSelect, TranslationSection } from "./TranslationSection";
import styles from "./EmailSidePanel.module.css";

// EmailSidePanel previews an email next to the board, so people can glance at
// emails while moving around the board without leaving it.
export function EmailSidePanel({
  canManage,
  email,
  siblings,
  onClose,
  onSelect,
}: {
  canManage: boolean;
  email: EmailListItem;
  // Other emails of the same slot, to switch language or version in place.
  siblings: EmailListItem[];
  onClose: () => void;
  onSelect: (email: EmailListItem) => void;
}) {
  const detailQuery = useQuery({
    queryKey: ["emails", email.id, "review"],
    queryFn: () => fetchEmailDetail(email.id),
  });
  const languages = [...new Set(siblings.map((s) => s.language))];
  const sameLanguage = siblings.filter((s) => s.language === email.language);

  return (
    <aside aria-label="Email preview" className={styles.panel}>
      <Group className={styles.header} justify="space-between" wrap="nowrap">
        <Stack gap={2} miw={0}>
          <Text c="dimmed" size="xs">
            {formatStageName(email.stage ?? "")} · #{email.sort_order}
          </Text>
          <Title lineClamp={1} order={5}>
            {email.title}
          </Title>
        </Stack>
        <Group gap={6} wrap="nowrap">
          <Button
            component={Link}
            leftSection={<ArrowSquareOutIcon aria-hidden="true" size={14} />}
            size="xs"
            to={`/emails/${encodeURIComponent(email.id)}/review`}
            variant="light"
          >
            Open
          </Button>
          <Tooltip label="Close">
            <ActionIcon aria-label="Close preview" variant="subtle" onClick={onClose}>
              <XIcon aria-hidden="true" size={16} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Group>

      <Stack className={styles.switchers} gap={6}>
        {languages.length > 1 ? (
          <SegmentedControl
            data={languages.map((l) => ({ label: l.toUpperCase(), value: l }))}
            size="xs"
            value={email.language}
            onChange={(language) => {
              const next = pickPrimary(siblings.filter((s) => s.language === language));
              if (next) onSelect(next);
            }}
          />
        ) : null}
        {sameLanguage.length > 1 ? (
          <Group gap={4}>
            {sameLanguage.map((version) => (
              <Button
                key={version.id}
                size="compact-xs"
                variant={version.id === email.id ? "filled" : "default"}
                onClick={() => onSelect(version)}
              >
                {version.variant}
                {version.adaptation_key !== "default" ? ` · ${version.adaptation_label}` : ""}
              </Button>
            ))}
          </Group>
        ) : null}
        {canManage && email.language !== "en" ? (
          <TranslationLinkSelect email={email} siblings={siblings} />
        ) : null}
        <Group gap={6}>
          <ProductionBadge email={email} versions={sameLanguage} />
          <Badge size="xs" variant="light">
            {formatEmailReviewStatus(email.review_status)}
          </Badge>
          {email.open_comment_count > 0 ? (
            <Badge color="orange" size="xs" variant="light">
              {email.open_comment_count} open comments
            </Badge>
          ) : null}
          <Text c="dimmed" size="xs">
            {[email.send_timing, email.send_condition].filter(Boolean).join(" · ")}
          </Text>
        </Group>
      </Stack>

      <ScrollArea className={styles.body} type="auto">
        <div className={styles.translation}>
          <TranslationSection canManage={canManage} email={email} />
        </div>
        {detailQuery.data ? (
          <MailPreview email={detailQuery.data} isScanning={false} viewport="desktop" />
        ) : (
          <Stack align="center" p="xl">
            <Loader size="sm" />
          </Stack>
        )}
      </ScrollArea>
    </aside>
  );
}
