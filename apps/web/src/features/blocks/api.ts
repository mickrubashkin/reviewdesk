import { useQuery } from "@tanstack/react-query";

import { fetchJson } from "../../shared/api";
import type { LayoutItem } from "../emails/types";

// A field of a block: key and type come from its data-edit-* markup.
export type BlockField = {
  key: string;
  type: "text" | "url" | "image" | "number";
  label: string;
  required?: boolean;
  max_length?: number;
  // An email takes the shared text with the same key unless it keeps its own.
  shared?: boolean;
  // "markdown": a new email gets the field as Markdown (a list).
  format?: "markdown";
};

// A block of the library emails are built from; "frame" is the document the
// others go into.
export type EmailBlock = {
  key: string;
  name: string;
  description: string;
  html: string;
  fields: BlockField[];
  version: number;
  updated_by: string;
  updated_at: string;
  // Active emails built with the block, and how many of them are live.
  emails: number;
  live_emails: number;
};

// A saved block and the emails that got a new version because of it.
export type BlockSaveResult = {
  block: EmailBlock;
  updated_emails: number;
  live_emails: number;
};

export const frameKey = "frame";

export const blocksQueryKey = ["blocks"];

// A block as the email editor offers it: no HTML, no frame.
export type BlockCatalogItem = Pick<EmailBlock, "key" | "name" | "description" | "fields">;

export const blockCatalogQueryKey = ["block-catalog"];

export function useBlockCatalog(enabled = true) {
  return useQuery({
    queryKey: blockCatalogQueryKey,
    queryFn: async () =>
      (await fetchJson<{ blocks: BlockCatalogItem[] }>("/api/block-catalog")).blocks,
    enabled,
    staleTime: 5 * 60 * 1000,
  });
}

export function useBlocks(enabled = true) {
  return useQuery({
    queryKey: blocksQueryKey,
    queryFn: async () => (await fetchJson<{ blocks: EmailBlock[] }>("/api/blocks")).blocks,
    enabled,
  });
}

function blockURL(key: string) {
  return `/api/blocks/${encodeURIComponent(key)}`;
}

export async function updateBlock(block: EmailBlock, html: string): Promise<BlockSaveResult> {
  return fetchJson<BlockSaveResult>(blockURL(block.key), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ html, version: block.version }),
  });
}

// Renders HTML for a block, saved or not, inside the frame (the frame with
// every block in it) with the values written in its markup.
export async function previewBlock(key: string, html: string): Promise<string> {
  const response = await fetchJson<{ html: string }>(`${blockURL(key)}/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ html }),
  });
  return response.html;
}

// The blocks a new email in the stage starts with: the most common layout of
// the stage, then of the board, then the library default.
export function useDefaultLayout(sequence: string | undefined, stage: string | null, enabled = true) {
  return useQuery({
    queryKey: ["default-layout", sequence, stage],
    queryFn: async () =>
      (
        await fetchJson<{ layout: LayoutItem[] }>(
          `/api/default-layout?${new URLSearchParams({ sequence: sequence ?? "", stage: stage ?? "" })}`
        )
      ).layout,
    enabled: enabled && Boolean(sequence && stage),
    staleTime: 60 * 1000,
  });
}

export type LayoutPreviewPayload = {
  sequence: string;
  language: string;
  layout: LayoutItem[];
  subject?: string;
  preheader?: string;
};

// Renders an email not created yet, the way creating it from the blocks
// would: shared texts of the language and board, the rest as in the blocks.
export async function previewLayout(payload: LayoutPreviewPayload): Promise<string> {
  const response = await fetchJson<{ html: string }>("/api/layout-preview", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return response.html;
}
