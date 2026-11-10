import { describe, expect, it } from "vitest";

import { splitDetail, splitTexts } from "./splitTexts";

const text = (id: string, first: string, sends: number) => ({
  group_id: id,
  first_sent_at: first,
  last_sent_at: "2026-11-09T10:00:00Z",
  sends,
});

describe("splitTexts", () => {
  it("is empty for an email sent in one text", () => {
    expect(splitTexts({ portal_split_texts: null })).toEqual([]);
    expect(splitTexts({})).toEqual([]);
    expect(splitTexts({ portal_split_texts: [text("a", "2026-11-01T10:00:00Z", 3)] })).toEqual([]);
  });

  it("keeps the texts of a split email", () => {
    const texts = [text("new", "2026-11-03T10:00:00Z", 12), text("old", "2026-10-20T10:00:00Z", 1)];
    expect(splitTexts({ portal_split_texts: texts })).toEqual(texts);
  });
});

describe("splitDetail", () => {
  it("names each text by its first send and counts its sends", () => {
    const detail = splitDetail([text("new", "2026-11-03T10:00:00Z", 12), text("old", "2026-10-20T10:00:00Z", 1)]);
    expect(detail).toMatch(/^Over the last 14 days robots sent 2 different texts of this email: /);
    expect(detail).toMatch(/Text since [^(]+\(12 sends\), Text since [^(]+\(1 send\)\./);
  });
});
