import GithubSlugger from "github-slugger";
import { describe, expect, it } from "vitest";

import { helpPages } from "./content";
import { buildHelpPages, helpHref, helpRole, helpTopics, isExternalHref } from "./help";

const readme = `# ReviewDesk help

See [comments](comments.md) first.

## All task pages

- [Find an email](find-an-email.md)
- [Comments](comments.md)
`;

describe("buildHelpPages", () => {
  it("puts the index first, then README's list, then the rest", () => {
    const pages = buildHelpPages({
      "../docs/help/en/zeta.md": "# Zeta",
      "../docs/help/en/comments.md": "# Comment and mention\n\ntext",
      "../docs/help/en/README.md": readme,
      "../docs/help/en/alpha.md": "no heading",
      "../docs/help/en/find-an-email.md": "# Find an email",
    });
    expect(pages.map((page) => page.slug)).toEqual(["", "find-an-email", "comments", "alpha", "zeta"]);
    expect(pages[0].title).toBe("ReviewDesk help");
    expect(pages[2].title).toBe("Comment and mention");
    expect(pages[3].title).toBe("alpha");
  });
});

describe("helpHref", () => {
  it("maps page links to /help", () => {
    expect(helpHref("editing.md")).toBe("/help/editing");
    expect(helpHref("./editing.md#mark-the-email-ready")).toBe("/help/editing#mark-the-email-ready");
    expect(helpHref("README.md")).toBe("/help");
    expect(helpHref("README.md#viewer")).toBe("/help#viewer");
  });

  it("leaves anchors and outside links alone", () => {
    expect(helpHref("#viewer")).toBe("#viewer");
    expect(helpHref("https://example.com/a.md")).toBe("https://example.com/a.md");
    expect(isExternalHref("https://example.com")).toBe(true);
    expect(isExternalHref("mailto:a@example.com")).toBe(true);
    expect(isExternalHref("/help/editing")).toBe(false);
    expect(isExternalHref("#viewer")).toBe(false);
  });
});

describe("helpRole", () => {
  it("maps the account to the help's roles", () => {
    expect(helpRole({ role: "super_admin" }).anchor).toBe("crm-operator");
    expect(helpRole({ role: "admin", managed_boards: ["a"] }).anchor).toBe("board-owner");
    expect(helpRole({ role: "reviewer", managed_boards: ["a"], languages: ["de"] }).label).toBe("Region manager");
    expect(helpRole({ role: "reviewer", languages: ["de"] })).toEqual({ label: "Translator", anchor: "region-manager" });
    expect(helpRole({ role: "reviewer", managed_boards: [], languages: null as unknown as string[] }).anchor).toBe("viewer");
  });
});

describe("helpTopics", () => {
  it("point at pages and headings that exist", () => {
    const pages = new Map(helpPages.map((page) => [page.slug, page.markdown]));
    for (const { path } of Object.values(helpTopics)) {
      const [slug, anchor] = path.split("#");
      const markdown = pages.get(slug);
      expect(markdown, path).toBeDefined();
      if (anchor) {
        // The same slugs rehype-slug gives the headings on the page.
        const slugger = new GithubSlugger();
        const anchors = [...markdown!.matchAll(/^#+\s+(.+)$/gm)].map((match) => slugger.slug(match[1]));
        expect(anchors, path).toContain(anchor);
      }
    }
  });
});
