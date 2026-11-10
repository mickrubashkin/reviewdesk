import { Badge, Tooltip, type MantineSize } from "@mantine/core";

import { splitDetail, splitTexts } from "./splitTexts";
import type { EmailListItem } from "./types";

// SplitTextsBadge marks an email whose robots send different texts; it opens
// the email's sends, where the texts show side by side.
export function SplitTextsBadge({
  email,
  size = "xs",
  onOpenSends,
}: {
  email: Pick<EmailListItem, "portal_split_texts">;
  size?: MantineSize;
  onOpenSends: () => void;
}) {
  const texts = splitTexts(email);
  if (texts.length === 0) return null;
  return (
    <Tooltip label={`${splitDetail(texts)} Click to see the sends.`} multiline maw={320}>
      {/* A span: the cards and rows around it are buttons that open the email. */}
      <Badge
        color="red"
        component="span"
        role="button"
        size={size}
        style={{ cursor: "pointer", flexShrink: 0 }}
        tabIndex={0}
        variant="light"
        onClick={(event) => {
          event.stopPropagation();
          onOpenSends();
        }}
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            onOpenSends();
          }
        }}
      >
        {texts.length} texts
      </Badge>
    </Tooltip>
  );
}
