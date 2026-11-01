import { useQuery } from "@tanstack/react-query";

import { fetchJson } from "../../shared/api";
import type {
  Board,
  BoardRegion,
  CreateBoardPayload,
  CreateBoardStagePayload,
  ReorderBoardStagesPayload,
  UpdateBoardStagePayload,
} from "../emails/types";

export function fetchBoards(): Promise<Board[]> {
  return fetchJson<Board[]>("/api/boards");
}

export function createBoard(payload: CreateBoardPayload): Promise<Board> {
  return fetchJson<Board>("/api/boards", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

export function createBoardStage(
  boardKey: string,
  payload: CreateBoardStagePayload
): Promise<Board> {
  return fetchJson<Board>(`/api/boards/${encodeURIComponent(boardKey)}/stages`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

export function updateBoardStage(
  boardKey: string,
  stage: string,
  payload: UpdateBoardStagePayload
): Promise<Board> {
  return fetchJson<Board>(
    `/api/boards/${encodeURIComponent(boardKey)}/stages/${encodeURIComponent(stage)}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function deleteBoardStage(boardKey: string, stage: string): Promise<Board> {
  return fetchJson<Board>(
    `/api/boards/${encodeURIComponent(boardKey)}/stages/${encodeURIComponent(stage)}`,
    {
      method: "DELETE",
    }
  );
}

export function reorderBoardStages(
  boardKey: string,
  payload: ReorderBoardStagesPayload
): Promise<Board> {
  return fetchJson<Board>(`/api/boards/${encodeURIComponent(boardKey)}/stages`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
}

export const boardRegionsQueryKey = (boardKey: string) => ["board-regions", boardKey];

export function useBoardRegions(boardKey: string | undefined) {
  return useQuery({
    queryKey: boardRegionsQueryKey(boardKey ?? ""),
    queryFn: async () =>
      (await fetchJson<{ regions: BoardRegion[] }>(`/api/boards/${encodeURIComponent(boardKey ?? "")}/regions`)).regions,
    enabled: Boolean(boardKey),
  });
}

// saveBoardRegions replaces the region list of a board; a region left out is
// deleted and its emails have none.
export async function saveBoardRegions(boardKey: string, regions: Array<Partial<BoardRegion>>): Promise<BoardRegion[]> {
  const response = await fetchJson<{ regions: BoardRegion[] }>(`/api/boards/${encodeURIComponent(boardKey)}/regions`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ regions }),
  });
  return response.regions;
}
