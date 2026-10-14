export const markdownSectionOptions = [
  { label: "Metadata (stage, timing, language, status, owner, dates)", value: "metadata" },
  { label: "Email copy (subject, preheader, banner, body text)", value: "copy" },
  { label: "CTA and links", value: "links" },
  { label: "Implementation notes", value: "notes" },
  { label: "Review comments", value: "comments" },
];

export const allMarkdownSections = markdownSectionOptions.map(
  (option) => option.value
);
