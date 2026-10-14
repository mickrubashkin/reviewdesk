import type { EmailVariant } from "../types";

export function formatEmailTitle(title: string) {
  switch (title) {
    case "Follow Up 1":
      return "First Follow-up";
    case "Follow Up 2":
      return "Second Follow-up";
    default:
      return title;
  }
}

export function formatVariantLabel(variant: EmailVariant) {
  const normalizedVariant = variant.trim().toLowerCase();
  if (normalizedVariant === "old") {
    return "v0";
  }
  if (normalizedVariant === "new") {
    return "v1";
  }

  return variant;
}

export function formatVariantOptionLabel(
  variant: EmailVariant,
  variants: EmailVariant[]
) {
  const label = formatVariantLabel(variant);
  const hasLabelCollision = variants.some(
    (otherVariant) =>
      otherVariant !== variant && formatVariantLabel(otherVariant) === label
  );

  if (hasLabelCollision && isLegacyVariant(variant)) {
    return `${label} legacy`;
  }

  return label;
}

export function formatDueDate(value: string) {
  const dueDate = parseDateOnly(value);
  if (!dueDate) {
    return value;
  }

  return dueDate.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
  });
}

export function getDueDateTone(value: string) {
  const dueDate = parseDateOnly(value);
  if (!dueDate) {
    return undefined;
  }

  const today = new Date();
  today.setHours(0, 0, 0, 0);
  dueDate.setHours(0, 0, 0, 0);

  const daysUntilDue = Math.round(
    (dueDate.getTime() - today.getTime()) / 86_400_000
  );

  if (daysUntilDue < 0) {
    return "overdue";
  }
  if (daysUntilDue <= 2) {
    return "soon";
  }

  return undefined;
}

function isLegacyVariant(variant: EmailVariant) {
  const normalizedVariant = variant.trim().toLowerCase();
  return normalizedVariant === "old" || normalizedVariant === "new";
}

function parseDateOnly(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) {
    return null;
  }

  const [, year, month, day] = match;
  return new Date(Number(year), Number(month) - 1, Number(day));
}
