import type { EmailListItem, ProductionStatus } from "./types";

export function getProductionStatus(email: EmailListItem, boardSyncedAt: string | null | undefined): ProductionStatus {
  if (email.portal_send_count > 0) return "live";
  if (email.live_marked_at) return "live_manual";
  return boardSyncedAt ? "not_seen" : "unknown";
}

export const productionStatusOptions = [
  { label: "Live (sent by robots or marked)", value: "live" },
  { label: "Not seen on the portal", value: "not_seen" },
];

export function matchesProductionFilter(status: ProductionStatus | undefined, filter: string) {
  if (!filter) return true;
  if (filter === "live") return status === "live" || status === "live_manual";
  return status === filter;
}

const dayMs = 24 * 60 * 60 * 1000;

// sendQuietDays tells a live email whose robot seems to have stopped: the
// days from its last send to the board's last portal sync, when that is three
// times its usual gap (30 days over its sends of the last 30 days), at least a
// week and at most 30 days. null when it sends as usual, or nothing is known.
export function sendQuietDays(
  email: Pick<EmailListItem, "portal_send_count" | "portal_sends_30d" | "portal_last_sent_at">,
  boardSyncedAt: string | null | undefined
): number | null {
  if (!boardSyncedAt || !email.portal_last_sent_at || email.portal_send_count === 0) {
    return null;
  }
  const days = Math.floor((Date.parse(boardSyncedAt) - Date.parse(email.portal_last_sent_at)) / dayMs);
  const sends = email.portal_sends_30d ?? 0;
  const usualGap = sends > 0 ? 30 / sends : 30;
  return days >= Math.min(30, Math.max(7, 3 * usualGap)) ? days : null;
}

// withProductionStatus adds what the board's last portal sync says about an
// email: whether it is live and whether it went quiet.
export function withProductionStatus<T extends EmailListItem>(email: T, boardSyncedAt: string | null | undefined): T {
  return {
    ...email,
    production_status: getProductionStatus(email, boardSyncedAt),
    portal_quiet_days: sendQuietDays(email, boardSyncedAt),
  };
}
