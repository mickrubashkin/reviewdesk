import { formatStageName } from "../../emails/stages";
import { emailStatusParts } from "../../emails/statusSummary";
import type { BoardRegion, EmailListItem } from "../../emails/types";
import { regionName } from "../regions";

const preferredLanguages = ["en", "de", "es", "pl", "br"];

export type ChainCell = string | number | null;

export type ChainTable = {
  headers: string[];
  rows: ChainCell[][];
};

const headers = [
  "Stage",
  "Position",
  "Email",
  "Timing",
  "Condition",
  "Language",
  "Version",
  "Adaptation",
  "Status",
  "Review status",
  "Current version",
  "Uploaded version",
  "Portal sends",
  "Last sent",
  "Translation outdated",
  "Subject",
  "Preheader",
  "Owner",
  "Open comments",
  "Link",
];

function slotKey(email: EmailListItem) {
  return `${email.stage ?? ""}#${email.sort_order}`;
}

function languageRank(language: string) {
  const index = preferredLanguages.indexOf(language);
  return index >= 0 ? index : preferredLanguages.length;
}

// buildChainTable lays the emails out as the sequence reads: stage by stage
// in board order, slot by slot, then language. Status is the same one-line
// summary the board shows.
// On a board with regions the Adaptation column is the email's region.
export function buildChainTable(
  emails: EmailListItem[],
  stages: string[],
  origin: string,
  regions: BoardRegion[] = []
): ChainTable {
  const stageRank = (stage: string) => {
    const index = stages.indexOf(stage);
    return index >= 0 ? index : stages.length;
  };
  const sorted = [...emails].sort(
    (a, b) =>
      stageRank(a.stage ?? "") - stageRank(b.stage ?? "") ||
      (a.stage ?? "").localeCompare(b.stage ?? "") ||
      a.sort_order - b.sort_order ||
      languageRank(a.language) - languageRank(b.language) ||
      a.language.localeCompare(b.language) ||
      a.variant.localeCompare(b.variant) ||
      a.adaptation_label.localeCompare(b.adaptation_label)
  );
  // Versions of the same slot and language, so the status can tell when
  // another version is the one the robots send.
  const siblings = new Map<string, EmailListItem[]>();
  for (const email of emails) {
    const key = `${slotKey(email)}:${email.language}`;
    siblings.set(key, [...(siblings.get(key) ?? []), email]);
  }

  const rows = sorted.map((email): ChainCell[] => [
    formatStageName(email.stage ?? ""),
    email.sort_order,
    email.title,
    email.send_timing ?? "",
    email.send_condition ?? "",
    email.language.toUpperCase(),
    email.variant,
    regions.length > 0 ? regionName(email, regions) : email.adaptation_key === "default" ? "" : email.adaptation_label,
    emailStatusParts(email, siblings.get(`${slotKey(email)}:${email.language}`) ?? [])
      .map((part) => part.label)
      .join(" · "),
    email.review_status,
    email.latest_version ?? null,
    email.uploaded_version ?? null,
    email.portal_send_count,
    email.portal_last_sent_at ? email.portal_last_sent_at.slice(0, 10) : "",
    email.translation_stale ? "yes" : "",
    email.subject ?? "",
    email.preheader ?? "",
    email.owner_email ?? "",
    email.open_comment_count,
    `${origin}/emails/${encodeURIComponent(email.id)}/review`,
  ]);
  return {
    headers: regions.length > 0 ? headers.map((header) => (header === "Adaptation" ? "Region" : header)) : headers,
    rows,
  };
}

function csvCell(value: ChainCell) {
  const text = value === null ? "" : String(value);
  return /[",\n\r]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

// chainTableCSV starts with a byte order mark so Excel reads UTF-8.
export function chainTableCSV(table: ChainTable) {
  return "﻿" + [table.headers, ...table.rows].map((row) => row.map(csvCell).join(",")).join("\r\n");
}

export async function chainTableXLSX(table: ChainTable): Promise<Blob> {
  const { default: writeXlsxFile } = await import("write-excel-file/browser");
  const header = table.headers.map((value) => ({ value, fontWeight: "bold" as const }));
  const widths = table.headers.map((name) =>
    ["Email", "Subject", "Preheader", "Status"].includes(name) ? 40 : name === "Link" ? 30 : 14
  );
  return writeXlsxFile([header, ...table.rows], {
    columns: widths.map((width) => ({ width })),
    sheet: "Chain",
    stickyRowsCount: 1,
  }).toBlob();
}
