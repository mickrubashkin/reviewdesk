import GithubSlugger from "github-slugger";

import type { HelpPage } from "./help";

// A HelpSection is one heading of a help page and the text under it, up to
// the next heading. The page's own title is a section too, with no anchor.
export type HelpSection = {
  slug: string;
  pageTitle: string;
  heading: string;
  // The id rehype-slug gives the heading on the page; "" for the title.
  anchor: string;
  text: string;
};

export type HelpSearchResult = {
  section: HelpSection;
  // Where the section is in the app: "/help/comments#mention-someone".
  href: string;
  snippet: string;
};

// buildHelpSections splits every page at its headings.
export function buildHelpSections(pages: HelpPage[]): HelpSection[] {
  const sections: HelpSection[] = [];
  for (const page of pages) {
    const slugger = new GithubSlugger();
    let current: HelpSection = { slug: page.slug, pageTitle: page.title, heading: page.title, anchor: "", text: "" };
    const lines: string[] = [];
    const close = () => {
      sections.push({ ...current, text: plainText(lines.join("\n")) });
      lines.length = 0;
    };
    for (const line of page.markdown.split("\n")) {
      const heading = line.match(/^(#+)\s+(.+)$/);
      if (!heading) {
        lines.push(line);
        continue;
      }
      const text = plainText(heading[2]);
      const anchor = slugger.slug(text);
      if (heading[1].length === 1) {
        // The title: its section is the page itself.
        continue;
      }
      close();
      current = { slug: page.slug, pageTitle: page.title, heading: text, anchor, text: "" };
    }
    close();
  }
  return sections.filter((section) => section.anchor !== "" || section.text !== "");
}

// plainText drops the Markdown markup the help uses: links, emphasis, code,
// list markers and table pipes.
function plainText(markdown: string): string {
  return markdown
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/[*`]/g, "")
    .replace(/^\s*(?:[-+]|\d+\.)\s+/gm, "")
    .replace(/^\s*\|?[\s:|-]+\|[\s:|-]*$/gm, "")
    .replace(/\|/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

// helpSearchTerms splits a query into lowercase words.
export function helpSearchTerms(query: string): string[] {
  return query.toLowerCase().split(/\s+/).filter(Boolean);
}

// searchHelp finds the sections that hold every word of the query, best
// first: words in a heading count more than words in the text, and the whole
// query as written counts extra. A word may be
// in the page's title instead ("regions reference" finds the reference part
// of Regions), but one of them must be in the section itself, or every
// section of a page would match its title.
export function searchHelp(sections: HelpSection[], query: string, limit = 8): HelpSearchResult[] {
  const terms = helpSearchTerms(query);
  if (terms.length === 0) {
    return [];
  }
  const phrase = terms.join(" ");
  const scored: { section: HelpSection; score: number; order: number }[] = [];
  sections.forEach((section, order) => {
    const heading = section.heading.toLowerCase();
    const pageTitle = section.anchor ? section.pageTitle.toLowerCase() : "";
    const text = section.text.toLowerCase();
    let score = (heading.includes(phrase) ? 5 : 0) + (terms.length > 1 && text.includes(phrase) ? 2 : 0);
    let inSection = false;
    for (const term of terms) {
      const own = (heading.includes(term) ? 3 : 0) + (text.includes(term) ? 1 : 0);
      const found = own + (pageTitle.includes(term) ? 1 : 0);
      if (found === 0) {
        return;
      }
      inSection ||= own > 0;
      score += found;
    }
    if (inSection) {
      scored.push({ section, score, order });
    }
  });
  return scored
    .sort((a, b) => b.score - a.score || a.order - b.order)
    .slice(0, limit)
    .map(({ section }) => ({
      section,
      href: `/help${section.slug ? `/${section.slug}` : ""}${section.anchor ? `#${section.anchor}` : ""}`,
      snippet: snippet(section.text, terms),
    }));
}

const snippetLength = 140;

// snippet cuts the text around the first word of the query found in it, or
// takes its start.
function snippet(text: string, terms: string[]): string {
  const lower = text.toLowerCase();
  const hits = terms.map((term) => lower.indexOf(term)).filter((at) => at !== -1);
  const hit = hits.length > 0 ? Math.min(...hits) : 0;
  let start = Math.max(0, hit - 40);
  if (start > 0) {
    // Start at a word.
    const space = text.indexOf(" ", start);
    start = space !== -1 && space < hit ? space + 1 : start;
  }
  let end = Math.min(text.length, start + snippetLength);
  if (end < text.length) {
    const space = text.lastIndexOf(" ", end);
    end = space > hit ? space : end;
  }
  return (start > 0 ? "…" : "") + text.slice(start, end) + (end < text.length ? "…" : "");
}
