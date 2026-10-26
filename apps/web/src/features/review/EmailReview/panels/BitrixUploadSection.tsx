import { ActionIcon, Badge, Button, Group, Stack, Text, Tooltip } from "@mantine/core";
import { XIcon } from "@phosphor-icons/react";
import { notifications } from "@mantine/notifications";
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";

import { markEmailUploaded } from "../../../emails/api";
import { BitrixCopyModal } from "../../../emails/BitrixCopyModal";
import { emailUploadState, robotName, robotsBehind, uploadRobots, type UploadState } from "../../../emails/reviewStatus";
import type { EmailDetail } from "../../../emails/types";
import { MarkSeenUploadedButton } from "../../../portal/MarkSeenUploadedButton";
import { describeSuggestion, suggestionFor, useUploadSuggestions } from "../../../portal/uploadSuggestions";
import { formatCommentDate } from "../EmailReview.helpers";
import styles from "../EmailReview.module.css";
import { useUserName } from "../../../../shared/userNames";

const uploadStateLabels: Record<UploadState, { color: string; label: string; hint: string }> = {
  not_ready: { color: "gray", label: "Not ready", hint: "Mark the email ready before it goes to Bitrix24." },
  to_upload: { color: "blue", label: "To upload", hint: "Paste the HTML into the robot, then mark it uploaded." },
  uploaded: { color: "teal", label: "Uploaded", hint: "Every robot has the current version." },
  changed_since_upload: {
    color: "orange",
    label: "Changed since upload",
    hint: "A robot has an older version or none. Re-upload once the email is ready.",
  },
};

// BitrixUploadSection shows which of the email's robots in Bitrix24 (one per
// manager of its region) have the current version, and lets a super admin
// copy the HTML and record the upload.
export function BitrixUploadSection({
  canMarkUploaded,
  email,
  onChanged,
}: {
  canMarkUploaded: boolean;
  email: EmailDetail;
  onChanged: () => void;
}) {
  const userName = useUserName();
  const state = emailUploadState(email);
  // Some robots have the current version and the rest none: nothing changed.
  const partly = state === "changed_since_upload" && robotsBehind(email).every((robot) => robot.version === null);
  const { color, label } = partly ? { color: "blue", label: "Not in every robot" } : uploadStateLabels[state];
  const hint = partly
    ? "Some robots do not have the email yet. Copy it into them."
    : state === "changed_since_upload" && email.review_status === "ready"
      ? "A robot has an older version or none. Copy the current one into it."
      : uploadStateLabels[state].hint;
  const robots = uploadRobots(email);
  const named = robots.length > 1 || robots[0]?.robot !== "";
  const mutation = useMutation({
    mutationFn: (robot?: string) => markEmailUploaded(email.id, false, undefined, robot === undefined ? undefined : [robot]),
    onSuccess: onChanged,
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });
  const [copyOpened, setCopyOpened] = useState(false);
  const suggestions = useUploadSuggestions(canMarkUploaded);
  const suggestion = canMarkUploaded && email.review_status === "ready" ? suggestionFor(suggestions, email) : null;

  return (
    <Stack className={styles.handoffSection} gap="xs">
      <Group justify="space-between" gap="xs" wrap="nowrap">
        <Text fw={700} size="sm">
          Bitrix24
        </Text>
        <Badge color={color} radius="sm" variant="light">
          {label}
        </Badge>
      </Group>
      <Text c="dimmed" size="xs">
        {hint}
      </Text>
      {robots.map((robot) =>
        robot.version !== null || named ? (
          <Group key={robot.robot} gap={4} wrap="nowrap">
            <Text size="xs">
              {named ? `${robot.robot}: ` : ""}
              {robot.version === null ? (
                <Text c="dimmed" component="span" size="xs">
                  not uploaded
                </Text>
              ) : (
                <>
                  version {robot.version}
                  {robot.version !== email.latest_version ? ` (current ${email.latest_version ?? "–"})` : ""}
                  {robot.uploaded_by_email && robot.uploaded_by_email !== "migration"
                    ? ` · ${userName(robot.uploaded_by_email)}`
                    : ""}
                  {robot.uploaded_at && robot.uploaded_by_email !== "migration" ? `, ${formatCommentDate(robot.uploaded_at)}` : ""}
                  {robot.uploaded_by_email === "migration" ? " (assumed: it was live when statuses were simplified)" : ""}
                </>
              )}
            </Text>
            {canMarkUploaded && named && robot.version !== null ? (
              <Tooltip label={`Remove the mark of ${robotName(robot.robot)}`}>
                <ActionIcon
                  aria-label={`Remove the mark of ${robotName(robot.robot)}`}
                  color="gray"
                  disabled={mutation.isPending}
                  size="xs"
                  variant="subtle"
                  onClick={() => mutation.mutate(robot.robot)}
                >
                  <XIcon size={10} />
                </ActionIcon>
              </Tooltip>
            ) : null}
          </Group>
        ) : null
      )}
      {suggestion ? (
        <Stack align="flex-start" gap={4}>
          <Text size="xs">{describeSuggestion(suggestion)} Is it in the robot?</Text>
          <MarkSeenUploadedButton email={email} suggestion={suggestion} />
        </Stack>
      ) : null}
      {canMarkUploaded ? (
        <Group gap="xs">
          <Button size="compact-xs" variant={state === "uploaded" ? "light" : "filled"} onClick={() => setCopyOpened(true)}>
            Copy for Bitrix24...
          </Button>
          {!named && email.uploaded_version ? (
            <Button
              color="red"
              loading={mutation.isPending}
              size="compact-xs"
              variant="subtle"
              onClick={() => mutation.mutate(undefined)}
            >
              Remove upload mark
            </Button>
          ) : null}
        </Group>
      ) : null}
      <BitrixCopyModal email={email} opened={copyOpened} onClose={() => setCopyOpened(false)} />
    </Stack>
  );
}
