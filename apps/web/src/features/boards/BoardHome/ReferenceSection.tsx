import { Alert, Anchor, Badge, Button, Group, Select, Stack, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useBoardRegions } from "../../boards/api";
import { confirmBasedOn, fetchBasedOnStatus, setBasedOnLink } from "../../emails/api";
import { formatStageName } from "../../emails/stages";
import type { BasedOnStatus, BlockChange, EmailListItem } from "../../emails/types";
import { HelpLink } from "../../help/HelpLink";
import { WordDiff } from "./TranslationSection";

type ReferenceEmail = Pick<
  EmailListItem,
  "id" | "sequence" | "language" | "region_key" | "translation_of" | "based_on"
>;

// ReferenceSection shows how an email of a region other than the reference
// stands against the reference email it adapts: whether the reference
// changed since and what to take, and how the email differs from it now.
// A region's email without one is its own. Translations and emails of the
// reference region show nothing.
export function ReferenceSection({
  candidates,
  canEdit,
  canManage,
  email,
}: {
  // Emails to pick the reference from: those of the reference region count.
  candidates: EmailListItem[];
  canEdit: boolean;
  canManage: boolean;
  email: ReferenceEmail;
}) {
  const queryClient = useQueryClient();
  const regions = useBoardRegions(email.sequence).data ?? [];
  const region = regions.find((r) => r.key === email.region_key);
  const referenceRegion = regions.find((r) => r.reference);
  const adapts = Boolean(region && !region.reference && !email.translation_of);

  const statusQuery = useQuery({
    queryKey: ["emails", email.id, "based-on"],
    queryFn: () => fetchBasedOnStatus(email.id),
    enabled: adapts && Boolean(email.based_on),
  });
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["emails"] });
    void queryClient.invalidateQueries({ queryKey: ["email-activity", email.id] });
  };
  const confirmMutation = useMutation({
    mutationFn: () => confirmBasedOn(email.id),
    onSuccess: () => {
      invalidate();
      notifications.show({ color: "green", message: "Marked up to date with the reference.", title: "Reference" });
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });
  const linkMutation = useMutation({
    mutationFn: (referenceId: string | null) => setBasedOnLink(email.id, referenceId),
    onSuccess: invalidate,
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });

  if (!adapts || !region) {
    return null;
  }
  const references = candidates.filter((c) => referenceRegion && c.region_key === referenceRegion.key && !c.archived_at);
  const status = email.based_on ? statusQuery.data : undefined;

  return (
    <Stack gap="xs">
      {canManage ? (
        <Select
          allowDeselect={false}
          data={[
            ...references.map((r) => ({
              label: `${formatStageName(r.stage ?? "")} #${r.sort_order} · ${r.title}`,
              value: r.id,
            })),
            { label: `Own email of ${region.name}`, value: ownEmail },
          ]}
          disabled={linkMutation.isPending}
          label={
            <Group component="span" gap={4} wrap="nowrap">
              Based on the reference
              <HelpLink topic="reference" />
            </Group>
          }
          size="xs"
          value={linkMutation.isPending ? (linkMutation.variables ?? ownEmail) : (email.based_on ?? ownEmail)}
          onChange={(value) => linkMutation.mutate(value === ownEmail || !value ? null : value)}
        />
      ) : null}
      {!email.based_on ? (
        <Text c="dimmed" size="xs">
          Own email of {region.name}: not based on a reference email
          {referenceRegion ? ` (${referenceRegion.name})` : ""}.
        </Text>
      ) : status ? (
        <>
          <ReferenceChanges
            canEdit={canEdit}
            isConfirming={confirmMutation.isPending}
            status={status}
            onConfirm={() => confirmMutation.mutate()}
          />
          <ReferenceDifferences status={status} />
        </>
      ) : null}
    </Stack>
  );
}

const ownEmail = "__own__";

function ReferenceChanges({
  canEdit,
  isConfirming,
  status,
  onConfirm,
}: {
  canEdit: boolean;
  isConfirming: boolean;
  status: BasedOnStatus;
  onConfirm: () => void;
}) {
  if (!status.stale) {
    return (
      <Group gap={6}>
        <Badge color="green" size="xs" variant="light">
          Up to date with the reference
        </Badge>
        <Text c="dimmed" size="xs">
          {status.reference_title} · v{status.reference_latest_version}
        </Text>
      </Group>
    );
  }
  return (
    <Alert
      color="orange"
      title={`The reference changed since v${status.based_on_version} (${status.changes.length})`}
      variant="light"
    >
      <Stack gap="xs">
        {status.block_changes.length > 0 ? <Text size="xs">{blockChangesText(status.block_changes)}</Text> : null}
        {status.changes.map((change) => (
          <Stack key={change.key} gap={2}>
            <Text fw={600} size="xs">
              {change.key}
            </Text>
            <WordDiff after={change.after} before={change.before} />
          </Stack>
        ))}
        {canEdit ? (
          <Group>
            <Button loading={isConfirming} size="compact-xs" variant="default" onClick={onConfirm}>
              Mark up to date
            </Button>
            <Text c="dimmed" size="xs">
              after taking what the region needs
            </Text>
          </Group>
        ) : null}
      </Stack>
    </Alert>
  );
}

// ReferenceDifferences sums up how the email differs from the reference now;
// the fields open on demand.
function ReferenceDifferences({ status }: { status: BasedOnStatus }) {
  const [opened, setOpened] = useState(false);
  const fields = status.field_differences;
  const parts = [
    fields.length > 0 ? `${fields.length} ${fields.length === 1 ? "field" : "fields"}` : "",
    status.block_differences.length > 0 ? `blocks: ${blockDifferencesText(status.block_differences)}` : "",
    status.timing_difference
      ? `timing: ${status.timing_difference.own || "none"} (reference ${status.timing_difference.reference || "none"})`
      : "",
  ].filter(Boolean);

  if (parts.length === 0) {
    return (
      <Text c="dimmed" size="xs">
        Same as the reference.
      </Text>
    );
  }
  return (
    <Stack gap={4}>
      <Text size="xs">
        Differs from the reference: {parts.join(" · ")}
        {fields.length > 0 ? (
          <>
            {" "}
            <Anchor component="button" size="xs" type="button" onClick={() => setOpened((value) => !value)}>
              {opened ? "Hide" : "Show"}
            </Anchor>
          </>
        ) : null}
      </Text>
      {opened
        ? fields.map((field) => (
            <Stack key={field.key} gap={2}>
              <Text fw={600} size="xs">
                {field.key}
              </Text>
              <WordDiff after={field.own} before={field.reference} />
            </Stack>
          ))
        : null}
    </Stack>
  );
}

function blockChangesText(changes: BlockChange[]) {
  return changes
    .map((change) => `${change.action === "added" ? "Added" : "Removed"} block ${change.name || change.block}`)
    .join(" · ");
}

function blockDifferencesText(changes: BlockChange[]) {
  return changes.map((change) => `${change.action === "added" ? "+" : "−"}${change.name || change.block}`).join(", ");
}
