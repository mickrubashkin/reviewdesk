import { describe, expect, it } from "vitest";

import { normalizeSendTiming, sendTimingOptions } from "./sendTiming";

describe("sendTimingOptions", () => {
  it("lists each timing once, shortest delay first", () => {
    expect(
      sendTimingOptions([
        "+10 days",
        "immediately",
        "+ 3 day",
        "+3 days",
        "Immediately",
        "2 days after previous email",
        "+2 days",
        "+21 day",
        "+1 day",
        null,
        "",
        "after the call",
      ])
    ).toEqual([
      "Immediately",
      "+1 day",
      "+2 days",
      "2 days after previous email",
      "+3 days",
      "+10 days",
      "+21 days",
      "after the call",
    ]);
  });
});

describe("normalizeSendTiming", () => {
  it("keeps timings it does not know as typed", () => {
    expect(normalizeSendTiming("  on   Monday ")).toBe("on Monday");
    expect(normalizeSendTiming("1 days after previous email")).toBe("1 day after previous email");
  });
});
