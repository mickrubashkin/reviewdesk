import { useQuery, type QueryClient } from "@tanstack/react-query";

import { fetchJson } from "../../shared/api";

export type MentionableUser = {
  id: string;
  email: string;
  name: string;
};

export function fetchMentionableUsers(): Promise<MentionableUser[]> {
  return fetchJson<MentionableUser[]>("/api/users/mentionable");
}

export const mentionableUsersQueryKey = ["users", "mentionable"];

export function useMentionableUsers() {
  return useQuery({
    queryKey: mentionableUsersQueryKey,
    queryFn: fetchMentionableUsers,
    staleTime: 5 * 60 * 1000,
  });
}

// mentionLabel is what follows "@" in a message: the name, or the address
// for people without one.
export function mentionLabel(user: Pick<MentionableUser, "email" | "name">) {
  return user.name.trim() || user.email;
}

const wordChar = /[\p{L}\p{N}_]/u;

// findMention returns where "@label" occurs in text as a whole mention, so
// "@Ali" does not match inside "@Alice".
export function findMention(text: string, label: string, from = 0) {
  const needle = `@${label}`.toLowerCase();
  const haystack = text.toLowerCase();
  let index = haystack.indexOf(needle, from);
  while (index >= 0) {
    const next = text.charAt(index + needle.length);
    if (!next || !wordChar.test(next)) {
      return index;
    }
    index = haystack.indexOf(needle, index + 1);
  }
  return -1;
}

// mentionedUserIds lists the people whose "@label" is in the text. The
// longest labels win, so "@Ali Lee" is not also counted as "@Ali".
export function mentionedUserIds(text: string, users: MentionableUser[]) {
  let remaining = text;
  const ids: string[] = [];
  const byLongestLabel = [...users].sort((a, b) => mentionLabel(b).length - mentionLabel(a).length);
  for (const user of byLongestLabel) {
    const label = mentionLabel(user);
    let index = findMention(remaining, label);
    if (index < 0) {
      continue;
    }
    ids.push(user.id);
    while (index >= 0) {
      remaining = remaining.slice(0, index) + " ".repeat(label.length + 1) + remaining.slice(index + label.length + 1);
      index = findMention(remaining, label);
    }
  }
  return ids;
}

// mentionIdsForBody reads who a message mentions, using the people the
// mention textarea loaded.
export function mentionIdsForBody(queryClient: QueryClient, body: string) {
  return mentionedUserIds(body, queryClient.getQueryData<MentionableUser[]>(mentionableUsersQueryKey) ?? []);
}

// mentionQueryAt finds an "@query" being typed right before the caret. A
// query may hold one space, so "@Ali Le" still searches.
export function mentionQueryAt(text: string, caret: number) {
  const before = text.slice(0, caret);
  const match = /(?:^|\s)@([\p{L}\p{N}._-]*(?: [\p{L}\p{N}._-]*)?)$/u.exec(before);
  if (!match) {
    return null;
  }
  const query = match[1];
  return { query, start: caret - query.length - 1 };
}

export function filterMentionableUsers(users: MentionableUser[], query: string, limit = 8) {
  const needle = query.trim().toLowerCase();
  return users
    .filter((user) => !needle || user.name.toLowerCase().includes(needle) || user.email.includes(needle))
    .slice(0, limit);
}

export type MessagePart = { text: string; mention: boolean };

// splitMentions cuts a message into plain text and "@label" parts for the
// given labels, so mentions can be highlighted.
export function splitMentions(text: string, labels: string[]): MessagePart[] {
  const ranges: Array<[number, number]> = [];
  for (const label of [...labels].sort((a, b) => b.length - a.length)) {
    let index = findMention(text, label);
    while (index >= 0) {
      const end = index + label.length + 1;
      if (!ranges.some(([start, stop]) => index < stop && end > start)) {
        ranges.push([index, end]);
      }
      index = findMention(text, label, index + 1);
    }
  }
  ranges.sort((a, b) => a[0] - b[0]);

  const parts: MessagePart[] = [];
  let cursor = 0;
  for (const [start, end] of ranges) {
    if (start > cursor) {
      parts.push({ text: text.slice(cursor, start), mention: false });
    }
    parts.push({ text: text.slice(start, end), mention: true });
    cursor = end;
  }
  if (cursor < text.length) {
    parts.push({ text: text.slice(cursor), mention: false });
  }
  return parts;
}
