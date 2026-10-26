import { describe, expect, it } from "vitest";

import type { EmailListItem } from "../emails/types";
import { findHeroEmail, guideSteps } from "./demoGuideSteps";

function email(overrides: Partial<EmailListItem>): EmailListItem {
  return {
    id: "hero",
    sequence: "demo-visit-1",
    title: "Complete Setup",
    stage: "activation",
    language: "en",
    review_status: "draft",
    open_comment_count: 1,
    open_blocking_comment_count: 1,
    latest_version: 1,
    ...overrides,
  } as EmailListItem;
}

describe("findHeroEmail", () => {
  it("picks the EN Complete Setup email, not its translation", () => {
    const hero = email({ id: "en" });
    const emails = [email({ id: "de", language: "de" }), email({ id: "other", title: "Connect Data" }), hero];
    expect(findHeroEmail(emails)).toBe(hero);
  });

  it("finds nothing on a board without the scenario", () => {
    expect(findHeroEmail([email({ title: "Welcome", stage: "signup" })])).toBeUndefined();
  });
});

describe("guideSteps", () => {
  const done = (steps: ReturnType<typeof guideSteps>) =>
    steps.filter((step) => step.done).map((step) => step.key);

  it("starts with nothing done on the seeded hero", () => {
    expect(done(guideSteps(email({}), false))).toEqual([]);
  });

  it("ticks steps off from the hero's state, in any order", () => {
    expect(done(guideSteps(email({ open_blocking_comment_count: 0 }), true))).toEqual(["open", "resolve"]);
    expect(
      done(
        guideSteps(email({ latest_version: 2, open_blocking_comment_count: 0, review_status: "ready" }), true)
      )
    ).toEqual(["open", "edit", "resolve", "ready"]);
  });
});
