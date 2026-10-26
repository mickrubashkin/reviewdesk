import { useQuery } from "@tanstack/react-query";

import { fetchJson } from "../../shared/api";

// A text many emails share, one per key and language: footer, privacy link,
// logo, signature. Emails point at it from a field with the same key. A board
// can have its own version (a regional footer); its emails take that one.
export type SharedText = {
  key: string;
  language: string;
  // Key of the board this version is for, "" for all boards.
  board: string;
  type: "text" | "url" | "image";
  value: string;
  version: number;
  updated_by: string;
  updated_at: string;
  linked_emails: number;
  linked_live: number;
  own_emails: number;
  en_changed: boolean;
};

export type UpdateSharedTextResponse = {
  text: SharedText;
  updated_emails: number;
  live_emails: number;
};

export const sharedTextsQueryKey = ["shared-texts"];

export async function fetchSharedTexts(): Promise<SharedText[]> {
  const response = await fetchJson<{ texts: SharedText[] }>("/api/shared-texts");
  return response.texts;
}

function sharedTextURL(key: string, language: string, board: string) {
  const path = `/api/shared-texts/${encodeURIComponent(key)}/${encodeURIComponent(language)}`;
  return board ? `${path}?board=${encodeURIComponent(board)}` : path;
}

export async function updateSharedText(
  text: SharedText,
  value: string
): Promise<UpdateSharedTextResponse> {
  return fetchJson<UpdateSharedTextResponse>(sharedTextURL(text.key, text.language, text.board), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ value, version: text.version }),
  });
}

// Gives board its own version of a shared text; the board's emails that use
// the text switch to it.
export async function createBoardSharedText(
  common: SharedText,
  board: string,
  value: string
): Promise<UpdateSharedTextResponse> {
  return fetchJson<UpdateSharedTextResponse>(sharedTextURL(common.key, common.language, board), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ value }),
  });
}

// Removes a board's own version; its emails go back to the text for all boards.
export async function deleteBoardSharedText(text: SharedText): Promise<UpdateSharedTextResponse> {
  return fetchJson<UpdateSharedTextResponse>(sharedTextURL(text.key, text.language, text.board), {
    method: "DELETE",
  });
}

export function useSharedTexts(enabled = true) {
  return useQuery({
    queryKey: sharedTextsQueryKey,
    queryFn: fetchSharedTexts,
    staleTime: 60_000,
    enabled,
  });
}

// Shared texts an email of one language on board uses, by key: the board's
// own version where there is one.
export function sharedTextsForEmail(
  texts: SharedText[] | undefined,
  language: string | undefined,
  board: string | undefined
): Record<string, SharedText> {
  const lang = (language ?? "").toLowerCase();
  const result: Record<string, SharedText> = {};
  for (const text of texts ?? []) {
    if (text.language !== lang) {
      continue;
    }
    if (text.board === "" ? !result[text.key] : text.board === board) {
      result[text.key] = text;
    }
  }
  return result;
}
