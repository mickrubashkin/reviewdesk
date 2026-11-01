import { useMediaQuery } from "@mantine/hooks";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { ApiError } from "../../../shared/api";
import { demoMode } from "../../auth/api";
import { createBoard, fetchBoards, useBoardRegions } from "../api";
import {
  fetchEmails,
  updateEmailReviewStatus,
} from "../../emails/api";
import { formatEmailReviewStatus } from "../../emails/reviewStatus";
import { buildStageColumns } from "../../emails/stages";
import { getProductionStatus } from "../../emails/productionStatus";
import { withFirstSendEstimates } from "../../emails/firstSend";
import type {
  AuthUser,
  Board,
  CreateBoardPayload,
  EmailReviewStatus,
} from "../../emails/types";

import {
  applyReviewStatusToBoardCaches,
  filterColumnsByBoardFilters,
  filterColumnsBySearch,
  getActiveBoardFilterCount,
  getBoardFilterCounts,
  getBoardFilterOptions,
} from "./BoardHome.helpers";
import {
  defaultBoardFilters,
  type BoardFilterChange,
  type BoardFilters,
} from "./BoardHome.types";
import { canManageBoard, isAdminRole } from "../../emails/permissions";

export function useBoardHomeData({
  boardFilters,
  boardKey,
  boardSearchQuery,
  currentUser,
  onBoardFiltersChange,
}: {
  boardFilters: BoardFilters;
  boardKey: string;
  boardSearchQuery: string;
  currentUser: AuthUser;
  onBoardFiltersChange: (filters: BoardFilters) => void;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [userMenuOpened, setUserMenuOpened] = useState(false);
  const [createBoardModalOpened, setCreateBoardModalOpened] = useState(false);
  const [manageStagesModalOpened, setManageStagesModalOpened] = useState(false);
  const isCompactHeader = useMediaQuery("(max-width: 70em)");
  const isAdmin = isAdminRole(currentUser.role);
  // canManage is admin rights on this board: admins everywhere, board
  // managers (Users → Manages) on their boards; elsewhere the board is read-only.
  const canManage = canManageBoard(currentUser, boardKey);

  const boardsQuery = useQuery({
    queryKey: ["boards"],
    queryFn: fetchBoards,
  });
  const emailsQuery = useQuery({
    queryKey: ["emails", boardKey],
    queryFn: () => fetchEmails(boardKey),
    enabled: boardKey.trim() !== "",
  });
  const createBoardMutation = useMutation({
    mutationFn: (payload: CreateBoardPayload) => createBoard(payload),
    onSuccess: (board) => {
      // Known before the refetch, or the board page would not find it and
      // send us back to the previous board.
      queryClient.setQueryData<Board[]>(["boards"], (boards = []) => [...boards, board]);
      void queryClient.invalidateQueries({ queryKey: ["boards"] });
      setCreateBoardModalOpened(false);
      if (board.copied_email_count !== undefined) {
        notifications.show({
          color: "green",
          message: `${board.name} created with ${board.copied_email_count} EN drafts`,
        });
      }
      navigate(`/boards/${encodeURIComponent(board.key)}`);
    },
  });
  const reviewStatusMutation = useMutation({
    mutationFn: ({
      emailId,
      reviewStatus,
    }: {
      emailId: string;
      reviewStatus: EmailReviewStatus;
    }) =>
      updateEmailReviewStatus(emailId, {
        review_status: reviewStatus,
      }),
    onSuccess: (response, variables) => {
      applyReviewStatusToBoardCaches(
        queryClient,
        boardKey,
        variables.emailId,
        response.review_status
      );
      notifications.show({
        color: "green",
        message: `Status changed to ${formatEmailReviewStatus(response.review_status)}.`,
        title: "Status updated",
      });
    },
    onError: (error) => {
      notifications.show({
        color: "red",
        message:
          error instanceof ApiError && error.status === 409
            ? error.message.trim() || "Fix the email before marking it ready."
            : "Try again or check that you have admin access.",
        title:
          error instanceof ApiError && error.status === 409
            ? "Approval blocked"
            : "Status update failed",
      });
    },
  });

  const refreshBoardData = () => {
    void queryClient.invalidateQueries({ queryKey: ["boards"] });
    void queryClient.invalidateQueries({ queryKey: ["emails", boardKey] });
  };
  const boards = boardsQuery.data ?? [];
  const activeBoard = boards.find((board) => board.key === boardKey);

  const boardEmails = useMemo(
    () =>
      withFirstSendEstimates(
        (emailsQuery.data ?? []).map((email) => ({
          ...email,
          production_status: getProductionStatus(email, activeBoard?.portal_synced_at),
        }))
      ),
    [activeBoard?.portal_synced_at, emailsQuery.data]
  );
  const columns = useMemo(
    () => buildStageColumns(boardEmails, activeBoard?.stages ?? []),
    [activeBoard?.stages, boardEmails]
  );
  const boardRegions = useBoardRegions(boardKey).data;
  const searchQuery = boardSearchQuery.trim();
  const searchedColumns = useMemo(
    () => filterColumnsBySearch(columns, searchQuery),
    [columns, searchQuery]
  );
  const boardFilterOptions = useMemo(
    () => getBoardFilterOptions(emailsQuery.data ?? [], boardRegions ?? []),
    [emailsQuery.data, boardRegions]
  );
  const boardFilterCounts = useMemo(
    () => getBoardFilterCounts(boardEmails),
    [boardEmails]
  );
  const visibleColumns = useMemo(
    () => filterColumnsByBoardFilters(searchedColumns, boardFilters),
    [boardFilters, searchedColumns]
  );
  const filteredEmailIds = useMemo(
    () =>
      visibleColumns.flatMap((column) =>
        column.emailGroups.flatMap((group) =>
          group.versions.map((email) => email.id)
        )
      ),
    [visibleColumns]
  );
  const activeBoardFilterCount = getActiveBoardFilterCount(boardFilters);
  const updateBoardFilter: BoardFilterChange = (key, value) => {
    onBoardFiltersChange({ ...boardFilters, [key]: value });
  };
  const resetBoardFilters = () => {
    onBoardFiltersChange(defaultBoardFilters);
  };
  const hasBoardFilters = activeBoardFilterCount > 0;

  const filterEmptyState =
    searchQuery || hasBoardFilters ? "No matching emails" : "No emails on this board";
  const filterEmptyHint = searchQuery
    ? "Try another search or clear the query."
    : hasBoardFilters
      ? "Clear filters to browse the full sequence."
      : "Create or import an email to start reviewing.";

  const visibleGroupCount = visibleColumns.reduce(
    (count, column) => count + column.emailGroups.length,
    0
  );
  const totalGroupCount = columns.reduce(
    (count, column) => count + column.emailGroups.length,
    0
  );
  const preferredBoard =
    boards.find((board) => board.key === "onboarding") ?? boards[0];
  // The board's name is in the board picker next to it; the compact header
  // has the picker in its menu, so it names the board instead. A demo
  // visitor works on a private copy that the demo deletes after a day.
  const headerSubtitle = demoMode
    ? "Your own copy, resets in 24h"
    : searchQuery
    ? `${visibleGroupCount} of ${totalGroupCount} matches`
    : isCompactHeader
    ? (activeBoard?.name ?? "Board review")
    : `${emailsQuery.data?.length ?? 0} emails`;

  const handleReviewStatusChange = (
    emailId: string,
    reviewStatus: EmailReviewStatus
  ) => {
    reviewStatusMutation.mutate({ emailId, reviewStatus });
  };

  return {
    activeBoard,
    activeBoardFilterCount,
    boardFilterCounts,
    boardFilterOptions,
    boards,
    boardsQuery,
    columns,
    createBoardModalOpened,
    createBoardMutation,
    emailsQuery,
    filteredEmailIds,
    filterEmptyHint,
    filterEmptyState,
    hasBoardFilters,
    headerSubtitle,
    canManage,
    isAdmin,
    isCompactHeader,
    manageStagesModalOpened,
    preferredBoard,
    refreshBoardData,
    resetBoardFilters,
    setCreateBoardModalOpened,
    setManageStagesModalOpened,
    setUserMenuOpened,
    updateBoardFilter,
    userMenuOpened,
    visibleColumns,
    handleReviewStatusChange,
  };
}
