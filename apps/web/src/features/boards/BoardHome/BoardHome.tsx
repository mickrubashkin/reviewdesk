import {
  type Dispatch,
  type SetStateAction,
  useEffect,
  useState,
} from "react";
import {
  Alert,
  AppShell,
  Box,
  Loader,
  Stack,
  Text,
} from "@mantine/core";
import { useQuery } from "@tanstack/react-query";
import { Navigate, useNavigate, useParams } from "react-router-dom";

import { fetchBoards } from "../api";
import { EmailBoard } from "../../emails/EmailBoard";
import type { AuthUser } from "../../emails/types";

import {
  boardFilterStorageKey,
  boardScrollPositionStorageKey,
  boardSearchStorageKey,
  boardSelectedVersionsStorageKey,
  boardViewStorageKey,
  type BoardFilters,
  type BoardView,
} from "./BoardHome.types";
import {
  readStoredBoardFilters,
  readStoredBoardScrollPosition,
  readStoredBoardSearch,
  readStoredBoardView,
  readStoredSelectedVersions,
  writeSessionStorageValue,
} from "./BoardHome.storage";
import { BoardHeader } from "./BoardHeader";
import { CreateBoardModal } from "./CreateBoardModal";
import { BoardAIExportModal } from "./BoardAIExportModal";
import { useFilteredExport } from "./useFilteredExport";
import { EmailSidePanel } from "./EmailSidePanel";
import { SlotMatrix } from "./SlotMatrix";
import { UnsortedPanel } from "./UnsortedPanel";
import matrixStyles from "./SlotMatrix.module.css";
import { fetchEmails } from "../../emails/api";
import { getProductionStatus } from "../../emails/productionStatus";
import { ManageStagesModal } from "./ManageStagesModal";
import { useBoardHomeData } from "./useBoardHomeData";
import styles from "../../../App.module.css";

export function BoardHomeRedirect() {
  const boardsQuery = useQuery({
    queryKey: ["boards"],
    queryFn: fetchBoards,
  });

  if (boardsQuery.isLoading) {
    return (
      <Stack align="center" justify="center" h="100dvh">
        <Loader />
        <Text c="dimmed">Loading boards</Text>
      </Stack>
    );
  }

  if (boardsQuery.isError || !boardsQuery.data || boardsQuery.data.length === 0) {
    return (
      <Stack align="center" justify="center" h="100dvh" p="md">
        <Alert color="red" title="Failed to load boards">
          Check that the API server is reachable.
        </Alert>
      </Stack>
    );
  }

  const preferredBoard =
    boardsQuery.data.find((board) => board.key === "onboarding") ??
    boardsQuery.data[0];

  return <Navigate replace to={`/boards/${encodeURIComponent(preferredBoard.key)}`} />;
}

export function EmailBoardRoute({
  currentUser,
  isLoggingOut,
  onLogout,
}: {
  currentUser: AuthUser;
  isLoggingOut: boolean;
  onLogout: () => void;
}) {
  const { boardKey } = useParams();
  const [selectedVersionByGroup, setSelectedVersionByGroup] = useState<
    Record<string, string>
  >(readStoredSelectedVersions);
  const [boardScrollPosition, setBoardScrollPosition] = useState(
    readStoredBoardScrollPosition
  );
  const [boardFilters, setBoardFilters] = useState<BoardFilters>(
    readStoredBoardFilters
  );
  const [boardSearchQuery, setBoardSearchQuery] = useState(readStoredBoardSearch);
  useEffect(() => {
    writeSessionStorageValue(
      boardSelectedVersionsStorageKey,
      selectedVersionByGroup
    );
  }, [selectedVersionByGroup]);
  useEffect(() => {
    writeSessionStorageValue(boardScrollPositionStorageKey, boardScrollPosition);
  }, [boardScrollPosition]);
  useEffect(() => {
    writeSessionStorageValue(boardFilterStorageKey, boardFilters);
  }, [boardFilters]);
  useEffect(() => {
    writeSessionStorageValue(boardSearchStorageKey, boardSearchQuery);
  }, [boardSearchQuery]);

  return boardKey ? (
    <EmailBoardApp
      boardFilters={boardFilters}
      boardKey={boardKey}
      boardScrollPosition={boardScrollPosition}
      boardSearchQuery={boardSearchQuery}
      currentUser={currentUser}
      isLoggingOut={isLoggingOut}
      selectedVersionByGroup={selectedVersionByGroup}
      onLogout={onLogout}
      onBoardScrollPositionChange={setBoardScrollPosition}
      onBoardFiltersChange={setBoardFilters}
      onBoardSearchQueryChange={setBoardSearchQuery}
      onSelectedVersionByGroupChange={setSelectedVersionByGroup}
    />
  ) : (
    <Navigate replace to="/" />
  );
}

function EmailBoardApp({
  boardFilters,
  boardKey,
  boardScrollPosition,
  boardSearchQuery,
  currentUser,
  isLoggingOut,
  selectedVersionByGroup,
  onLogout,
  onBoardScrollPositionChange,
  onBoardFiltersChange,
  onBoardSearchQueryChange,
  onSelectedVersionByGroupChange,
}: {
  boardFilters: BoardFilters;
  boardKey: string;
  boardScrollPosition: { x: number; y: number };
  boardSearchQuery: string;
  currentUser: AuthUser;
  isLoggingOut: boolean;
  selectedVersionByGroup: Record<string, string>;
  onLogout: () => void;
  onBoardScrollPositionChange: (position: { x: number; y: number }) => void;
  onBoardFiltersChange: (filters: BoardFilters) => void;
  onBoardSearchQueryChange: (query: string) => void;
  onSelectedVersionByGroupChange: Dispatch<
    SetStateAction<Record<string, string>>
  >;
}) {
  const navigate = useNavigate();
  const [aiExportOpened, setAiExportOpened] = useState(false);
  const [panelEmailId, setPanelEmailId] = useState<string | null>(null);
  // Live emails imported from the portal wait on the "<board>-portal" board
  // until they are placed into slots of this board from the matrix.
  const portalBoardKey = `${boardKey}-portal`;
  const [boardView, setBoardViewState] = useState<BoardView>(readStoredBoardView);
  const setBoardView = (view: BoardView) => {
    setBoardViewState(view);
    writeSessionStorageValue(boardViewStorageKey, view);
  };
  const {
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
    handleReviewStatusChange,
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
    sequenceHandoffEmailCount,
    sequenceHandoffJSON,
    setCreateBoardModalOpened,
    setHandoffFilters,
    setManageStagesModalOpened,
    setUserMenuOpened,
    updateBoardFilter,
    userMenuOpened,
    visibleColumns,
  } = useBoardHomeData({
    boardFilters,
    boardKey,
    boardSearchQuery,
    currentUser,
    onBoardFiltersChange,
  });

  const filteredExport = useFilteredExport(filteredEmailIds);
  const unsortedQuery = useQuery({
    queryKey: ["emails", portalBoardKey],
    queryFn: () => fetchEmails(portalBoardKey),
    enabled: boardView === "matrix" && isAdmin && boards.some((b) => b.key === portalBoardKey),
  });

  if (boardsQuery.isSuccess && !activeBoard && preferredBoard) {
    return <Navigate replace to={`/boards/${encodeURIComponent(preferredBoard.key)}`} />;
  }

  const handleSelectVersion = (groupKey: string, emailId: string) => {
    onSelectedVersionByGroupChange((current) => ({
      ...current,
      [groupKey]: emailId,
    }));
  };

  // Emails open in a side panel next to the board; "Open" in the panel goes
  // to the full review page.
  const handleOpenVersionGroup = (_groupKey: string, emailId: string) => {
    setPanelEmailId(emailId);
  };
  const unsortedEmails = (unsortedQuery.data ?? []).map((email) => ({
    ...email,
    production_status: getProductionStatus(email, activeBoard?.portal_synced_at),
  }));
  const boardGroup = panelEmailId
    ? columns.flatMap((c) => c.emailGroups).find((g) => g.versions.some((v) => v.id === panelEmailId))
    : undefined;
  const unsortedPanelEmail = boardGroup ? undefined : unsortedEmails.find((e) => e.id === panelEmailId);
  const panelSiblings = boardGroup?.versions ?? (unsortedPanelEmail ? [unsortedPanelEmail] : []);
  const panelEmail = panelSiblings.find((v) => v.id === panelEmailId);

  const handleBoardChange = (value: string | null) => {
    if (value) {
      navigate(`/boards/${encodeURIComponent(value)}`);
    }
  };

  return (
    <AppShell header={{ height: 64 }} padding={0}>
      <AppShell.Header className={styles.appHeader}>
        <BoardHeader
          activeBoard={activeBoard}
          activeBoardFilterCount={activeBoardFilterCount}
          activeHandoffFilterCount={activeHandoffFilterCount}
          boardFilterOptions={boardFilterOptions}
          boardFilters={boardFilters}
          boardKey={boardKey}
          boardSearchQuery={boardSearchQuery}
          boards={boards}
          boardsLoading={boardsQuery.isLoading}
          canExportSequenceHandoff={canExportSequenceHandoff}
          currentUser={currentUser}
          filteredExport={filteredExport}
          handoffFilters={handoffFilters}
          hasBoardFilters={hasBoardFilters}
          headerSubtitle={headerSubtitle}
          isAdmin={isAdmin}
          isCompactHeader={isCompactHeader}
          isLoggingOut={isLoggingOut}
          openCommentEmailCount={openCommentEmailCount}
          sequenceHandoffEmailCount={sequenceHandoffEmailCount}
          sequenceHandoffJSON={sequenceHandoffJSON}
          userMenuOpened={userMenuOpened}
          onBoardChange={handleBoardChange}
          onBoardFilterChange={updateBoardFilter}
          onBoardSearchQueryChange={onBoardSearchQueryChange}
          onCreateBoard={() => setCreateBoardModalOpened(true)}
          onExportForAI={() => setAiExportOpened(true)}
          onHandoffFiltersChange={setHandoffFilters}
          onLogout={onLogout}
          onManageStages={() => setManageStagesModalOpened(true)}
          onResetBoardFilters={resetBoardFilters}
          onUserMenuOpenedChange={setUserMenuOpened}
          boardView={boardView}
          onBoardViewChange={setBoardView}
        />
      </AppShell.Header>

      <AppShell.Main>
        <Box className={styles.boardPage}>
          {emailsQuery.isLoading || boardsQuery.isLoading ? (
            <Stack align="center" justify="center" h="100%">
              <Loader />
              <Text c="dimmed">Loading emails</Text>
            </Stack>
          ) : null}

          {emailsQuery.isError || boardsQuery.isError ? (
            <Alert color="red" title="Failed to load emails">
              Check that the API server is reachable.
            </Alert>
          ) : null}

          {emailsQuery.isSuccess ? (
            visibleColumns.length > 0 ? (
              boardView === "matrix" ? (
                <div className={matrixStyles.matrixView}>
                  {activeBoard && isAdmin ? (
                    <UnsortedPanel
                      board={activeBoard}
                      columns={columns}
                      emails={unsortedEmails}
                      onPreview={(email) => setPanelEmailId(email.id)}
                    />
                  ) : null}
                  <SlotMatrix
                    columns={visibleColumns}
                    selectedEmailId={panelEmailId}
                    onOpenEmail={(email) => setPanelEmailId(email.id)}
                  />
                </div>
              ) : (
              <EmailBoard
                columns={visibleColumns}
                scrollPosition={boardScrollPosition}
                selectedVersionByGroup={selectedVersionByGroup}
                onOpenVersionGroup={handleOpenVersionGroup}
                onReviewStatusChange={handleReviewStatusChange}
                onScrollPositionChange={onBoardScrollPositionChange}
                onSelectVersion={handleSelectVersion}
              />
              )
            ) : (
              <Stack align="center" justify="center" h="100%" ta="center">
                <Text fw={600}>{filterEmptyState}</Text>
                <Text c="rgba(255, 255, 255, 0.72)" size="sm">
                  {filterEmptyHint}
                </Text>
              </Stack>
            )
          ) : null}
        </Box>
      </AppShell.Main>

      {panelEmail ? (
        <EmailSidePanel
          canManage={isAdmin}
          email={panelEmail}
          siblings={panelSiblings}
          onClose={() => setPanelEmailId(null)}
          onSelect={(email) => setPanelEmailId(email.id)}
        />
      ) : null}

      {isAdmin ? filteredExport.dialog : null}

      {aiExportOpened && activeBoard && emailsQuery.data ? (
        <BoardAIExportModal
          board={activeBoard}
          emails={emailsQuery.data}
          onClose={() => setAiExportOpened(false)}
        />
      ) : null}

      {createBoardModalOpened && activeBoard ? (
        <CreateBoardModal
          error={createBoardMutation.error}
          isSubmitting={createBoardMutation.isPending}
          sourceBoard={activeBoard}
          onClose={() => {
            createBoardMutation.reset();
            setCreateBoardModalOpened(false);
          }}
          onResetError={() => createBoardMutation.reset()}
          onSubmit={(payload) => createBoardMutation.mutateAsync(payload)}
        />
      ) : null}

      {manageStagesModalOpened && activeBoard ? (
        <ManageStagesModal
          board={activeBoard}
          stageCounts={new Map(columns.map((column) => [column.stage, column.emailGroups.length]))}
          onClose={() => setManageStagesModalOpened(false)}
          onMutated={refreshBoardData}
        />
      ) : null}
    </AppShell>
  );
}
