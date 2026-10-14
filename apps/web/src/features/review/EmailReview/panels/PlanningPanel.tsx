import { Button, Group, Loader, Select, Stack, Text, Textarea, TextInput } from "@mantine/core";
import { useState } from "react";

import type { EmailDetail, UpdateEmailPlanningFieldsPayload, UserAdminItem, UserRole } from "../../../emails/types";
import { nullableTrimmed } from "../EmailReview.helpers";
import styles from "../EmailReview.module.css";

function assigneeOptions(
  users: UserAdminItem[],
  currentEmail: string,
  allowedRoles: UserRole[]
) {
  const allowedRoleSet = new Set<UserRole>(allowedRoles);
  const options = users
    .filter((user) => allowedRoleSet.has(user.role))
    .map((user) => ({
      label: `${user.email} (${formatUserRole(user.role)})`,
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
  const [reviewerEmail, setReviewerEmail] = useState(email?.reviewer_email ?? "");
  const [dueDate, setDueDate] = useState(email?.due_date ?? "");
  const [implementationNotes, setImplementationNotes] = useState(
    email?.implementation_notes ?? ""
  );
  const [sendTiming, setSendTiming] = useState(email?.send_timing ?? "");
  const [sendCondition, setSendCondition] = useState(email?.send_condition ?? "");
  const [adaptationLabel, setAdaptationLabel] = useState(
    email?.adaptation_label ?? "Default"
  );

  if (!email) {
    return (
      <Stack className={styles.emptyState} align="center" justify="center">
        <Loader size="sm" />
      </Stack>
    );
  }

  const payload = {
    owner_email: nullableTrimmed(ownerEmail),
    reviewer_email: nullableTrimmed(reviewerEmail),
    due_date: nullableTrimmed(dueDate),
    implementation_notes: nullableTrimmed(implementationNotes),
    send_timing: nullableTrimmed(sendTiming),
    send_condition: nullableTrimmed(sendCondition),
    adaptation_label: adaptationLabel.trim() || "Default",
  };
  const isDirty =
    payload.owner_email !== (email.owner_email ?? null) ||
    payload.reviewer_email !== (email.reviewer_email ?? null) ||
    payload.due_date !== (email.due_date ?? null) ||
    payload.implementation_notes !== (email.implementation_notes ?? null) ||
    payload.send_timing !== (email.send_timing ?? null) ||
    payload.send_condition !== (email.send_condition ?? null) ||
    payload.adaptation_label !== email.adaptation_label;
  const ownerOptions = assigneeOptions(adminUsers, ownerEmail, [
    "admin",
    "super_admin",
  ]);
  const reviewerOptions = assigneeOptions(adminUsers, reviewerEmail, [
    "reviewer",
    "admin",
    "super_admin",
  ]);

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
        <Select
          clearable
          disabled={!canManage || isSaving}
          label="Reviewer"
          nothingFoundMessage="No users found"
          placeholder="reviewer@example.com"
          searchable
          data={reviewerOptions}
          value={reviewerEmail || null}
          onChange={(value) => setReviewerEmail(value ?? "")}
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
          <TextInput
            disabled={!canManage || isSaving}
            label="Adaptation"
            placeholder="Default"
            value={adaptationLabel}
            onChange={(event) => setAdaptationLabel(event.currentTarget.value)}
          />
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
