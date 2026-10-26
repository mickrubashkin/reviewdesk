import { List, Stack, Text, ThemeIcon } from "@mantine/core";
import { WarningCircleIcon, WarningIcon } from "@phosphor-icons/react";

import type { EmailCheck } from "../../../emails/types";

// ChecksPanel lists the rules of form the email breaks. Errors (no button,
// an empty required field) cannot be brought in by a save; warnings (a text
// over its length limit) can, but both keep the email from Ready.
export function ChecksPanel({ checks }: { checks: EmailCheck[] }) {
  if (checks.length === 0) {
    return null;
  }
  return (
    <Stack gap={6}>
      <Text fw={700} size="sm">
        Checks
      </Text>
      <Text c="dimmed" size="xs">
        Fix these before marking the email ready.
      </Text>
      <List spacing={4} size="sm">
        {checks.map((check) => (
          <List.Item
            icon={
              <ThemeIcon color={check.level === "error" ? "red" : "yellow"} size={18} variant="light">
                {check.level === "error" ? <WarningCircleIcon size={13} /> : <WarningIcon size={13} />}
              </ThemeIcon>
            }
            key={`${check.level}:${check.field ?? ""}:${check.message}`}
          >
            {check.message}
          </List.Item>
        ))}
      </List>
    </Stack>
  );
}
