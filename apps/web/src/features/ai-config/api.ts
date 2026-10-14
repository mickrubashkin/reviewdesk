import { fetchJson } from "../../shared/api";

export type AIRule = { title: string; body: string; enabled: boolean };
export type AIRole = { name: string; description: string; enabled: boolean };

export type BoardAIConfigDraft = {
  instruction: string;
  sequence_context: string;
  rules: AIRule[];
  roles: AIRole[];
};

export type BoardAIConfig = BoardAIConfigDraft & {
  board_key: string;
  is_default: boolean;
  updated_by_email: string;
  updated_at: string;
};

const configURL = (boardKey: string) =>
  `/api/boards/${encodeURIComponent(boardKey)}/ai-config`;

export function fetchBoardAIConfig(boardKey: string): Promise<BoardAIConfig> {
  return fetchJson<BoardAIConfig>(configURL(boardKey));
}

export function saveBoardAIConfig(
  boardKey: string,
  draft: BoardAIConfigDraft
): Promise<BoardAIConfig> {
  return fetchJson<BoardAIConfig>(configURL(boardKey), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(draft),
  });
}

export function resetBoardAIConfig(boardKey: string): Promise<void> {
  return fetchJson<void>(configURL(boardKey), { method: "DELETE" });
}

export type BoardAIExportPayload = {
  email_ids: string[];
  include_board: boolean;
  include_ai_setup: boolean;
  email_sections: string[];
  open_comments_only: boolean;
};

export type BoardAIExportResult = {
  markdown: string;
  email_count: number;
  file_name: string;
};

export function exportBoardForAI(
  boardKey: string,
  payload: BoardAIExportPayload
): Promise<BoardAIExportResult> {
  return fetchJson<BoardAIExportResult>(
    `/api/boards/${encodeURIComponent(boardKey)}/ai-export`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    }
  );
}
