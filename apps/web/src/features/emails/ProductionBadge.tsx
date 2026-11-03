import { Badge, Tooltip } from "@mantine/core";

import { firstSendPart, liveSendsLabel, portalSendsText } from "./statusSummary";
import type { EmailListItem } from "./types";

// ProductionBadge shows whether robots actually send an email, from the last
// portal sync. versions are the other emails of the same slot (and language),
// to point out when a different version is the one that goes out. With
// onOpenSends, a live badge opens the email's sends.
export function ProductionBadge({
  email,
  versions,
  onOpenSends,
}: {
  email: EmailListItem;
  versions: EmailListItem[];
  onOpenSends?: () => void;
}) {
  const liveOthers = versions.filter(
    (v) => v.id !== email.id && (v.production_status === "live" || v.production_status === "live_manual")
  );
  if (email.production_status === "not_seen" && liveOthers.length > 0) {
    return (
      <Tooltip
        label={`This version is not sent; live instead: ${liveOthers
          .map((v) => `${v.language.toUpperCase()} ${v.variant}${v.adaptation_key !== "default" ? ` (${v.adaptation_label})` : ""}`)
          .join(", ")}`}
        multiline
        w={260}
      >
        <Badge color="orange" size="xs" style={{ flexShrink: 0 }} variant="light">
          Other version live
        </Badge>
      </Tooltip>
    );
  }
  switch (email.production_status) {
    case "live":
      return (
        <Tooltip label={`${portalSendsText(email)}${onOpenSends ? " Click to see the sends." : ""}`} multiline w={280}>
          <Badge
            color={email.portal_quiet_days != null ? "yellow" : "green"}
            size="xs"
            style={{ flexShrink: 0, cursor: onOpenSends ? "pointer" : undefined }}
            variant="light"
            {...(onOpenSends ? { component: "button" as const, type: "button" as const, onClick: onOpenSends } : {})}
          >
            Live · {liveSendsLabel(email)}
          </Badge>
        </Tooltip>
      );
    case "live_manual":
      return (
        <Badge color="teal" size="xs" style={{ flexShrink: 0 }} variant="light">
          Live (manual)
        </Badge>
      );
    case "not_seen": {
      if (email.first_send) {
        const part = firstSendPart(email, email.first_send);
        return (
          <Tooltip label={part.detail} multiline w={280}>
            <Badge color={part.color} size="xs" style={{ flexShrink: 0 }} variant="light">
              {part.label}
            </Badge>
          </Tooltip>
        );
      }
      return (
        <Tooltip label="The portal sync has not seen CRM robots send it">
          <Badge color="gray" size="xs" style={{ flexShrink: 0 }} variant="outline">
            Not seen
          </Badge>
        </Tooltip>
      );
    }
    default:
      return null;
  }
}
