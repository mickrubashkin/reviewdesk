import { ActionIcon, Group, Tooltip } from "@mantine/core";
import {
  LinkSimpleIcon,
  ListBulletsIcon,
  ListNumbersIcon,
  TextBIcon,
  TextHOneIcon,
  TextHTwoIcon,
  TextItalicIcon,
} from "@phosphor-icons/react";
import { insertLink, prefixLines, wrapSelection, type MarkdownEdit } from "./markdownEditing";

// MarkdownToolbar adds Markdown marks to the text in a textarea: the field
// renders them as bold, italic, links, headings and lists in the email.
export function MarkdownToolbar({
  disabled,
  getTextarea,
  value,
  onChange,
}: {
  disabled: boolean;
  getTextarea: () => HTMLTextAreaElement | null;
  value: string;
  onChange: (value: string) => void;
}) {
  const apply = (edit: (value: string, start: number, end: number) => MarkdownEdit) => {
    const textarea = getTextarea();
    const start = textarea?.selectionStart ?? value.length;
    const end = textarea?.selectionEnd ?? value.length;
    const next = edit(value, start, end);
    onChange(next.value);
    requestAnimationFrame(() => {
      textarea?.focus();
      textarea?.setSelectionRange(next.start, next.end);
    });
  };

  const actions = [
    { label: "Bold", icon: TextBIcon, edit: (v: string, s: number, e: number) => wrapSelection(v, s, e, "**", "bold text") },
    { label: "Italic", icon: TextItalicIcon, edit: (v: string, s: number, e: number) => wrapSelection(v, s, e, "*", "italic text") },
    { label: "Link", icon: LinkSimpleIcon, edit: insertLink },
    { label: "Heading", icon: TextHOneIcon, edit: (v: string, s: number, e: number) => prefixLines(v, s, e, "# ") },
    { label: "Subheading", icon: TextHTwoIcon, edit: (v: string, s: number, e: number) => prefixLines(v, s, e, "## ") },
    { label: "Bulleted list", icon: ListBulletsIcon, edit: (v: string, s: number, e: number) => prefixLines(v, s, e, "- ") },
    { label: "Numbered list", icon: ListNumbersIcon, edit: (v: string, s: number, e: number) => prefixLines(v, s, e, "numbered") },
  ];

  return (
    <Group gap={2}>
      {actions.map(({ label, icon: Icon, edit }) => (
        <Tooltip key={label} label={label} openDelay={400}>
          <ActionIcon
            aria-label={label}
            disabled={disabled}
            size="sm"
            variant="subtle"
            // Keep the textarea's selection: a click would move focus first.
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => apply(edit)}
          >
            <Icon aria-hidden="true" size={14} />
          </ActionIcon>
        </Tooltip>
      ))}
    </Group>
  );
}
