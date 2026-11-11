import { describe, expect, it } from "vitest";

import type { EmailActivityItem } from "./types";
import { changesSinceVisit, visitSince } from "./visit";

const now = new Date("2026-11-11T12:00:00Z");

describe("visitSince", () => {
  it("takes the last open when this one starts a new visit", () => {
    expect(visitSince("2026-11-11T10:00:00Z", "2026-11-10T09:00:00Z", now)).toBe("2026-11-11T10:00:00Z");
  });

  it("keeps the visit before while this one goes on", () => {
    expect(visitSince("2026-11-11T11:50:00Z", "2026-11-10T09:00:00Z", now)).toBe("2026-11-10T09:00:00Z");
  });

  it("has no visit before on a first open", () => {
    expect(visitSince(null, null, now)).toBeNull();
    expect(visitSince("2026-11-11T11:50:00Z", null, now)).toBeNull();
  });
});

function edit(fields: Partial<EmailActivityItem>): EmailActivityItem {
  return {
    id: "e",
    type: "email_updated",
    actor_email: "reviewer@example.com",
    created_at: "2026-11-10T17:32:00Z",
    summary: "",
    metadata: {},
    changes: {},
    ...fields,
  };
}

describe("changesSinceVisit", () => {
  const activities = [
    edit({ changes: { subject: {} }, metadata: { changed_review_blocks: ["subject"] } }),
    edit({ created_at: "2026-11-10T17:34:00+03:00", changes: { editable_fields: { step_2: {} } } }),
    edit({ actor_email: "Me@example.com", changes: { preheader: {} } }),
    edit({ created_at: "2026-11-10T10:00:00Z", changes: { title: {} } }),
    edit({ type: "comment_created", changes: { body: {} } }),
  ];

  it("sums up others' edits after the visit", () => {
    expect(changesSinceVisit(activities, "2026-11-10T12:00:00Z", "me@example.com")).toEqual({
      since: "2026-11-10T12:00:00Z",
      fields: ["Subject", "Step 2"],
      blocks: ["subject"],
      people: ["reviewer@example.com"],
    });
  });

  it("returns nothing without a visit before or edits since", () => {
    expect(changesSinceVisit(activities, null, "me@example.com")).toBeNull();
    expect(changesSinceVisit(activities, "2026-11-10T18:00:00Z", "me@example.com")).toBeNull();
  });
});
