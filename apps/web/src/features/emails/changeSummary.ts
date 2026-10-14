export function formatEmailUpdateSummary(changes: Record<string, unknown>) {
  const parts = emailUpdateChangeParts(changes);
  if (parts.length === 0) {
    return "Updated email content";
  }

  return `${formatList(parts)} changed`;
}

export function formatEmailUpdateChangedFields(changes: Record<string, unknown>) {
  const parts = emailUpdateChangeParts(changes, { includeEditableFieldNames: true });
  return parts.length > 0 ? parts.join(", ") : "";
}

export function formatEmailUpdateChangedReviewBlocks(
  metadata: Record<string, unknown>
) {
  const blocks = stringArrayMetadata(metadata, "changed_review_blocks");
  if (blocks.length === 0) {
    return "";
  }

  return `Blocks: ${blocks.map(humanizeKey).join(", ")}`;
}

function emailUpdateChangeParts(
  changes: Record<string, unknown>,
  options: { includeEditableFieldNames?: boolean } = {}
) {
  const parts: string[] = [];

  if (hasChange(changes, "title")) {
    parts.push("Title");
  }
  if (hasChange(changes, "subject")) {
    parts.push("Subject");
  }
  if (hasChange(changes, "preheader")) {
    parts.push("Preheader");
  }
  if (hasChange(changes, "original_html")) {
    parts.push("Template HTML");
  }
  if (hasChange(changes, "template_hash")) {
    parts.push("Template hash");
  }

  const editableFields = changes.editable_fields;
  if (isRecord(editableFields)) {
    const fieldNames = Object.keys(editableFields).sort();
    if (options.includeEditableFieldNames) {
      parts.push(...fieldNames.map((fieldName) => `Editable field: ${humanizeKey(fieldName)}`));
    } else if (fieldNames.length === 1) {
      parts.push(`Editable field ${humanizeKey(fieldNames[0])}`);
    } else if (fieldNames.length > 1) {
      parts.push(`${fieldNames.length} editable fields`);
    }
  }

  for (const key of Object.keys(changes).sort()) {
    if (knownChangeKeys.has(key)) {
      continue;
    }
    parts.push(humanizeKey(key));
  }

  return parts;
}

function hasChange(changes: Record<string, unknown>, key: string) {
  return Object.prototype.hasOwnProperty.call(changes, key);
}

function formatList(parts: string[]) {
  if (parts.length <= 2) {
    return parts.join(" and ");
  }

  return `${parts.slice(0, -1).join(", ")} and ${parts[parts.length - 1]}`;
}

function humanizeKey(key: string) {
  return key
    .replaceAll("_", " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stringArrayMetadata(metadata: Record<string, unknown>, key: string) {
  const value = metadata[key];
  if (!Array.isArray(value)) {
    return [];
  }

  return value.filter((item): item is string => typeof item === "string");
}

const knownChangeKeys = new Set([
  "editable_fields",
  "original_html",
  "preheader",
  "subject",
  "template_hash",
  "title",
]);
