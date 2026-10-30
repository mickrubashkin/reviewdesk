import { describe, expect, it } from "vitest";

import { helpPages } from "./content";
import type { HelpPage } from "./help";
import { buildHelpSections, helpSearchTerms, searchHelp } from "./helpSearch";

const pages: HelpPage[] = [
  {
    slug: "comments",
    title: "Comment and mention a colleague",
    markdown: `# Comment and mention a colleague

Anyone who can sign in can comment on any email.

## Comment on a piece of text

1. Open the email.
2. Click **Comment** in the [toolbar](editing.md#toolbar).

## Mention someone

Type \`@\` and a name.

## Mention someone
`,
  },
  {
    slug: "glossary",
    title: "Glossary",
    markdown: `# Glossary

| Term | Meaning |
| --- | --- |
| Slot | A place in the sequence |
`,
  },
];

describe("buildHelpSections", () => {
  it("splits pages at headings, with the anchors rehype-slug gives them", () => {
    const sections = buildHelpSections(pages);
    expect(sections.map((section) => [section.slug, section.anchor, section.heading])).toEqual([
      ["comments", "", "Comment and mention a colleague"],
      ["comments", "comment-on-a-piece-of-text", "Comment on a piece of text"],
      ["comments", "mention-someone", "Mention someone"],
      ["comments", "mention-someone-1", "Mention someone"],
      ["glossary", "", "Glossary"],
    ]);
  });

  it("keeps the text without Markdown", () => {
    const sections = buildHelpSections(pages);
    expect(sections[1].text).toBe("Open the email. Click Comment in the toolbar.");
    expect(sections[2].text).toBe("Type @ and a name.");
    expect(sections[4].text).toBe("Term Meaning Slot A place in the sequence");
  });

  it("builds every real page", () => {
    const sections = buildHelpSections(helpPages);
    for (const page of helpPages) {
      expect(sections.some((section) => section.slug === page.slug), page.slug).toBe(true);
    }
  });
});

describe("searchHelp", () => {
  const sections = buildHelpSections(pages);

  it("needs every word, and ranks headings first", () => {
    expect(searchHelp(sections, "mention").map((result) => result.href)).toEqual([
      "/help/comments#mention-someone",
      "/help/comments#mention-someone-1",
      "/help/comments",
    ]);
    expect(searchHelp(sections, "comment toolbar").map((result) => result.href)).toEqual([
      "/help/comments#comment-on-a-piece-of-text",
    ]);
    expect(searchHelp(sections, "comment nothing")).toEqual([]);
  });

  it("takes a word from the page title, but not the title alone", () => {
    expect(searchHelp(sections, "colleague name").map((result) => result.href)).toEqual([
      "/help/comments#mention-someone",
    ]);
    expect(searchHelp(sections, "colleague").map((result) => result.href)).toEqual(["/help/comments"]);
  });

  it("ranks the words as written above the words apart", () => {
    const apart: HelpPage = { slug: "apart", title: "Apart", markdown: "# Apart\n\n## One\n\nlink to copy\n\n## Two\n\nCopy link here" };
    expect(searchHelp(buildHelpSections([apart]), "copy link").map((result) => result.href)).toEqual([
      "/help/apart#two",
      "/help/apart#one",
    ]);
  });

  it("ignores case and empty queries", () => {
    expect(searchHelp(sections, "SLOT")[0].href).toBe("/help/glossary");
    expect(searchHelp(sections, "   ")).toEqual([]);
    expect(helpSearchTerms(" Mention  Someone ")).toEqual(["mention", "someone"]);
  });

  it("cuts a snippet around the match", () => {
    const long: HelpPage = {
      slug: "long",
      title: "Long",
      markdown: `# Long\n\n${"word ".repeat(60)}needle ${"word ".repeat(60)}`,
    };
    const [result] = searchHelp(buildHelpSections([long]), "needle");
    expect(result.snippet.startsWith("…word")).toBe(true);
    expect(result.snippet.endsWith("…")).toBe(true);
    expect(result.snippet).toContain("needle");
    expect(result.snippet.length).toBeLessThanOrEqual(142);
  });
});
