import type { EventGroupOption } from "./EmailCreate.types";

export function optionalString(value: string) {
  const trimmed = value.trim();
  return trimmed || undefined;
}

export function uniqueSorted(values: string[]) {
  return Array.from(
    new Set(values.map((value) => value.trim()).filter(Boolean))
  ).sort((first, second) => first.localeCompare(second));
}

export function getNextSortOrder(
  emails: Array<{ sequence: string; stage: string | null; sort_order: number }>,
  sequence: string,
  stage: string
) {
  const matchingSortOrders = emails
    .filter((email) => email.sequence === sequence && (email.stage ?? "") === stage)
    .map((email) => email.sort_order);

  if (matchingSortOrders.length === 0) {
    return 0;
  }

  return Math.max(...matchingSortOrders) + 1;
}

export function buildEventGroupOptions(
  emails: Array<{
    sequence: string;
    stage: string | null;
    sort_order: number;
    title: string;
    language: string;
    variant: string;
    adaptation_label: string;
    send_timing: string | null;
  }>,
  sequence: string,
  stage: string
): EventGroupOption[] {
  const groups = new Map<string, {
    title: string;
    sortOrder: number;
    sendTiming: string | null;
    adaptationLabel: string;
    languages: Set<string>;
  }>();

  for (const email of emails) {
    if (email.sequence !== sequence || (email.stage ?? "") !== stage) {
      continue;
    }

    const key = [email.sequence, stage, email.sort_order].join("/");
    const group = groups.get(key) ?? {
      title: email.title,
      sortOrder: email.sort_order,
      sendTiming: email.send_timing,
      adaptationLabel: email.adaptation_label,
      languages: new Set<string>(),
    };
    group.languages.add(email.language);
    groups.set(key, group);
  }

  return [...groups.entries()]
    .sort(([, first], [, second]) => first.sortOrder - second.sortOrder)
    .map(([value, group]) => ({
      value,
      label: `${group.title} (${[...group.languages].sort().join(", ")})`,
      title: group.title,
      sortOrder: group.sortOrder,
      sendTiming: group.sendTiming,
      adaptationLabel: group.adaptationLabel,
    }));
}
