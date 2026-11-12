import { describe, expect, it } from "vitest";

import type { UploadSuggestion } from "./api";
import { describeSuggestion, seenRobotsBehind, suggestionFor } from "./uploadSuggestions";

const email = {
  id: "e1",
  latest_version: 2,
  uploaded_version: 1,
  review_status: "ready" as const,
  upload_robots: [
    { robot: "Ann", version: 1, uploaded_at: null, uploaded_by_email: null },
    { robot: "Ben", version: 2, uploaded_at: null, uploaded_by_email: null },
  ],
};

function suggestion(robots: UploadSuggestion["robots"], unknown: number): UploadSuggestion {
  const sends = (robots ?? []).reduce((sum, robot) => sum + robot.sends, unknown);
  return { email_id: "e1", version: 2, sends, last_sent_at: "2026-11-10T10:00:00Z", robots, unknown_sends: unknown };
}

describe("suggestionFor", () => {
  it("keeps a suggestion a lagging robot was seen sending", () => {
    const s = suggestion([{ robot: "Ann", sends: 2 }], 0);
    expect(suggestionFor(new Map([["e1", s]]), email)).toBe(s);
    expect(seenRobotsBehind(s, email)).toEqual(["Ann"]);
  });

  it("drops a suggestion only robots with the version were seen sending", () => {
    const s = suggestion([{ robot: "Ben", sends: 3 }], 0);
    expect(suggestionFor(new Map([["e1", s]]), email)).toBeNull();
  });

  it("keeps a suggestion with sends of unknown robots", () => {
    const s = suggestion([{ robot: "Ben", sends: 3 }], 1);
    expect(suggestionFor(new Map([["e1", s]]), email)).toBe(s);
    expect(seenRobotsBehind(s, email)).toEqual([]);
  });

  it("keeps a suggestion for a robot with no mark while the others have the version", () => {
    const withNewRobot = {
      ...email,
      uploaded_version: 2,
      upload_robots: [
        { robot: "Ben", version: 2, uploaded_at: null, uploaded_by_email: null },
        { robot: "Omar", version: null, uploaded_at: null, uploaded_by_email: null },
      ],
    };
    const s = suggestion([{ robot: "Omar", sends: 4 }], 0);
    expect(suggestionFor(new Map([["e1", s]]), withNewRobot)).toBe(s);
    expect(seenRobotsBehind(s, withNewRobot)).toEqual(["Omar"]);
  });

  it("keeps an older response without robots", () => {
    const s: UploadSuggestion = { email_id: "e1", version: 2, sends: 2, last_sent_at: "2026-11-10T10:00:00Z" };
    expect(suggestionFor(new Map([["e1", s]]), email)).toBe(s);
  });
});

describe("describeSuggestion", () => {
  it("names the robots seen sending", () => {
    expect(describeSuggestion(suggestion([{ robot: "Ann", sends: 2 }], 1))).toMatch(
      /^The portal sent version 2 3 times \(Ann's robot 2 times, unknown robots once\), last on /
    );
  });

  it("stays short when no robot is paired", () => {
    expect(describeSuggestion(suggestion([], 1))).toMatch(/^The portal sent version 2 once, last on /);
  });
});
