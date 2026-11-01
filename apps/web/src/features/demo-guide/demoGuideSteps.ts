import type { EmailListItem } from "../emails/types";

// The demo scenario's hero: the EN "Complete Setup" email, seeded with a
// comment on one paragraph (apps/api/internal/demo/seed.go).
export function findHeroEmail(emails: EmailListItem[]): EmailListItem | undefined {
  return emails.find(
    (email) =>
      email.title === "Complete Setup" && email.language === "en" && email.stage === "activation"
  );
}

export type GuideStepKey = "open" | "ai" | "edit" | "resolve" | "ready";

export type GuideStep = {
  key: GuideStepKey;
  done: boolean;
};

// guideSteps ticks each step off from the hero's state, so the guide follows
// what the visitor actually did, in any order.
export function guideSteps(hero: EmailListItem, opened: boolean, analyzed: boolean): GuideStep[] {
  return [
    { key: "open", done: opened },
    // The seed leaves the hero without an AI analysis.
    { key: "ai", done: analyzed },
    // The seed makes v1; saving edited text makes a new version.
    { key: "edit", done: (hero.latest_version ?? 1) > 1 },
    { key: "resolve", done: hero.open_comment_count === 0 },
    { key: "ready", done: hero.review_status === "ready" },
  ];
}
