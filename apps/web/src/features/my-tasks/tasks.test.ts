import { describe, expect, it } from "vitest";

import type { AuthUser, EmailListItem } from "../emails/types";
import { buildTaskSections, countTasks } from "./tasks";

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
