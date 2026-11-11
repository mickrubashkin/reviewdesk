import { describe, expect, it } from "vitest";

import { masterInReview } from "./masterInReview";

const master = { id: "en", review_status: "draft" as const, open_comment_count: 2 };

describe("masterInReview", () => {
  it("returns a draft master with open comments", () => {
    expect(masterInReview({ translation_of: "en" }, [master])).toBe(master);
  });

  it("lets a translation go when EN is ready, even with comments left", () => {
    expect(masterInReview({ translation_of: "en" }, [{ ...master, review_status: "ready" as const }])).toBeNull();
  });

  it("lets a translation go when nobody comments on the EN draft", () => {
    expect(masterInReview({ translation_of: "en" }, [{ ...master, open_comment_count: 0 }])).toBeNull();
  });

  it("returns nothing for an email that is not a translation or whose master is not listed", () => {
    expect(masterInReview({ translation_of: null }, [master])).toBeNull();
    expect(masterInReview({ translation_of: "archived" }, [master])).toBeNull();
  });
});
