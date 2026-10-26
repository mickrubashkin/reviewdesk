import { describe, expect, it } from "vitest";

import type { BoardRegion, EmailListItem, EmailVersionGroup, StageColumn } from "../../../emails/types";
import { buildTimeline, countMatches, formatDay, formatDelay, isHighlight, matchesHighlight, summarizeStage } from "./timeline";

let nextId = 0;

function email(overrides: Partial<EmailListItem>): EmailListItem {
  nextId += 1;
  return {
    id: `email-${nextId}`,
    sequence: "onboarding",
    title: `Email ${nextId}`,
    subject: null,
    preheader: null,
    send_timing: "immediately",
    send_condition: null,
    stage: "registered",
    sort_order: 1,
    language: "en",
    variant: "v1",
    adaptation_key: "default",
    adaptation_label: "Default",
    review_status: "draft",
    owner_email: null,
    reviewer_email: null,
    due_date: null,
    implementation_notes: null,
    open_comment_count: 0,
    open_blocking_comment_count: 0,
    portal_send_count: 0,
    portal_last_sent_at: null,
    translation_of: null,
    translation_stale: false,
    ...overrides,
  } as EmailListItem;
}

function group(sortOrder: number, versions: EmailListItem[]): EmailVersionGroup {
  return { key: `registered#${sortOrder}`, stage: "registered", sortOrder, versions };
}

function column(groups: EmailVersionGroup[]): StageColumn {
  return { stage: "registered", title: "Registered", sortOrder: 1, emailGroups: groups };
}

const region = (key: string, language: string, reference = false): BoardRegion => ({
  key,
  name: key.toUpperCase(),
  language,
  managers: [],
  reference,
  emails: 0,
});

describe("buildTimeline by language", () => {
  it("adds the waits up into days from the stage's entry", () => {
    const stage = column([
      group(1, [email({ send_timing: "immediately" })]),
      group(2, [email({ send_timing: "2 days after previous email" })]),
      group(3, [email({ send_timing: "5 days after previous email" })]),
    ]);

    const [{ branches }] = buildTimeline([stage], { kind: "language", language: "en" });

    expect(branches).toHaveLength(1);
    expect(branches[0].cells.map((cell) => cell?.day)).toEqual([0, 2, 7]);
    expect(branches[0].cells.map((cell) => cell?.delay)).toEqual([0, 2, 5]);
  });

  it("stops counting days after a timing it cannot read", () => {
    const stage = column([
      group(1, [email({ send_timing: "immediately" })]),
      group(2, [email({ send_timing: "when the moon is full" })]),
      group(3, [email({ send_timing: "3 days after previous email" })]),
    ]);

    const [{ branches }] = buildTimeline([stage], { kind: "language", language: "en" });

    expect(branches[0].cells.map((cell) => cell?.day)).toEqual([0, null, null]);
  });

  it("counts the slots without an email in the language and leaves no gaps", () => {
    const stage = column([
      group(1, [email({ language: "en" }), email({ language: "de" })]),
      group(2, [email({ language: "en" })]),
    ]);

    const [{ branches, missing }] = buildTimeline([stage], { kind: "language", language: "de" });

    expect(branches[0].cells).toHaveLength(1);
    expect(missing).toBe(1);
  });

  it("has no branch when the stage has nothing in the language", () => {
    const stage = column([group(1, [email({ language: "en" })])]);

    const [{ branches }] = buildTimeline([stage], { kind: "language", language: "pl" });

    expect(branches).toEqual([]);
  });
});

describe("buildTimeline by region", () => {
  const regions = [region("de", "de"), region("reference", "en", true), region("pl", "pl")];

  it("puts the reference first and lines the branches up slot by slot", () => {
    const stage = column([
      group(1, [email({ region_key: "reference" }), email({ region_key: "de", language: "de" })]),
      group(2, [email({ region_key: "reference", send_timing: "2 days after previous email" })]),
    ]);

    const [{ branches }] = buildTimeline([stage], { kind: "region", regions });

    expect(branches.map((branch) => branch.key)).toEqual(["reference", "de"]);
    expect(branches[0].reference).toBe(true);
    expect(branches[0].cells.map((cell) => cell !== null)).toEqual([true, true]);
    // The slot the region has no email for stays a gap.
    expect(branches[1].cells.map((cell) => cell !== null)).toEqual([true, false]);
  });

  it("drops a slot nobody has an email for", () => {
    const stage = column([
      group(1, [email({ region_key: "reference" })]),
      group(2, []),
      group(3, [email({ region_key: "reference" })]),
    ]);

    const [{ branches }] = buildTimeline([stage], { kind: "region", regions });

    expect(branches[0].cells).toHaveLength(2);
  });

  it("marks a wait that differs from the reference's", () => {
    const stage = column([
      group(1, [
        email({ region_key: "reference", send_timing: "immediately" }),
        email({ region_key: "de", language: "de", send_timing: "2 days after previous email" }),
        email({ region_key: "pl", language: "pl", send_timing: "immediately" }),
      ]),
    ]);

    const [{ branches }] = buildTimeline([stage], { kind: "region", regions });
    const byKey = Object.fromEntries(branches.map((branch) => [branch.key, branch.cells[0]]));

    expect(byKey.de?.referenceDelay).toBe(0);
    expect(byKey.pl?.referenceDelay).toBeUndefined();
    expect(byKey.reference?.referenceDelay).toBeUndefined();
  });

  it("does not mark a wait that cannot be read", () => {
    const stage = column([
      group(1, [
        email({ region_key: "reference", send_timing: "immediately" }),
        email({ region_key: "de", language: "de", send_timing: "soon" }),
      ]),
    ]);

    const [{ branches }] = buildTimeline([stage], { kind: "region", regions });

    expect(branches.find((branch) => branch.key === "de")?.cells[0]?.referenceDelay).toBeUndefined();
  });

  it("gives the emails without a region a branch of their own, last", () => {
    const stage = column([
      group(1, [email({ region_key: "reference" }), email({ region_key: null }), email({ region_key: "gone" })]),
    ]);

    const [{ branches }] = buildTimeline([stage], { kind: "region", regions });

    expect(branches.map((branch) => branch.key)).toEqual(["reference", ""]);
    expect(branches[1].name).toBe("No region");
  });

  it("counts days per branch", () => {
    const stage = column([
      group(1, [email({ region_key: "reference" }), email({ region_key: "de", language: "de" })]),
      group(2, [
        email({ region_key: "reference", send_timing: "2 days after previous email" }),
        email({ region_key: "de", language: "de", send_timing: "5 days after previous email" }),
      ]),
    ]);

    const [{ branches }] = buildTimeline([stage], { kind: "region", regions });

    expect(branches.find((b) => b.key === "reference")?.cells.map((c) => c?.day)).toEqual([0, 2]);
    expect(branches.find((b) => b.key === "de")?.cells.map((c) => c?.day)).toEqual([0, 5]);
  });
});

describe("summarizeStage", () => {
  it("counts live emails, open comments and the last day over all branches", () => {
    const stage = column([
      group(1, [
        email({ region_key: "reference", production_status: "live", open_comment_count: 2 }),
        email({ region_key: "de", language: "de", production_status: "not_seen" }),
      ]),
      group(2, [
        email({ region_key: "reference", send_timing: "5 days after previous email", open_blocking_comment_count: 1, open_comment_count: 1 }),
      ]),
    ]);
    const regions = [region("reference", "en", true), region("de", "de")];

    const [built] = buildTimeline([stage], { kind: "region", regions });
    const summary = summarizeStage(built);

    expect(summary).toMatchObject({ emails: 3, live: 1, openComments: 3, blockingComments: 1, lastDay: 5, branches: 2 });
  });

  it("has no last day when no branch knows it", () => {
    const stage = column([group(1, [email({ send_timing: "whenever" })]), group(2, [email({ send_timing: "later" })])]);

    const [built] = buildTimeline([stage], { kind: "language", language: "en" });

    expect(summarizeStage(built).lastDay).toBeNull();
  });
});

describe("labels", () => {
  it("writes the waits", () => {
    expect(formatDelay(null)).toBe("?");
    expect(formatDelay(0)).toBe("right away");
    expect(formatDelay(0.5)).toBe("+12 h");
    expect(formatDelay(2)).toBe("+2 d");
    expect(formatDelay(1.5)).toBe("+1.5 d");
  });

  it("writes the days", () => {
    expect(formatDay(null)).toBe("?");
    expect(formatDay(7)).toBe("7");
    expect(formatDay(1.5)).toBe("1.5");
  });
});

describe("highlights", () => {
  it("lights up everything when nothing is chosen", () => {
    expect(matchesHighlight(email({}), "none")).toBe(true);
  });

  it("finds emails with open comments", () => {
    expect(matchesHighlight(email({ open_comment_count: 1 }), "comments")).toBe(true);
    expect(matchesHighlight(email({ open_comment_count: 0 }), "comments")).toBe(false);
  });

  it("finds emails that are not live", () => {
    expect(matchesHighlight(email({ production_status: "live" }), "not_live")).toBe(false);
    expect(matchesHighlight(email({ production_status: "live_manual" }), "not_live")).toBe(false);
    expect(matchesHighlight(email({ production_status: "not_seen" }), "not_live")).toBe(true);
  });

  it("finds emails whose reference or master changed", () => {
    expect(matchesHighlight(email({ based_on_stale: true }), "outdated")).toBe(true);
    expect(matchesHighlight(email({ translation_stale: true }), "outdated")).toBe(true);
    expect(matchesHighlight(email({}), "outdated")).toBe(false);
  });

  it("finds ready emails a robot does not have yet", () => {
    // Drafts never need an upload.
    expect(matchesHighlight(email({ review_status: "draft" }), "to_upload")).toBe(false);
    expect(matchesHighlight(email({ review_status: "ready" }), "to_upload")).toBe(true);
  });

  it("counts the emails a highlight lights up", () => {
    const stage = column([
      group(1, [email({ open_comment_count: 2 })]),
      group(2, [email({ open_comment_count: 0 })]),
      group(3, [email({ open_comment_count: 1 })]),
    ]);
    const stages = buildTimeline([stage], { kind: "language", language: "en" });

    expect(countMatches(stages, "comments")).toEqual({ matching: 2, total: 3 });
    expect(countMatches(stages, "none")).toEqual({ matching: 3, total: 3 });
  });

  it("knows the options it offers", () => {
    expect(isHighlight("to_upload")).toBe(true);
    expect(isHighlight("everything")).toBe(false);
    expect(isHighlight(null)).toBe(false);
  });
});
