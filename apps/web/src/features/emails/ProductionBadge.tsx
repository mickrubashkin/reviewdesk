import { Badge, Tooltip } from "@mantine/core";

import type { EmailListItem } from "./types";

// ProductionBadge shows whether robots actually send an email, from the last
// portal sync. versions are the other emails of the same slot (and language),
// to point out when a different version is the one that goes out.
export function ProductionBadge({ email, versions }: { email: EmailListItem; versions: EmailListItem[] }) {
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
        <Tooltip
          label={`Sent ${email.portal_send_count} times in the synced period${
            email.portal_last_sent_at ? `, last on ${new Date(email.portal_last_sent_at).toLocaleDateString()}` : ""
          }`}
        >
          <Badge color="green" size="xs" style={{ flexShrink: 0 }} variant="light">
            Live · {email.portal_send_count}
          </Badge>
        </Tooltip>
      );
    case "live_manual":
      return (
        <Badge color="teal" size="xs" style={{ flexShrink: 0 }} variant="light">
          Live (manual)
        </Badge>
      );
    case "not_seen":
      return (
        <Tooltip label="Not sent by CRM robots in the last synced period">
          <Badge color="gray" size="xs" style={{ flexShrink: 0 }} variant="outline">
            Not seen
          </Badge>
        </Tooltip>
      );
    default:
      return null;
  }
}
