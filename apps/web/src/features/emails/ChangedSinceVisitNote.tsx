import { Alert, Anchor, Text, type MantineSpacing } from "@mantine/core";

import { useUserName } from "../../shared/userNames";
import type { ChangesSinceVisit } from "./visit";

const shortDateTime = (value: string) =>
  new Date(value).toLocaleString(undefined, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });

// ChangedSinceVisitNote says what others changed in an email since the
// person's last visit: the fields and who; the blocks are outlined in the
// preview.
export function ChangedSinceVisitNote({
  changes,
  mb,
  onOpenHistory,
}: {
  changes: ChangesSinceVisit | null;
  mb?: MantineSpacing;
  onOpenHistory: () => void;
}) {
  const userName = useUserName();
  if (!changes) {
    return null;
  }
  return (
    <Alert color="blue" mb={mb} py="xs" title={`Changed since your last visit (${shortDateTime(changes.since)})`} variant="light">
      <Text size="xs">
        {changes.fields.length > 0 ? `${changes.fields.join(", ")}, ` : ""}by{" "}
        {changes.people.map((person) => userName(person)).join(", ")}.
        {changes.blocks.length > 0 ? " Outlined in blue below." : ""}{" "}
        <Anchor component="button" size="xs" onClick={onOpenHistory}>
          See in History
        </Anchor>
      </Text>
    </Alert>
  );
}
