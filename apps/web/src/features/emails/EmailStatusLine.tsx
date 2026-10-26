import { Fragment } from "react";
import { Stack, Text, Tooltip } from "@mantine/core";

import { emailStatusParts } from "./statusSummary";
import type { EmailListItem } from "./types";

// EmailStatusLine shows an email's status as one line, "Live v3 · v4 draft ·
// re-upload", with the details in a tooltip.
export function EmailStatusLine({
  email,
  versions,
  size = "xs",
  noWrap = false,
}: {
  email: EmailListItem;
  // Other emails of the same slot and language.
  versions?: EmailListItem[];
  size?: "xs" | "sm";
  // Keep the whole line on one row, e.g. in a card footer.
  noWrap?: boolean;
}) {
  const parts = emailStatusParts(email, versions);
  if (parts.length === 0) {
    return null;
  }
  return (
    <Tooltip
      label={
        <Stack gap={2}>
          {parts.map((part) => (
            <Text key={part.label} size="xs">
              {part.detail}
            </Text>
          ))}
        </Stack>
      }
      multiline
      w={280}
    >
      <Text component="span" fw={500} size={size} style={noWrap ? { flexShrink: 0, whiteSpace: "nowrap" } : { minWidth: 0 }}>
        {parts.map((part, index) => (
          <Fragment key={part.label}>
            {index > 0 ? (
              <Text c="dimmed" component="span" inherit>
                {" · "}
              </Text>
            ) : null}
            <Text
              c={part.color === "dimmed" ? "dimmed" : `${part.color}.8`}
              component="span"
              inherit
              style={{ whiteSpace: "nowrap" }}
            >
              {part.label}
            </Text>
          </Fragment>
        ))}
      </Text>
    </Tooltip>
  );
}
