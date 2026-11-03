import { describe, expect, it } from "vitest";

import type { EmailSends, SentText, SlotVersionSends } from "./api";
import { buildSendsChart, earlierKey, periodDays, seriesTotals, versionLabel } from "./emailSends";

function text(id: string, firstSentAt: string): SentText {
  return {
    group_id: id,
    board_key: "onboarding",
    subject: "Welcome",
    first_sent_at: firstSentAt,
    last_sent_at: firstSentAt,
    sends: 0,
  };
}

describe("periodDays", () => {
  it("lists every day of the period, across a month end", () => {
    expect(periodDays("2026-10-30", "2026-11-02")).toEqual(["2026-10-30", "2026-10-31", "2026-11-01", "2026-11-02"]);
  });
});

const noStagesOrDeals = { stages: [], deals: { deals: 0, repeated: 0, doubled: 0 }, versions: [] };

describe("buildSendsChart", () => {
  it("fills days without sends with zeros, one column per text", () => {
    const sends: EmailSends = {
      days: 3,
      since: "2026-10-18",
      until: "2026-10-20",
      texts: [text("old", "2026-07-01T09:00:00Z"), text("new", "2026-10-19T09:00:00Z")],
      daily: [
        { date: "2026-10-18", group_id: "old", sends: 4 },
        { date: "2026-10-19", group_id: "old", sends: 1 },
        { date: "2026-10-19", group_id: "new", sends: 2 },
      ],
      ...noStagesOrDeals,
    };
    const chart = buildSendsChart(sends);
    expect(chart.series.map((series) => series.key)).toEqual(["old", "new"]);
    expect(chart.rows).toEqual([
      { date: "2026-10-18", old: 4, new: 0 },
      { date: "2026-10-19", old: 1, new: 2 },
      { date: "2026-10-20", old: 0, new: 0 },
    ]);
  });

  it("keeps the two newest of more than three texts and folds the rest", () => {
    const sends: EmailSends = {
      days: 1,
      since: "2026-10-20",
      until: "2026-10-20",
      texts: ["a", "b", "c", "d"].map((id, index) => text(id, `2026-0${index + 1}-01T09:00:00Z`)),
      daily: [
        { date: "2026-10-20", group_id: "a", sends: 1 },
        { date: "2026-10-20", group_id: "b", sends: 2 },
        { date: "2026-10-20", group_id: "d", sends: 5 },
      ],
      ...noStagesOrDeals,
    };
    const chart = buildSendsChart(sends);
    expect(chart.series.map((series) => series.key)).toEqual([earlierKey, "c", "d"]);
    expect(chart.series[0].texts.map((t) => t.group_id)).toEqual(["a", "b"]);
    expect(chart.rows).toEqual([{ date: "2026-10-20", [earlierKey]: 3, c: 0, d: 5 }]);
  });

  it("has no series when nothing was sent", () => {
    const chart = buildSendsChart({
      days: 2,
      since: "2026-10-19",
      until: "2026-10-20",
      texts: [],
      daily: [],
      ...noStagesOrDeals,
    });
    expect(chart.series).toEqual([]);
    expect(chart.rows).toHaveLength(2);
  });
});

describe("seriesTotals", () => {
  it("sums the folded texts and keeps the latest send", () => {
    const series = {
      key: earlierKey,
      label: "2 earlier texts",
      color: "gray",
      texts: [
        { ...text("a", "2026-01-01T09:00:00Z"), sends: 2, last_sent_at: "2026-09-01T09:00:00Z" },
        { ...text("b", "2026-02-01T09:00:00Z"), sends: 3, last_sent_at: "2026-08-01T09:00:00Z" },
      ],
    };
    expect(seriesTotals(series)).toEqual({ sends: 5, lastSentAt: "2026-09-01T09:00:00Z" });
  });
});

describe("versionLabel", () => {
  const version = (fields: Partial<SlotVersionSends>): SlotVersionSends => ({
    email_id: "1",
    title: "Welcome",
    language: "en",
    variant: "new",
    region_name: "",
    reference: false,
    sends: 0,
    deals: 0,
    last_sent_at: null,
    ...fields,
  });

  it("names versions by language off a board with regions", () => {
    expect(versionLabel(version({}))).toBe("EN · new");
    expect(versionLabel(version({ variant: "old" }))).toBe("EN · old");
  });

  it("names versions by region on a board with regions", () => {
    expect(versionLabel(version({ region_name: "Brazil", language: "br" }))).toBe("Brazil");
    expect(versionLabel(version({ region_name: "Global", reference: true }))).toBe("Global (reference)");
    expect(versionLabel(version({ region_name: "Brazil", variant: "old" }))).toBe("Brazil · old");
  });
});
