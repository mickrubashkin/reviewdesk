import { describe, expect, it } from "vitest";

import { suggestionFromSelection } from "./suggestion";

describe("suggestionFromSelection", () => {
  it("takes text picked in an EN email as the term", () => {
    expect(suggestionFromSelection(" Deals ", "en", "e1")).toEqual({
      term: "Deals",
      language: "",
      translation: "",
      note: "",
      email_id: "e1",
    });
  });

  it("takes text picked in a translation as that language's wording", () => {
    expect(suggestionFromSelection("Aufträge", "de", "e2")).toMatchObject({ term: "", language: "de", translation: "Aufträge" });
  });
});
