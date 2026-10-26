import { Alert, Button, Group, Text } from "@mantine/core";
import { ArchiveIcon } from "@phosphor-icons/react";

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

// Shown on an archived email: it is board history and read-only until an
// admin restores it.
export function ArchivedEmailNotice({
  archivedAt,
  archivedBy,
  canRestore,
  isRestoring,
  onRestore,
}: {
  archivedAt: string;
  archivedBy: string | null;
  canRestore: boolean;
  isRestoring: boolean;
  onRestore: () => void;
}) {
  return (
    <Alert color="gray" icon={<ArchiveIcon aria-hidden="true" size={18} />} m="md" mb={0} radius="md">
      <Group justify="space-between" wrap="wrap" gap="sm">
        <Text size="sm">
          Archived {dateFormat.format(new Date(archivedAt))}
          {archivedBy ? ` by ${archivedBy}` : ""}. It is not on the board and cannot be edited
          {canRestore ? " until you restore it." : "."}
        </Text>
        {canRestore ? (
          <Button loading={isRestoring} size="compact-sm" onClick={onRestore}>
            Restore to the board
          </Button>
        ) : null}
      </Group>
    </Alert>
  );
}
