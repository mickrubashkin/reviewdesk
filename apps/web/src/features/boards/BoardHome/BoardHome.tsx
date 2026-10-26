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
import {
  Navigate,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";

import { fetchBoards, useBoardRegions } from "../api";
import { EmailBoard } from "../../emails/EmailBoard";
import type { AuthUser, EmailListItem } from "../../emails/types";

import {
  boardFilterStorageKey,
  boardScrollPositionStorageKey,
  boardSearchStorageKey,
  boardSelectedVersionsStorageKey,
  boardViewStorageKey,
  isBoardView,
  matrixByRegionStorageKey,
  type BoardFilters,
  type BoardView,
} from "./BoardHome.types";
import {
  readStoredBoardFilters,
  readStoredBoardScrollPosition,
  readStoredBoardSearch,
  readStoredBoardView,
  readStoredMatrixByRegion,
  readStoredSelectedVersions,
  writeSessionStorageValue,
} from "./BoardHome.storage";
import { BoardHeader } from "./BoardHeader";
import { CreateBoardModal } from "./CreateBoardModal";
import { ActiveBoardFilters } from "./ActiveBoardFilters";
import { BoardAIExportModal } from "./BoardAIExportModal";
import { ExportModal, type ExportScope } from "./ExportModal";
import { EmailSidePanel } from "./EmailSidePanel";
import { ArchiveList } from "./ArchiveList";
import { SlotMatrix } from "./SlotMatrix";
import { BoardTimeline } from "./Timeline/BoardTimeline";
import { UnsortedPanel } from "./UnsortedPanel";
import matrixStyles from "./SlotMatrix.module.css";
import { fetchEmails } from "../../emails/api";
import { getProductionStatus } from "../../emails/productionStatus";
import { ManageStagesModal } from "./ManageStagesModal";
import { canEditEmailContent } from "../../emails/permissions";
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
  const [searchParams, setSearchParams] = useSearchParams();
  const [aiExportOpened, setAiExportOpened] = useState(false);
  const [exportRequest, setExportRequest] = useState<{ scope: ExportScope; selected: EmailListItem[] } | null>(
    null
  );
  const [panelEmailId, setPanelEmailId] = useState<string | null>(null);
  // Live emails imported from the portal wait on the "<board>-portal" board
  // until they are placed into slots of this board from the matrix.
  const portalBoardKey = `${boardKey}-portal`;
  // The view is in the URL (?view=timeline) so a link opens the same one; a
  // link without it falls back to the one this tab used last.
  const [storedView, setStoredView] = useState<BoardView>(readStoredBoardView);
  const viewParam = searchParams.get("view");
  const boardView: BoardView = isBoardView(viewParam) ? viewParam : storedView;
  const setBoardView = (view: BoardView) => {
    setStoredView(view);
    writeSessionStorageValue(boardViewStorageKey, view);
    setSearchParams(
      (params) => {
        params.set("view", view);
        return params;
      },
      { replace: true }
    );
  };
  // On a board with regions the matrix can show them as columns instead of
  // languages.
  const boardRegions = useBoardRegions(boardKey).data ?? [];
  const [matrixByRegion, setMatrixByRegionState] = useState(readStoredMatrixByRegion);
  const setMatrixByRegion = (byRegion: boolean) => {
    setMatrixByRegionState(byRegion);
    writeSessionStorageValue(matrixByRegionStorageKey, byRegion);
  };
  const {
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
    handleReviewStatusChange,
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
  } = useBoardHomeData({
    boardFilters,
    boardKey,
    boardSearchQuery,
    currentUser,
    onBoardFiltersChange,
  });

  // Links from the review page ("Board › Stage") ask for a column to be
  // brought into view; the param is dropped once the column is found.
  const stageToReveal = searchParams.get("stage");
  useEffect(() => {
    if (!stageToReveal) {
      return;
    }
    const column = Array.from(
      document.querySelectorAll<HTMLElement>("[data-stage]")
    ).find((element) => element.dataset.stage === stageToReveal);
    if (!column) {
      return;
    }
    column.scrollIntoView({ block: "nearest", inline: "start" });
    setSearchParams(
      (params) => {
        params.delete("stage");
        return params;
      },
      { replace: true }
    );
  }, [stageToReveal, columns, setSearchParams]);

  const filteredIdSet = new Set(filteredEmailIds);
  const filteredEmails = (emailsQuery.data ?? []).filter((email) => filteredIdSet.has(email.id));
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
          boardFilterCounts={boardFilterCounts}
          boardFilterOptions={boardFilterOptions}
          boardFilters={boardFilters}
          boardSearchQuery={boardSearchQuery}
          boards={boards}
          boardsLoading={boardsQuery.isLoading}
          currentUser={currentUser}
          hasBoardFilters={hasBoardFilters}
          headerSubtitle={headerSubtitle}
          canManage={canManage}
          isAdmin={isAdmin}
          isCompactHeader={isCompactHeader}
          isLoggingOut={isLoggingOut}
          userMenuOpened={userMenuOpened}
          onBoardChange={handleBoardChange}
          onBoardFilterChange={updateBoardFilter}
          onBoardSearchQueryChange={onBoardSearchQueryChange}
          onCreateBoard={() => setCreateBoardModalOpened(true)}
          onExport={() => setExportRequest({ scope: hasBoardFilters ? "filtered" : "board", selected: [] })}
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
          <ActiveBoardFilters
            boardFilterOptions={boardFilterOptions}
            boardFilters={boardFilters}
            onBoardFilterChange={updateBoardFilter}
            onResetBoardFilters={resetBoardFilters}
          />
          <Box className={styles.boardContent}>
            {boardView !== "archive" && (emailsQuery.isLoading || boardsQuery.isLoading) ? (
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

            {boardView === "archive" ? (
              <ArchiveList
                boardKey={boardKey}
                canRestore={canManage}
                languages={boardFilters.languages}
                searchQuery={boardSearchQuery}
              />
            ) : emailsQuery.isSuccess ? (
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
                      board={activeBoard}
                      boardColumns={columns}
                      canManage={canManage}
                      columns={visibleColumns}
                      selectedEmailId={panelEmailId}
                      canTranslate={(language) => canEditEmailContent(currentUser, { language, sequence: boardKey })}
                      onEmailCreated={setPanelEmailId}
                      onExportSelected={(selected) => setExportRequest({ scope: "selected", selected })}
                      onOpenEmail={(email) => setPanelEmailId(email.id)}
                      byRegion={matrixByRegion && boardRegions.length > 0}
                      regions={boardRegions}
                      onByRegionChange={setMatrixByRegion}
                    />
                  </div>
                ) : boardView === "timeline" ? (
                  <BoardTimeline
                    boardKey={boardKey}
                    columns={visibleColumns}
                    regions={boardRegions}
                    selectedEmailId={panelEmailId}
                    onOpenEmail={setPanelEmailId}
                  />
                ) : (
                <EmailBoard
                  columns={visibleColumns}
                  currentUser={currentUser}
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
        </Box>
      </AppShell.Main>

      {panelEmail ? (
        <EmailSidePanel
          canManage={canManage}
          canTranslate={canEditEmailContent(currentUser, panelEmail)}
          currentUserEmail={currentUser.email}
          email={panelEmail}
          siblings={panelSiblings}
          onClose={() => setPanelEmailId(null)}
          onSelect={(email) => setPanelEmailId(email.id)}
        />
      ) : null}

      {exportRequest && activeBoard && emailsQuery.data ? (
        <ExportModal
          board={activeBoard}
          boardEmails={emailsQuery.data}
          currentUser={currentUser}
          filteredEmails={filteredEmails}
          initialScope={exportRequest.scope}
          selectedEmails={exportRequest.selected}
          onClose={() => setExportRequest(null)}
          onOpenBoardPackForAI={() => {
            setExportRequest(null);
            setAiExportOpened(true);
          }}
        />
      ) : null}

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
