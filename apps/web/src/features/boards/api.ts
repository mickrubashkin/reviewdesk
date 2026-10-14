import { fetchJson } from "../../shared/api";
import type {
  Board,
  BoardApprovalArea,
  CreateBoardApprovalAreaPayload,
  CreateBoardPayload,
  CreateBoardStagePayload,
  ReorderBoardApprovalAreasPayload,
  ReorderBoardStagesPayload,
  UpdateBoardStagePayload,
  UpdateBoardApprovalAreaPayload,
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

export function fetchBoardApprovalAreas(
  boardKey: string
): Promise<BoardApprovalArea[]> {
  return fetchJson<BoardApprovalArea[]>(
    `/api/boards/${encodeURIComponent(boardKey)}/approval-areas`
  );
}

export function createBoardApprovalArea(
  boardKey: string,
  payload: CreateBoardApprovalAreaPayload
): Promise<BoardApprovalArea> {
  return fetchJson<BoardApprovalArea>(
    `/api/boards/${encodeURIComponent(boardKey)}/approval-areas`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function updateBoardApprovalArea(
  boardKey: string,
  areaKey: string,
  payload: UpdateBoardApprovalAreaPayload
): Promise<BoardApprovalArea> {
  return fetchJson<BoardApprovalArea>(
    `/api/boards/${encodeURIComponent(boardKey)}/approval-areas/${encodeURIComponent(areaKey)}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
}

export function deleteBoardApprovalArea(
  boardKey: string,
  areaKey: string
): Promise<BoardApprovalArea> {
  return fetchJson<BoardApprovalArea>(
    `/api/boards/${encodeURIComponent(boardKey)}/approval-areas/${encodeURIComponent(areaKey)}`,
    {
      method: "DELETE",
    }
  );
}

export function reorderBoardApprovalAreas(
  boardKey: string,
  payload: ReorderBoardApprovalAreasPayload
): Promise<BoardApprovalArea[]> {
  return fetchJson<BoardApprovalArea[]>(
    `/api/boards/${encodeURIComponent(boardKey)}/approval-areas`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    }
  );
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
