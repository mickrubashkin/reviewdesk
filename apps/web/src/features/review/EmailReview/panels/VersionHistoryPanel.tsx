import { Alert, Badge, Button, Group, Loader, Modal, Stack, Text } from "@mantine/core";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { fetchEmailVersion } from "../../../emails/api";
import type {
  EmailVersionDetail,
  EmailVersionListItem,
  UserRole,
} from "../../../emails/types";
import styles from "../EmailReview.module.css";

export function VersionHistoryPanel({
  canManage,
  currentUserRole,
  isLoading,
  isRestoring,
  onRestore,
  restoreError,
  versions,
}: {
  canManage: boolean;
  currentUserRole: UserRole;
  isLoading: boolean;
  isRestoring: boolean;
  onRestore: (versionId: string) => void;
  restoreError: boolean;
  versions: EmailVersionListItem[];
}) {
  const [restoreVersion, setRestoreVersion] =
    useState<EmailVersionListItem | null>(null);
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null);
  const selectedVersion =
    versions.find((version) => version.id === selectedVersionId) ?? null;
  const previousVersion = selectedVersion
    ? versions.find(
        (version) => version.version_number === selectedVersion.version_number - 1
      ) ?? null
    : null;
  const selectedVersionQuery = useQuery({
    queryKey: [
      "emails",
      selectedVersion?.email_id ?? "",
      "versions",
      selectedVersion?.id ?? "",
    ],
    queryFn: () =>
      fetchEmailVersion(selectedVersion?.email_id ?? "", selectedVersion?.id ?? ""),
    enabled: Boolean(selectedVersion),
  });
  const previousVersionQuery = useQuery({
    queryKey: [
      "emails",
      previousVersion?.email_id ?? "",
      "versions",
      previousVersion?.id ?? "",
    ],
    queryFn: () =>
      fetchEmailVersion(previousVersion?.email_id ?? "", previousVersion?.id ?? ""),
    enabled: Boolean(previousVersion),
  });

  if (isLoading) {
    return (
      <Stack className={styles.emptyState} align="center" justify="center">
        <Loader size="sm" />
        <Text c="dimmed" size="sm">
          Loading versions
        </Text>
      </Stack>
    );
  }

  if (versions.length === 0) {
    return (
      <Alert color="gray" title="No versions" variant="light">
        Save an edit to create version history.
      </Alert>
    );
  }

  return (
    <>
      <Stack gap="sm">
        {restoreError ? (
          <Alert color="red" title="Restore failed" variant="light">
            This version may require super admin access or contain invalid fields.
          </Alert>
        ) : null}
        {versions.map((version) => {
          const requiresSuperAdmin =
            version.html_changed && currentUserRole !== "super_admin";
          const isSelected = selectedVersionId === version.id;
          return (
            <section className={styles.historyVersionCard} key={version.id}>
              <Stack gap="xs">
                <Group justify="space-between" gap="sm">
                  <Group gap="xs">
                    <Text fw={700} size="sm">
                      v{version.version_number}
                    </Text>
                    <Badge size="sm" variant="light">
                      {formatVersionSource(version)}
                    </Badge>
                    {version.restored_from_version_number ? (
                      <Badge color="blue" size="sm" variant="light">
                        from v{version.restored_from_version_number}
                      </Badge>
                    ) : null}
                  </Group>
                  {canManage ? (
                    <Group gap={4} wrap="nowrap">
                      <Button
                        size="xs"
                        variant={isSelected ? "filled" : "subtle"}
                        onClick={() =>
                          setSelectedVersionId(isSelected ? null : version.id)
                        }
                      >
                        Details
                      </Button>
                      <Button
                        disabled={requiresSuperAdmin}
                        loading={isRestoring && restoreVersion?.id === version.id}
                        size="xs"
                        variant="light"
                        onClick={() => setRestoreVersion(version)}
                      >
                        Restore
                      </Button>
                    </Group>
                  ) : null}
                </Group>
                <Text c="dimmed" size="xs">
                  {formatVersionDate(version.created_at)} by{" "}
                  {version.created_by_email}
                </Text>
                <Text size="sm">{formatVersionSummary(version)}</Text>
                {requiresSuperAdmin ? (
                  <Text c="orange" size="xs">
                    Template HTML changed. Restore requires super admin.
                  </Text>
                ) : null}
                {isSelected ? (
                  <VersionChangeDetails
                    current={selectedVersionQuery.data}
                    isError={selectedVersionQuery.isError || previousVersionQuery.isError}
                    isLoading={
                      selectedVersionQuery.isLoading ||
                      (Boolean(previousVersion) && previousVersionQuery.isLoading)
                    }
                    previous={previousVersionQuery.data}
                    hasPrevious={Boolean(previousVersion)}
                  />
                ) : null}
              </Stack>
            </section>
          );
        })}
      </Stack>

      <Modal
        centered
        opened={Boolean(restoreVersion)}
        title="Restore version"
        onClose={() => setRestoreVersion(null)}
      >
        <Stack gap="md">
          <Text size="sm">
            Restore {restoreVersion ? `v${restoreVersion.version_number}` : "this version"} as
            the latest email state. This creates a new version.
          </Text>
          <Group justify="flex-end">
            <Button
              disabled={isRestoring}
              variant="subtle"
              onClick={() => setRestoreVersion(null)}
            >
              Cancel
            </Button>
            <Button
              loading={isRestoring}
              onClick={() => {
                if (restoreVersion) {
                  onRestore(restoreVersion.id);
                  setRestoreVersion(null);
                }
              }}
            >
              Restore
            </Button>
          </Group>
        </Stack>
      </Modal>
    </>
  );
}

function formatVersionDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatVersionSource(version: EmailVersionListItem) {
  switch (version.source) {
    case "initial":
      return "Initial";
    case "restore":
      return "Restore";
    default:
      return "Manual";
  }
}

function formatVersionSummary(version: EmailVersionListItem) {
  const parts: string[] = [];
  if (version.changed_metadata_count > 0) {
    parts.push(`${version.changed_metadata_count} metadata fields`);
  }
  if (version.changed_field_count > 0) {
    parts.push(`${version.changed_field_count} editable fields`);
  }
  if (version.html_changed) {
    parts.push("template HTML");
  }

  return parts.length > 0 ? `${parts.join(", ")} changed` : "Baseline snapshot";
}

function VersionChangeDetails({
  current,
  hasPrevious,
  isError,
  isLoading,
  previous,
}: {
  current: EmailVersionDetail | undefined;
  hasPrevious: boolean;
  isError: boolean;
  isLoading: boolean;
  previous: EmailVersionDetail | undefined;
}) {
  if (isLoading) {
    return (
      <Group className={styles.historyDetails} gap="xs">
        <Loader size="xs" />
        <Text c="dimmed" size="xs">
          Loading change details
        </Text>
      </Group>
    );
  }

  if (isError || !current) {
    return (
      <Alert color="red" title="Could not load changes" variant="light">
        Try reopening the history panel.
      </Alert>
    );
  }

  if (!hasPrevious || !previous) {
    return (
      <Stack className={styles.historyDetails} gap={4}>
        <Text c="dimmed" size="xs">
          Initial snapshot has no previous version to compare.
        </Text>
      </Stack>
    );
  }

  const changes = buildVersionChanges(current, previous);
  if (changes.length === 0) {
    return (
      <Stack className={styles.historyDetails} gap={4}>
        <Text c="dimmed" size="xs">
          No effective differences from the previous version.
        </Text>
      </Stack>
    );
  }

  return (
    <Stack className={styles.historyDetails} gap="xs">
      {changes.map((change) => (
        <Stack className={styles.historyChangeRow} gap={4} key={change.label}>
          <Text fw={700} size="xs">
            {change.label}
          </Text>
          <Group align="flex-start" gap="xs" wrap="nowrap">
            <Text className={styles.historyChangeValue} c="dimmed" size="xs">
              {change.before}
            </Text>
            <Text c="dimmed" size="xs">
              -&gt;
            </Text>
            <Text className={styles.historyChangeValue} size="xs">
              {change.after}
            </Text>
          </Group>
        </Stack>
      ))}
    </Stack>
  );
}

type VersionChange = {
  label: string;
  before: string;
  after: string;
};

function buildVersionChanges(
  current: EmailVersionDetail,
  previous: EmailVersionDetail
): VersionChange[] {
  const changes: VersionChange[] = [];
  appendChange(changes, "Title", previous.title, current.title);
  appendChange(changes, "Subject", previous.subject, current.subject);
  appendChange(changes, "Preheader", previous.preheader, current.preheader);

  const fieldKeys = Array.from(
    new Set([
      ...Object.keys(previous.editable_fields),
      ...Object.keys(current.editable_fields),
    ])
  ).sort((first, second) => {
    const firstOrder =
      current.editable_fields[first]?.order ??
      previous.editable_fields[first]?.order ??
      Number.MAX_SAFE_INTEGER;
    const secondOrder =
      current.editable_fields[second]?.order ??
      previous.editable_fields[second]?.order ??
      Number.MAX_SAFE_INTEGER;
    return firstOrder === secondOrder
      ? first.localeCompare(second)
      : firstOrder - secondOrder;
  });

  fieldKeys.forEach((key) => {
    const beforeField = previous.editable_fields[key];
    const afterField = current.editable_fields[key];
    if (editableFieldSignature(beforeField) === editableFieldSignature(afterField)) {
      return;
    }
    changes.push({
      label: key,
      before: formatEditableFieldValue(beforeField),
      after: formatEditableFieldValue(afterField),
    });
  });

  appendChange(
    changes,
    "Original HTML",
    previous.original_html,
    current.original_html,
    "Previous HTML",
    "New HTML"
  );
  appendChange(
    changes,
    "Template HTML",
    previous.template_html,
    current.template_html,
    "Previous HTML",
    "New HTML"
  );

  return changes;
}

function appendChange(
  changes: VersionChange[],
  label: string,
  before: string | number | null | undefined,
  after: string | number | null | undefined,
  beforeLabel?: string,
  afterLabel?: string
) {
  if (normalizeVersionValue(before) === normalizeVersionValue(after)) {
    return;
  }
  changes.push({
    label,
    before: beforeLabel ?? formatVersionValue(before),
    after: afterLabel ?? formatVersionValue(after),
  });
}

function editableFieldSignature(
  field: EmailVersionDetail["editable_fields"][string] | undefined
) {
  return field
    ? `${field.type}:${normalizeVersionValue(field.value)}`
    : "__missing__";
}

function formatEditableFieldValue(
  field: EmailVersionDetail["editable_fields"][string] | undefined
) {
  return field ? formatVersionValue(field.value) : "Not set";
}

function normalizeVersionValue(value: string | number | null | undefined) {
  return value === null || value === undefined ? "" : String(value);
}

function formatVersionValue(value: string | number | null | undefined) {
  const normalized = normalizeVersionValue(value);
  return normalized.trim() ? normalized : "Empty";
}
