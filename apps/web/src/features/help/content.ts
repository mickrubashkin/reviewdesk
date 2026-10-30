import { buildHelpPages } from "./help";
import { buildHelpSections } from "./helpSearch";

// The help is the Markdown in docs/help/en/, built into the app; vite.config
// lets the dev server read it.
const sources = import.meta.glob("../../../../../docs/help/en/*.md", {
  eager: true,
  import: "default",
  query: "?raw",
}) as Record<string, string>;

export const helpPages = buildHelpPages(sources);

export const helpSections = buildHelpSections(helpPages);
