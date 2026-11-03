import { describe, expect, it } from "vitest";

import { needsBitrixUpload, uploadTodo } from "./reviewStatus";

const robot = { robot: "", uploaded_at: null, uploaded_by_email: null };

describe("needsBitrixUpload", () => {
  it("asks to upload a ready email no robot has", () => {
    const email = { review_status: "ready" as const, latest_version: 2, upload_robots: [{ ...robot, version: null }] };
    expect(needsBitrixUpload(email)).toBe(true);
    expect(uploadTodo(email)?.label).toBe("to upload");
  });

  it("asks to re-upload when a robot has an older version", () => {
    expect(needsBitrixUpload({ review_status: "ready", latest_version: 2, upload_robots: [{ ...robot, version: 1 }] })).toBe(true);
  });

  it("never asks for a draft", () => {
    expect(needsBitrixUpload({ review_status: "draft", latest_version: 2, upload_robots: [{ ...robot, version: null }] })).toBe(false);
  });

  it("does not ask for an email marked live by hand", () => {
    const email = {
      review_status: "ready" as const,
      latest_version: 2,
      upload_robots: [{ ...robot, version: null }],
      live_marked_at: "2026-11-03T08:00:00Z",
    };
    expect(needsBitrixUpload(email)).toBe(false);
    expect(uploadTodo(email)).toBeNull();
  });
});
