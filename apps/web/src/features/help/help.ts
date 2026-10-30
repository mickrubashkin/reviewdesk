import type { AuthUser } from "../emails/types";

export type HelpPage = {
  // Path under /help; "" is the index (README.md).
  slug: string;
  title: string;
  markdown: string;
};

// buildHelpPages turns the Markdown files of docs/help/<lang>/, keyed by
// path, into pages: the index first, then the pages in the order README's
// "All task pages" lists them, then any page it leaves out.
export function buildHelpPages(sources: Record<string, string>): HelpPage[] {
  const pages = Object.entries(sources).map(([path, markdown]) => {
    const name = path.split("/").pop()!.replace(/\.md$/, "");
    const slug = name === "README" ? "" : name;
    const title = markdown.match(/^#\s+(.+)$/m)?.[1].trim() ?? name;
    return { slug, title, markdown };
  });
  const index = pages.find((page) => page.slug === "");
  const listed = index ? listedSlugs(index.markdown) : [];
  const rank = (page: HelpPage) => {
    if (page.slug === "") return -1;
    const at = listed.indexOf(page.slug);
    return at === -1 ? listed.length : at;
  };
  return pages.sort((a, b) => rank(a) - rank(b) || a.slug.localeCompare(b.slug));
}

// listedSlugs reads the page links under README's "All task pages".
function listedSlugs(readme: string): string[] {
  const section = readme.split(/^## All task pages\s*$/m)[1] ?? "";
  return [...section.matchAll(/\]\(([\w-]+)\.md\)/g)].map((match) => match[1]);
}

// helpHref maps a link in the Markdown to the app: "editing.md#x" opens
// /help/editing#x and "README.md" the index. Anchors and outside links stay
// as they are.
export function helpHref(href: string): string {
  const page = href.match(/^(?:\.\/)?([\w-]+)\.md(#.*)?$/);
  if (!page) {
    return href;
  }
  const slug = page[1] === "README" ? "" : page[1];
  return `/help${slug ? `/${slug}` : ""}${page[2] ?? ""}`;
}

// helpTopics are the parts of the help that the "?" next to a hard part of
// the app opens: a page, or a page and the slug of one of its headings.
export const helpTopics = {
  statusLine: { path: "find-an-email#read-the-status-line", label: "How to read the status line" },
  regions: { path: "regions", label: "Regions and the reference" },
  reference: { path: "regions#when-the-reference-changed", label: "When the reference changed" },
  timeline: { path: "board-views#timeline-default", label: "The timeline, Highlight and Copy link" },
} as const;

export type HelpTopic = keyof typeof helpTopics;

export function isExternalHref(href: string) {
  return /^[a-z][a-z0-9+.-]*:/i.test(href) || href.startsWith("//");
}

// helpRole names the part of the help written for this person, as README's
// "Who are you?" calls the roles. Someone who only translates reads the
// region manager's part.
export function helpRole(user: Pick<AuthUser, "role" | "languages" | "managed_boards">): {
  label: string;
  anchor: string;
} {
  if (user.role === "super_admin") return { label: "CRM operator", anchor: "crm-operator" };
  if (user.role === "admin") return { label: "Sequence owner", anchor: "sequence-owner" };
  if ((user.managed_boards ?? []).length > 0) return { label: "Region manager", anchor: "region-manager" };
  if ((user.languages ?? []).length > 0) return { label: "Translator", anchor: "region-manager" };
  return { label: "Viewer", anchor: "viewer" };
}
