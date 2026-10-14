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
