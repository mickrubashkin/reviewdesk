import { Button, Group, Loader, Select, Stack, Text, Textarea, TextInput } from "@mantine/core";
import { useState } from "react";

import type { EmailDetail, UpdateEmailPlanningFieldsPayload, UserAdminItem, UserRole } from "../../../emails/types";
import { useBoardRegions } from "../../../boards/api";
import { nullableTrimmed } from "../EmailReview.helpers";
import styles from "../EmailReview.module.css";
import { canManageBoard } from "../../../emails/permissions";

function assigneeOptions(
  users: UserAdminItem[],
  currentEmail: string,
  isAllowed: (user: UserAdminItem) => boolean
) {
  const options = users
    .filter(isAllowed)
    .map((user) => ({
      label: `${user.name ? `${user.name} · ` : ""}${user.email} (${formatUserRole(user.role)})`,
      value: user.email,
    }));
  const normalizedCurrentEmail = currentEmail.trim();
  if (
    normalizedCurrentEmail &&
    !options.some((option) => option.value === normalizedCurrentEmail)
  ) {
    options.unshift({
      label: normalizedCurrentEmail,
      value: normalizedCurrentEmail,
    });
  }

  return options;
}

function formatUserRole(role: UserRole) {
  return role.replaceAll("_", " ");
}

export function PlanningPanel({
  adminUsers,
  canManage,
  email,
  isLoadingUsers,
  isSaving,
  onSubmit,
}: {
  adminUsers: UserAdminItem[];
  canManage: boolean;
  email: EmailDetail | undefined;
  isLoadingUsers: boolean;
  isSaving: boolean;
  onSubmit: (payload: UpdateEmailPlanningFieldsPayload) => void;
}) {
  const [ownerEmail, setOwnerEmail] = useState(email?.owner_email ?? "");
  const [dueDate, setDueDate] = useState(email?.due_date ?? "");
  const [implementationNotes, setImplementationNotes] = useState(
    email?.implementation_notes ?? ""
  );
  const [sendTiming, setSendTiming] = useState(email?.send_timing ?? "");
  const [sendCondition, setSendCondition] = useState(email?.send_condition ?? "");
  const [adaptationLabel, setAdaptationLabel] = useState(
    email?.adaptation_label ?? "Default"
  );
  // On a board with regions the email's version follows its region (Region
  // above), so the adaptation is not edited here.
  const hasRegions = (useBoardRegions(email?.sequence).data ?? []).length > 0;

  if (!email) {
    return (
      <Stack className={styles.emptyState} align="center" justify="center">
        <Loader size="sm" />
      </Stack>
    );
  }

  const payload = {
    owner_email: nullableTrimmed(ownerEmail),
    // Not shown in the UI any more; send the stored value back unchanged.
    reviewer_email: email.reviewer_email ?? null,
    due_date: nullableTrimmed(dueDate),
    implementation_notes: nullableTrimmed(implementationNotes),
    send_timing: nullableTrimmed(sendTiming),
    send_condition: nullableTrimmed(sendCondition),
    adaptation_label: adaptationLabel.trim() || "Default",
  };
  const isDirty =
    payload.owner_email !== (email.owner_email ?? null) ||
    payload.due_date !== (email.due_date ?? null) ||
    payload.implementation_notes !== (email.implementation_notes ?? null) ||
    payload.send_timing !== (email.send_timing ?? null) ||
    payload.send_condition !== (email.send_condition ?? null) ||
    payload.adaptation_label !== email.adaptation_label;
  // Owners are the people who manage the email's board.
  const ownerOptions = assigneeOptions(adminUsers, ownerEmail, (user) =>
    canManageBoard(user, email.sequence)
  );

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (canManage && isDirty) {
          onSubmit(payload);
        }
      }}
    >
      <Stack gap="sm">
        <Select
          clearable
          disabled={!canManage || isSaving}
          label="Owner"
          nothingFoundMessage="No users found"
          placeholder="owner@example.com"
          searchable
          data={ownerOptions}
          value={ownerEmail || null}
          onChange={(value) => setOwnerEmail(value ?? "")}
        />
        {canManage && isLoadingUsers ? (
          <Text c="dimmed" size="xs">
            Loading user options
          </Text>
        ) : null}
        <TextInput
          disabled={!canManage || isSaving}
          label="Due date"
          type="date"
          value={dueDate}
          onChange={(event) => setDueDate(event.currentTarget.value)}
        />
        <Group grow align="flex-start">
          <TextInput
            disabled={!canManage || isSaving}
            label="Send timing"
            placeholder="Day 3"
            value={sendTiming}
            onChange={(event) => setSendTiming(event.currentTarget.value)}
          />
          {hasRegions ? null : (
            <TextInput
              disabled={!canManage || isSaving}
              label="Adaptation"
              placeholder="Default"
              value={adaptationLabel}
              onChange={(event) => setAdaptationLabel(event.currentTarget.value)}
            />
          )}
        </Group>
        <TextInput
          disabled={!canManage || isSaving}
          label="Send condition"
          placeholder="If documents are not uploaded"
          value={sendCondition}
          onChange={(event) => setSendCondition(event.currentTarget.value)}
        />
        <Textarea
          autosize
          disabled={!canManage || isSaving}
          label="Implementation notes"
          minRows={4}
          value={implementationNotes}
          onChange={(event) => setImplementationNotes(event.currentTarget.value)}
        />
        {canManage ? (
          <Group justify="flex-end">
            <Button disabled={!isDirty} loading={isSaving} type="submit">
              Save
            </Button>
          </Group>
        ) : null}
      </Stack>
    </form>
  );
}
