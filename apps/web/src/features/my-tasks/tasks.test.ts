import { describe, expect, it } from "vitest";

import type { AuthUser, EmailListItem } from "../emails/types";
import { buildTaskSections, countFresh, countTasks } from "./tasks";

const translator: AuthUser = { email: "es@example.com", role: "reviewer", languages: ["es"] };

function email(fields: Partial<EmailListItem>): EmailListItem {
  return {
    language: "en",
    owner_email: null,
    open_comment_count: 0,
    review_status: "draft",
    translation_of: null,
    translation_stale: false,
    ...fields,
  } as EmailListItem;
}

describe("translations waiting for EN", () => {
  const settled = email({ id: "en-settled", review_status: "ready" });
  const inReview = email({ id: "en-review", open_comment_count: 3 });
  const free = email({ id: "es-free", language: "es", translation_of: "en-settled", translation_stale: true });
  const waiting = email({ id: "es-waiting", language: "es", translation_of: "en-review", translation_stale: true });
  const sections = buildTaskSections([waiting, free, settled, inReview], translator);
  const outdated = sections.find((section) => section.key === "outdated");

  it("lists them after the ones to do now", () => {
    expect(outdated?.emails.map((item) => item.id)).toEqual(["es-free", "es-waiting"]);
    expect(outdated?.waiting?.get("es-waiting")?.id).toBe("en-review");
    expect(outdated?.waiting?.has("es-free")).toBe(false);
  });

  it("does not count them", () => {
    expect(countTasks(sections, translator)).toBe(1);
  });
});

describe("tasks new since the visit before", () => {
  const owner: AuthUser = { ...translator, email: "owner@example.com" };
  const master = email({ id: "en", review_status: "ready", changed_by_others_at: "2026-11-10T17:32:00Z" });
  const oldMaster = email({ id: "en-old", review_status: "ready", changed_by_others_at: "2026-11-09T10:00:00Z" });
  const enChanged = email({ id: "es-new", language: "es", translation_of: "en", translation_stale: true });
  const enChangedBefore = email({ id: "es-old", language: "es", translation_of: "en-old", translation_stale: true });
  const draftEdited = email({ id: "draft-edited", owner_email: "owner@example.com", changed_by_others_at: "2026-11-10T18:00:00Z" });
  const draftAssigned = email({ id: "draft-assigned", owner_email: "owner@example.com", assigned_to_me_at: "2026-11-10T19:00:00Z" });
  const draftQuiet = email({ id: "draft-quiet", owner_email: "owner@example.com" });
  const all = [enChangedBefore, enChanged, master, oldMaster, draftQuiet, draftEdited, draftAssigned];

  it("marks what came up after the visit and lists it first", () => {
    const sections = buildTaskSections(all, owner, new Map(), "2026-11-10T12:00:00Z");
    const outdated = sections.find((section) => section.key === "outdated");
    const drafts = sections.find((section) => section.key === "drafts");
    expect(outdated?.emails.map((item) => item.id)).toEqual(["es-new", "es-old"]);
    expect([...(outdated?.fresh ?? [])]).toEqual(["es-new"]);
    expect(drafts?.emails.map((item) => item.id)).toEqual(["draft-edited", "draft-assigned", "draft-quiet"]);
    expect(countFresh(sections, owner)).toBe(3);
  });

  it("marks nothing without a visit before", () => {
    const sections = buildTaskSections(all, owner);
    expect(countFresh(sections, owner)).toBe(0);
    expect(sections.every((section) => !section.fresh)).toBe(true);
  });
});
