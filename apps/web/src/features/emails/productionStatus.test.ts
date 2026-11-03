import { describe, expect, it } from "vitest";

import { sendQuietDays } from "./productionStatus";
import { emailStatusParts, lastSendLine, liveSendsLabel, portalSendsText, timeAgo } from "./statusSummary";
import type { EmailListItem } from "./types";

const synced = "2026-11-03T03:00:00Z";
const daysBefore = (days: number) => new Date(Date.parse(synced) - days * 24 * 60 * 60 * 1000).toISOString();

describe("sendQuietDays", () => {
  it("leaves an email that sends as usual alone", () => {
    expect(sendQuietDays({ portal_send_count: 400, portal_sends_30d: 120, portal_last_sent_at: daysBefore(1) }, synced)).toBeNull();
  });

  it("flags a daily email silent for a week", () => {
    expect(sendQuietDays({ portal_send_count: 400, portal_sends_30d: 90, portal_last_sent_at: daysBefore(8) }, synced)).toBe(8);
  });

  it("gives a rare email room: twice a month, 10 days without a send is normal", () => {
    expect(sendQuietDays({ portal_send_count: 12, portal_sends_30d: 2, portal_last_sent_at: daysBefore(10) }, synced)).toBeNull();
    expect(sendQuietDays({ portal_send_count: 12, portal_sends_30d: 2, portal_last_sent_at: daysBefore(25) }, synced)).toBeNull();
  });

  it("flags an email with no send in the last 30 days", () => {
    expect(sendQuietDays({ portal_send_count: 40, portal_sends_30d: 0, portal_last_sent_at: daysBefore(31) }, synced)).toBe(31);
  });

  it("knows nothing without a sync or a send", () => {
    expect(sendQuietDays({ portal_send_count: 40, portal_sends_30d: 0, portal_last_sent_at: daysBefore(31) }, null)).toBeNull();
    expect(sendQuietDays({ portal_send_count: 0, portal_sends_30d: 0, portal_last_sent_at: null }, synced)).toBeNull();
  });
});

describe("live send labels", () => {
  it("shows the sends of 30 days, or how long it has been quiet", () => {
    expect(liveSendsLabel({ portal_send_count: 400, portal_sends_30d: 120, portal_last_sent_at: null })).toBe("120/30d");
    expect(liveSendsLabel({ portal_send_count: 400, portal_sends_30d: 0, portal_last_sent_at: null, portal_quiet_days: 9 })).toBe("quiet 9d");
  });

  it("says when it was last sent", () => {
    const now = new Date("2026-11-03T10:00:00Z");
    expect(timeAgo("2026-11-03T08:00:00Z", now)).toBe("2 h ago");
    expect(timeAgo("2026-11-03T09:45:00Z", now)).toBe("15 min ago");
    expect(timeAgo("2026-11-01T10:00:00Z", now)).toBe("2 days ago");
    expect(
      portalSendsText({ portal_send_count: 400, portal_sends_30d: 120, portal_last_sent_at: "2026-11-03T08:00:00Z" }, now)
    ).toMatch(/^Last sent 2 h ago \(.+\)\. Sent 120 times in the last 30 days, 400 in the last 180 days\.$/);
  });
});

describe("timeline send line", () => {
  const now = new Date("2026-11-03T10:00:00Z");

  it("says when a live email was last sent, or how long it has been quiet", () => {
    expect(lastSendLine({ portal_send_count: 9, portal_sends_30d: 8, portal_last_sent_at: "2026-11-03T08:00:00Z" }, now)).toBe(
      "last sent 2 h ago"
    );
    expect(
      lastSendLine({ portal_send_count: 9, portal_sends_30d: 0, portal_last_sent_at: "2026-10-01T08:00:00Z", portal_quiet_days: 33 }, now)
    ).toBe("no sends for 33 days");
  });

  it("can leave the sends out of the Live label", () => {
    const email = {
      production_status: "live",
      review_status: "ready",
      portal_send_count: 9,
      portal_sends_30d: 8,
      portal_last_sent_at: null,
      uploaded_version: 2,
      latest_version: 2,
    } as unknown as EmailListItem;
    expect(emailStatusParts(email)[0].label).toBe("Live v2 · 8/30d");
    expect(emailStatusParts(email, [], { sendsInLabel: false })[0].label).toBe("Live v2");
  });
});
