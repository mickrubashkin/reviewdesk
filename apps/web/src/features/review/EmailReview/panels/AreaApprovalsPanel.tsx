import { Alert, Badge, Button, Group, Loader, Stack, Text, Textarea } from "@mantine/core";
import { CheckIcon } from "@phosphor-icons/react";
import { useState } from "react";

import type { EmailAreaApproval } from "../../../emails/types";
import {
  areaApprovalStatusColor,
  formatAreaApprovalStatus,
  formatCommentDate,
  nullableTrimmed,
} from "../EmailReview.helpers";
import styles from "../EmailReview.module.css";

export function AreaApprovalsPanel({
  approvals,
  canManage,
  isError,
  isLoading,
  onSubmit,
  pendingArea,
}: {
  approvals: EmailAreaApproval[];
  canManage: boolean;
  isError: boolean;
  isLoading: boolean;
  onSubmit: (
    area: string,
    status: "approved" | "changes_requested",
    decisionNote: string | null
  ) => void;
  pendingArea: string | null;
}) {
  const [notesByArea, setNotesByArea] = useState<Record<string, string>>({});

  if (isLoading) {
    return (
      <Stack className={styles.emptyState} align="center" justify="center">
        <Loader size="sm" />
      </Stack>
    );
  }

  if (isError) {
    return (
      <Alert color="red" title="Could not load approvals" variant="light">
        Try refreshing the page or check the API server.
      </Alert>
    );
  }

  if (approvals.length === 0) {
    return (
      <Alert color="gray" title="No approval areas" variant="light">
        Add approval areas in board settings.
      </Alert>
    );
  }

  const staleApprovals = approvals.filter((approval) => approval.status === "stale");

  return (
    <Stack gap="sm">
      {staleApprovals.length > 0 ? (
        <Alert color="orange" title="Approval stale after edit" variant="light">
          Re-approve {staleApprovals.map((approval) => approval.name).join(", ")} before
          final approval or handoff.
        </Alert>
      ) : null}

      {approvals.map((approval) => {
        const noteDraft =
          notesByArea[approval.area] ?? approval.decision_note ?? "";
        const isPending = pendingArea === approval.area;

        return (
          <Stack
            className={styles.approvalAreaItem}
            gap="xs"
            key={approval.board_approval_area_id}
          >
            <Group justify="space-between" gap="xs" wrap="nowrap">
              <Stack gap={2}>
                <Group gap={6} wrap="nowrap">
                  <Text fw={700} size="sm">
                    {approval.name}
                  </Text>
                  {approval.required ? (
                    <Badge color="blue" radius="sm" size="xs" variant="light">
                      Required
                    </Badge>
                  ) : (
                    <Badge color="gray" radius="sm" size="xs" variant="light">
                      Optional
                    </Badge>
                  )}
                </Group>
                {approval.decided_by_email && approval.decided_at ? (
                  <Text c="dimmed" size="xs">
                    {approval.decided_by_email} ·{" "}
                    {formatCommentDate(approval.decided_at)}
                  </Text>
                ) : null}
              </Stack>
              <Badge
                color={areaApprovalStatusColor(approval.status)}
                radius="sm"
                variant="light"
              >
                {formatAreaApprovalStatus(approval.status)}
              </Badge>
            </Group>

            {approval.decision_note ? (
              <Text c="dimmed" size="sm" style={{ whiteSpace: "pre-wrap" }}>
                {approval.decision_note}
              </Text>
            ) : null}

            {canManage ? (
              <>
                <Textarea
                  autosize
                  disabled={isPending}
                  minRows={2}
                  placeholder="Decision note"
                  value={noteDraft}
                  onChange={(event) =>
                    setNotesByArea((current) => ({
                      ...current,
                      [approval.area]: event.currentTarget.value,
                    }))
                  }
                />
                <Group gap="xs" grow>
                  <Button
                    color="green"
                    disabled={isPending}
                    leftSection={<CheckIcon aria-hidden="true" size={15} />}
                    loading={isPending}
                    size="xs"
                    variant="light"
                    onClick={() =>
                      onSubmit(
                        approval.area,
                        "approved",
                        nullableTrimmed(noteDraft)
                      )
                    }
                  >
                    Approve
                  </Button>
                  <Button
                    color="yellow"
                    disabled={isPending}
                    loading={isPending}
                    size="xs"
                    variant="light"
                    onClick={() =>
                      onSubmit(
                        approval.area,
                        "changes_requested",
                        nullableTrimmed(noteDraft)
                      )
                    }
                  >
                    Request changes
                  </Button>
                </Group>
              </>
            ) : null}
          </Stack>
        );
      })}
    </Stack>
  );
}
