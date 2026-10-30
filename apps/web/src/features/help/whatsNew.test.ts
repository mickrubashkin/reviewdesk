import { describe, expect, it } from "vitest";

import whatsNewMarkdown from "../../../../../docs/help/en/whats-new.md?raw";
import { hasUnreadNews, latestNewsDate } from "./whatsNew";

describe("latestNewsDate", () => {
  it("takes the first dated heading", () => {
    expect(latestNewsDate("# What's new\n\ntext\n\n## 2026-11-02\n\n- a\n\n## 2026-10-29\n")).toBe("2026-11-02");
    expect(latestNewsDate("# What's new\n\n## Soon\n")).toBe("");
  });

  it("finds a date in the real page, newest first", () => {
    const dates = [...whatsNewMarkdown.matchAll(/^##\s+(.+)$/gm)].map((match) => match[1].trim());
    expect(dates.length).toBeGreaterThan(0);
    for (const date of dates) {
      expect(date).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    }
    expect([...dates].sort().reverse()).toEqual(dates);
  });
});

describe("hasUnreadNews", () => {
  it("is true until the newest entry was opened", () => {
    expect(hasUnreadNews({}, "2026-10-29")).toBe(true);
    expect(hasUnreadNews({ whats_new_seen: "2026-10-28" }, "2026-10-29")).toBe(true);
    expect(hasUnreadNews({ whats_new_seen: "2026-10-29" }, "2026-10-29")).toBe(false);
    expect(hasUnreadNews({}, "")).toBe(false);
  });
});
