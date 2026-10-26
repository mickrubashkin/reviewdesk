import type { LayoutItem } from "../emails/types";
import type { BlockCatalogItem } from "./api";

// layoutItemId names the copy of a block inside an email, as the API does:
// the block key, plus the suffix for a repeat.
export function layoutItemId(item: LayoutItem) {
  return item.suffix ? `${item.block}_${item.suffix}` : item.block;
}

// withBlock adds a block at the end; a repeat gets the first free number as
// its suffix, so its fields and comments are its own.
export function withBlock(layout: LayoutItem[], block: string): LayoutItem[] {
  const ids = new Set(layout.map(layoutItemId));
  if (!ids.has(block)) {
    return [...layout, { block }];
  }
  let number = 2;
  while (ids.has(`${block}_${number}`)) {
    number += 1;
  }
  return [...layout, { block, suffix: String(number) }];
}

export function moveBlock(layout: LayoutItem[], index: number, offset: -1 | 1) {
  const target = index + offset;
  if (target < 0 || target >= layout.length) {
    return layout;
  }
  const next = [...layout];
  [next[index], next[target]] = [next[target], next[index]];
  return next;
}

export function sameLayout(a: LayoutItem[], b: LayoutItem[]) {
  return a.length === b.length && a.every((item, index) => layoutItemId(item) === layoutItemId(b[index]));
}

// Length limits of the subject and preheader, as the API checks them.
export const subjectMaxLength = 90;
export const preheaderMaxLength = 110;

// fieldLimits maps the subject, the preheader and each field of an email
// built from blocks to the length allowed, with the keys the email has them
// under.
export function fieldLimits(layout: LayoutItem[] | null | undefined, catalog: BlockCatalogItem[] | undefined) {
  const limits: Record<string, number> = { subject: subjectMaxLength, preheader: preheaderMaxLength };
  if (!layout || !catalog) {
    return limits;
  }
  for (const item of layout) {
    const block = catalog.find((candidate) => candidate.key === item.block);
    for (const field of block?.fields ?? []) {
      if (field.max_length) {
        limits[item.suffix ? `${field.key}_${item.suffix}` : field.key] = field.max_length;
      }
    }
  }
  return limits;
}
