import { useMediaQuery } from "@mantine/hooks";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { ApiError } from "../../../shared/api";
import { createBoard, fetchBoards } from "../api";
import {
  fetchEmails,
  updateEmailReviewStatus,
} from "../../emails/api";
import {
  formatEmailReviewStatus,
  isApprovedEmailReviewStatus,
} from "../../emails/reviewStatus";
import { buildStageColumns } from "../../emails/stages";
import { getProductionStatus } from "../../emails/productionStatus";
import type {
  AuthUser,
  CreateBoardPayload,
  EmailReviewStatus,
} from "../../emails/types";

import {
  applyReviewStatusToBoardCaches,
  buildSequenceHandoffManifest,
  filterColumnsByBoardFilters,
  filterColumnsBySearch,
  filterEmailsByHandoffFilters,
  getActiveBoardFilterCount,
  getActiveHandoffFilterCount,
  getBoardFilterOptions,
} from "./BoardHome.helpers";
import {
  defaultBoardFilters,
  defaultHandoffFilters,
  type BoardFilters,
  type HandoffFilters,
} from "./BoardHome.types";

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
  const [handoffFilters, setHandoffFilters] =
    useState<HandoffFilters>(defaultHandoffFilters);
  const isCompactHeader = useMediaQuery("(max-width: 64em)");
  const isAdmin =
    currentUser.role === "admin" || currentUser.role === "super_admin";

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
      void queryClient.invalidateQueries({ queryKey: ["boards"] });
      setCreateBoardModalOpened(false);
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
        message: `Review status changed to ${formatEmailReviewStatus(response.review_status)}.`,
        title: "Status updated",
      });
    },
    onError: (error) => {
      notifications.show({
        color: "red",
        message:
          error instanceof ApiError && error.status === 409
            ? "Resolve blocking comments before approval."
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
      (emailsQuery.data ?? []).map((email) => ({
        ...email,
        production_status: getProductionStatus(email, activeBoard?.portal_synced_at),
      })),
    [activeBoard?.portal_synced_at, emailsQuery.data]
  );
  const columns = useMemo(
    () => buildStageColumns(boardEmails, activeBoard?.stages ?? []),
    [activeBoard?.stages, boardEmails]
  );
  const searchQuery = boardSearchQuery.trim();
  const searchedColumns = useMemo(
    () => filterColumnsBySearch(columns, searchQuery),
    [columns, searchQuery]
  );
  const boardFilterOptions = useMemo(
    () => getBoardFilterOptions(emailsQuery.data ?? []),
    [emailsQuery.data]
  );
  const openCommentEmailCount = useMemo(
    () =>
      (emailsQuery.data ?? []).filter(
        (email) => (email.open_comment_count ?? 0) > 0
      ).length,
    [emailsQuery.data]
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
  const handoffEmails = useMemo(
    () => filterEmailsByHandoffFilters(emailsQuery.data ?? [], handoffFilters),
    [emailsQuery.data, handoffFilters]
  );
  const sequenceHandoffManifest = useMemo(
    () =>
      buildSequenceHandoffManifest(
        activeBoard?.key ?? boardKey,
        activeBoard?.name ?? boardKey,
        handoffEmails
      ),
    [activeBoard?.key, activeBoard?.name, boardKey, handoffEmails]
  );
  const sequenceHandoffJSON = useMemo(
    () => JSON.stringify(sequenceHandoffManifest, null, 2),
    [sequenceHandoffManifest]
  );
  const activeBoardFilterCount = getActiveBoardFilterCount(boardFilters);
  const updateBoardFilter = (key: keyof BoardFilters, value: string | null) => {
    onBoardFiltersChange({
      ...boardFilters,
      [key]: value ?? "",
    });
  };
  const resetBoardFilters = () => {
    onBoardFiltersChange(defaultBoardFilters);
  };
  const hasBoardFilters = activeBoardFilterCount > 0;
  const activeHandoffFilterCount = getActiveHandoffFilterCount(handoffFilters);
  const canExportSequenceHandoff = sequenceHandoffManifest.email_count > 0;

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
  const headerSubtitle = searchQuery
    ? `${activeBoard?.name ?? "Board review"} · ${visibleGroupCount} of ${totalGroupCount} matches`
    : `${activeBoard?.name ?? "Board review"} · ${emailsQuery.data?.length ?? 0} emails`;

  const handleReviewStatusChange = (
    emailId: string,
    reviewStatus: EmailReviewStatus
  ) => {
    const targetEmail = (emailsQuery.data ?? []).find((email) => email.id === emailId);
    if (
      isApprovedEmailReviewStatus(reviewStatus) &&
      targetEmail &&
      (targetEmail.open_blocking_comment_count ?? 0) > 0
    ) {
      notifications.show({
        color: "red",
        message: "Resolve blocking comments before approval.",
        title: "Approval blocked",
      });
      return;
    }

    reviewStatusMutation.mutate({ emailId, reviewStatus });
  };

  return {
    activeBoard,
    activeBoardFilterCount,
    activeHandoffFilterCount,
    boardFilterOptions,
    boards,
    boardsQuery,
    canExportSequenceHandoff,
    columns,
    createBoardModalOpened,
    createBoardMutation,
    emailsQuery,
    filteredEmailIds,
    filterEmptyHint,
    filterEmptyState,
    handoffFilters,
    hasBoardFilters,
    headerSubtitle,
    isAdmin,
    isCompactHeader,
    manageStagesModalOpened,
    openCommentEmailCount,
    preferredBoard,
    refreshBoardData,
    resetBoardFilters,
    sequenceHandoffEmailCount: sequenceHandoffManifest.email_count,
    sequenceHandoffJSON,
    setCreateBoardModalOpened,
    setHandoffFilters,
    setManageStagesModalOpened,
    setUserMenuOpened,
    updateBoardFilter,
    userMenuOpened,
    visibleColumns,
    handleReviewStatusChange,
  };
}
