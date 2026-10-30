import { describe, expect, it } from "vitest";

import { shouldStartTour, TOUR_VERSION, tourSteps } from "./tour";

describe("shouldStartTour", () => {
  it("starts once per version, never in the demo", () => {
    expect(shouldStartTour({ role: "reviewer" }, false)).toBe(true);
    expect(shouldStartTour({ role: "reviewer", tour_version: TOUR_VERSION - 1 }, false)).toBe(true);
    expect(shouldStartTour({ role: "reviewer", tour_version: TOUR_VERSION }, false)).toBe(false);
    expect(shouldStartTour({ role: "reviewer", tour_version: 0 }, true)).toBe(false);
  });
});

describe("tourSteps", () => {
  const targets = (steps: ReturnType<typeof tourSteps>) => steps.map((step) => step.target ?? "");

  it("skips New for someone who only reads", () => {
    const steps = tourSteps({ role: "reviewer" });
    expect(targets(steps)).toEqual(["", "board", "views", "my-tasks", "account"]);
    expect(steps[0].description).toContain("Viewer");
  });

  it("shows New to people who edit, and ends on the account menu", () => {
    expect(targets(tourSteps({ role: "reviewer", managed_boards: ["a"] }))).toContain("new");
    const owner = tourSteps({ role: "admin" });
    expect(owner.find((step) => step.target === "new")?.description).toContain("new board");
    expect(owner.at(-1)?.target).toBe("account");
  });

  it("tells the CRM operator about the upload queue", () => {
    const steps = tourSteps({ role: "super_admin" });
    expect(steps.find((step) => step.target === "my-tasks")?.description).toContain("Bitrix24");
  });
});
