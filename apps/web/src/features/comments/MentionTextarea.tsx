import { Combobox, Text, Textarea, useCombobox, type TextareaProps } from "@mantine/core";
import { useRef, useState, type ChangeEvent } from "react";

import {
  filterMentionableUsers,
  mentionLabel,
  mentionQueryAt,
  useMentionableUsers,
} from "./mentions";

type MentionTextareaProps = Omit<TextareaProps, "value" | "onChange"> & {
  value: string;
  onChange: (value: string) => void;
  // Keep the suggestions inside the parent, e.g. inside a popover that closes
  // on clicks outside it.
  withinPortal?: boolean;
};

// MentionTextarea is a textarea that suggests people after "@" and inserts
// "@Name". Who is mentioned is read back from the text when it is sent.
export function MentionTextarea({
  value,
  onChange,
  withinPortal = true,
  ...textareaProps
}: MentionTextareaProps) {
  const combobox = useCombobox();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [mention, setMention] = useState<{ query: string; start: number } | null>(null);
  const users = useMentionableUsers().data ?? [];
  const options = mention ? filterMentionableUsers(users, mention.query) : [];

  const handleChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    const text = event.currentTarget.value;
    const next = mentionQueryAt(text, event.currentTarget.selectionStart ?? text.length);
    onChange(text);
    setMention(next);
    if (next && filterMentionableUsers(users, next.query).length > 0) {
      combobox.openDropdown();
      window.requestAnimationFrame(() => combobox.selectFirstOption());
    } else {
      combobox.closeDropdown();
    }
  };

  const handleOptionSubmit = (userId: string) => {
    const user = users.find((candidate) => candidate.id === userId);
    const textarea = textareaRef.current;
    if (!user || !mention || !textarea) {
      return;
    }
    const caret = textarea.selectionStart ?? value.length;
    const inserted = `@${mentionLabel(user)} `;
    onChange(value.slice(0, mention.start) + inserted + value.slice(caret));
    setMention(null);
    combobox.closeDropdown();
    const position = mention.start + inserted.length;
    window.requestAnimationFrame(() => {
      textarea.focus();
      textarea.setSelectionRange(position, position);
    });
  };

  return (
    <Combobox
      position="bottom-start"
      store={combobox}
      width={280}
      withinPortal={withinPortal}
      onOptionSubmit={handleOptionSubmit}
    >
      <Combobox.Target>
        <Textarea
          {...textareaProps}
          ref={textareaRef}
          value={value}
          onBlur={(event) => {
            combobox.closeDropdown();
            textareaProps.onBlur?.(event);
          }}
          onChange={handleChange}
        />
      </Combobox.Target>
      <Combobox.Dropdown hidden={options.length === 0}>
        <Combobox.Options>
          {options.map((user) => (
            <Combobox.Option key={user.id} value={user.id}>
              <Text size="sm" truncate>
                {mentionLabel(user)}
              </Text>
              {user.name ? (
                <Text c="dimmed" size="xs" truncate>
                  {user.email}
                </Text>
              ) : null}
            </Combobox.Option>
          ))}
        </Combobox.Options>
      </Combobox.Dropdown>
    </Combobox>
  );
}
