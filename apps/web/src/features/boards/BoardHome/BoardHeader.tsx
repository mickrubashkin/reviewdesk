import type { Dispatch, SetStateAction } from "react";
import {
  Burger,
  Group,
  Menu,
  ScrollArea,
  SegmentedControl,
  Select,
  Stack,
  Text,
  Title,
} from "@mantine/core";
import {
  KanbanIcon,
  SignOutIcon,
  SlidersHorizontalIcon,
} from "@phosphor-icons/react";

import type { AuthUser, Board } from "../../emails/types";

import {
  BoardFilterControls,
  BoardFilterMenu,
  BoardSearchInput,
  MyEmailsToggle,
} from "./BoardFilterMenu";
import {
  type BoardFilterOptions,
  type BoardFilters,
  type BoardView,
  type HandoffFilters,
} from "./BoardHome.types";
import {
  AccountMenu,
  CreateActions,
  OperationsMenuItems,
  SettingsMenu,
  UserMenuLabel,
} from "./BoardUserMenus";
import { AppVersionInfo } from "./AppVersionInfo";
import { FilteredExportItems } from "./FilteredExportItems";
import type { FilteredExport } from "./useFilteredExport";
import { HandoffMenuContent } from "./HandoffMenu";
import styles from "../../../App.module.css";

type BoardHeaderProps = {
  activeBoard: Board | undefined;
  boardView: BoardView;
  onBoardViewChange: (view: BoardView) => void;
  activeBoardFilterCount: number;
  activeHandoffFilterCount: number;
  boardFilterOptions: BoardFilterOptions;
  boardFilters: BoardFilters;
  boardKey: string;
  boardSearchQuery: string;
  boards: Board[];
  boardsLoading: boolean;
  canExportSequenceHandoff: boolean;
  currentUser: AuthUser;
  filteredExport: FilteredExport;
  handoffFilters: HandoffFilters;
  hasBoardFilters: boolean;
  headerSubtitle: string;
  isAdmin: boolean;
  isCompactHeader: boolean;
  isLoggingOut: boolean;
  openCommentEmailCount: number;
  sequenceHandoffEmailCount: number;
  sequenceHandoffJSON: string;
  userMenuOpened: boolean;
  onBoardChange: (value: string | null) => void;
  onBoardFilterChange: (key: keyof BoardFilters, value: string | null) => void;
  onBoardSearchQueryChange: (query: string) => void;
  onCreateBoard: () => void;
  onExportForAI: () => void;
  onHandoffFiltersChange: Dispatch<SetStateAction<HandoffFilters>>;
  onLogout: () => void;
  onManageStages: () => void;
  onResetBoardFilters: () => void;
  onUserMenuOpenedChange: (opened: boolean) => void;
};

export function BoardHeader({
  activeBoard,
  boardView,
  onBoardViewChange,
  activeBoardFilterCount,
  activeHandoffFilterCount,
  boardFilterOptions,
  boardFilters,
  boardKey,
  boardSearchQuery,
  boards,
  boardsLoading,
  canExportSequenceHandoff,
  currentUser,
  filteredExport,
  handoffFilters,
  hasBoardFilters,
  headerSubtitle,
  isAdmin,
  isCompactHeader,
  isLoggingOut,
  openCommentEmailCount,
  sequenceHandoffEmailCount,
  sequenceHandoffJSON,
  userMenuOpened,
  onBoardChange,
  onBoardFilterChange,
  onBoardSearchQueryChange,
  onCreateBoard,
  onExportForAI,
  onHandoffFiltersChange,
  onLogout,
  onManageStages,
  onResetBoardFilters,
  onUserMenuOpenedChange,
}: BoardHeaderProps) {
  const boardOptions = boards.map((board) => ({
    label: board.name,
    value: board.key,
  }));

  return (
    <Group className={styles.headerInner} h="100%" px="md" wrap="nowrap">
      <Stack className={styles.headerBrand} gap={0}>
        <Title className={styles.headerTitle} order={4}>
          ReviewDesk
        </Title>
        <Text className={styles.headerSubtitle} size="xs">
          {headerSubtitle}
        </Text>
      </Stack>

      {isCompactHeader ? (
        <CompactHeaderMenu
          activeBoard={activeBoard}
          boardView={boardView}
          onBoardViewChange={onBoardViewChange}
          activeHandoffFilterCount={activeHandoffFilterCount}
          boardFilterOptions={boardFilterOptions}
          boardFilters={boardFilters}
          boardKey={boardKey}
          boardOptions={boardOptions}
          boardSearchQuery={boardSearchQuery}
          boardsLoading={boardsLoading}
          canExportSequenceHandoff={canExportSequenceHandoff}
          currentUser={currentUser}
          filteredExport={filteredExport}
          handoffFilters={handoffFilters}
          hasBoardFilters={hasBoardFilters}
          isAdmin={isAdmin}
          isLoggingOut={isLoggingOut}
          openCommentEmailCount={openCommentEmailCount}
          sequenceHandoffEmailCount={sequenceHandoffEmailCount}
          sequenceHandoffJSON={sequenceHandoffJSON}
          userMenuOpened={userMenuOpened}
          onBoardChange={onBoardChange}
          onBoardFilterChange={onBoardFilterChange}
          onBoardSearchQueryChange={onBoardSearchQueryChange}
          onCreateBoard={onCreateBoard}
          onExportForAI={onExportForAI}
          onHandoffFiltersChange={onHandoffFiltersChange}
          onLogout={onLogout}
          onManageStages={onManageStages}
          onResetBoardFilters={onResetBoardFilters}
          onUserMenuOpenedChange={onUserMenuOpenedChange}
        />
      ) : (
        <>
          <Group className={styles.boardControls} gap="xs" wrap="nowrap">
            <Select
              allowDeselect={false}
              className={styles.boardSelect}
              data={boardOptions}
              disabled={boardsLoading}
              leftSection={<KanbanIcon aria-hidden="true" size={16} />}
              size="sm"
              value={activeBoard?.key ?? null}
              onChange={onBoardChange}
            />
            <MyEmailsToggle
              boardFilterOptions={boardFilterOptions}
              boardFilters={boardFilters}
              currentUser={currentUser}
              onBoardFilterChange={onBoardFilterChange}
            />
            <BoardFilterMenu
              activeBoardFilterCount={activeBoardFilterCount}
              boardFilterOptions={boardFilterOptions}
              boardFilters={boardFilters}
              filteredExport={filteredExport}
              hasBoardFilters={hasBoardFilters}
              isAdmin={isAdmin}
              openCommentEmailCount={openCommentEmailCount}
              onBoardFilterChange={onBoardFilterChange}
              onResetBoardFilters={onResetBoardFilters}
            />
            <BoardSearchInput
              boardSearchQuery={boardSearchQuery}
              size="sm"
              onBoardSearchQueryChange={onBoardSearchQueryChange}
            />
            <BoardViewToggle value={boardView} onChange={onBoardViewChange} />
          </Group>

          <Group className={styles.headerActions} gap="xs" wrap="nowrap">
            {currentUser.role === "super_admin" ? <AppVersionInfo onDark /> : null}
            {isAdmin ? (
              <SettingsMenu
                currentUser={currentUser}
                onCreateBoard={onCreateBoard}
                onExportForAI={onExportForAI}
                onManageStages={onManageStages}
              />
            ) : null}

            <AccountMenu
              activeBoard={activeBoard}
              activeHandoffFilterCount={activeHandoffFilterCount}
              boardFilterOptions={boardFilterOptions}
              boardKey={boardKey}
              canExportSequenceHandoff={canExportSequenceHandoff}
              currentUser={currentUser}
              handoffFilters={handoffFilters}
              isLoggingOut={isLoggingOut}
              sequenceHandoffEmailCount={sequenceHandoffEmailCount}
              sequenceHandoffJSON={sequenceHandoffJSON}
              onHandoffFiltersChange={onHandoffFiltersChange}
              onLogout={onLogout}
            />
          </Group>
        </>
      )}
    </Group>
  );
}

function CompactHeaderMenu({
  activeBoard,
  boardView,
  onBoardViewChange,
  activeHandoffFilterCount,
  boardFilterOptions,
  boardFilters,
  boardKey,
  boardOptions,
  boardSearchQuery,
  boardsLoading,
  canExportSequenceHandoff,
  currentUser,
  filteredExport,
  handoffFilters,
  hasBoardFilters,
  isAdmin,
  isLoggingOut,
  openCommentEmailCount,
  sequenceHandoffEmailCount,
  sequenceHandoffJSON,
  userMenuOpened,
  onBoardChange,
  onBoardFilterChange,
  onBoardSearchQueryChange,
  onCreateBoard,
  onExportForAI,
  onHandoffFiltersChange,
  onLogout,
  onManageStages,
  onResetBoardFilters,
  onUserMenuOpenedChange,
}: Omit<
  BoardHeaderProps,
  | "activeBoardFilterCount"
  | "boards"
  | "headerSubtitle"
  | "isCompactHeader"
> & {
  boardOptions: Array<{ label: string; value: string }>;
}) {
  return (
    <Group className={styles.headerActions} gap="sm" wrap="nowrap">
      <Menu
        opened={userMenuOpened}
        position="bottom-end"
        width={260}
        withinPortal
        onChange={onUserMenuOpenedChange}
      >
        <Menu.Target>
          <Burger
            aria-label="Open user menu"
            color="white"
            opened={userMenuOpened}
            size="sm"
          />
        </Menu.Target>
        <Menu.Dropdown className={styles.compactHeaderDropdown} p={0}>
          <ScrollArea.Autosize
            mah="calc(100dvh - var(--app-shell-header-height) - 24px)"
            offsetScrollbars="y"
            overscrollBehavior="contain"
            scrollbars="y"
            type="auto"
          >
            <Menu.Label>Current board</Menu.Label>
            <div className={styles.boardFilterMenuControl}>
              <Select
                data={boardOptions}
                disabled={boardsLoading}
                size="xs"
                value={activeBoard?.key ?? null}
                onChange={onBoardChange}
              />
            </div>
            <Menu.Label>Search</Menu.Label>
            <div className={styles.boardFilterMenuControl}>
              <BoardSearchInput
                boardSearchQuery={boardSearchQuery}
                size="xs"
                onBoardSearchQueryChange={onBoardSearchQueryChange}
              />
            </div>
            <Menu.Label>View</Menu.Label>
            <div className={styles.boardFilterMenuControl}>
              <BoardViewToggle value={boardView} onChange={onBoardViewChange} />
            </div>
            <Menu.Label>Filters</Menu.Label>
            <div className={styles.boardFilterMenuControl}>
              <MyEmailsToggle
                boardFilterOptions={boardFilterOptions}
                boardFilters={boardFilters}
                currentUser={currentUser}
                fullWidth
                onBoardFilterChange={onBoardFilterChange}
              />
            </div>
            <BoardFilterControls
              boardFilterOptions={boardFilterOptions}
              boardFilters={boardFilters}
              commentLabel={`Open comments ${openCommentEmailCount}`}
              onBoardFilterChange={onBoardFilterChange}
            />
            {hasBoardFilters ? (
              <Menu.Item color="red" onClick={onResetBoardFilters}>
                Clear filters
              </Menu.Item>
            ) : null}
            {isAdmin ? (
              <>
                <Menu.Divider />
                <FilteredExportItems filteredExport={filteredExport} />
              </>
            ) : null}
            <Menu.Divider />
            <HandoffMenuContent
              activeBoard={activeBoard}
              activeHandoffFilterCount={activeHandoffFilterCount}
              boardFilterOptions={boardFilterOptions}
              boardKey={boardKey}
              canExportSequenceHandoff={canExportSequenceHandoff}
              handoffFilters={handoffFilters}
              sequenceHandoffEmailCount={sequenceHandoffEmailCount}
              sequenceHandoffJSON={sequenceHandoffJSON}
              onHandoffFiltersChange={onHandoffFiltersChange}
            />
            <Menu.Divider />
            <UserMenuLabel currentUser={currentUser} />
            {currentUser.role === "super_admin" ? (
              <Menu.Label>
                <AppVersionInfo />
              </Menu.Label>
            ) : null}
            {isAdmin ? (
              <>
                <Menu.Divider />
                <Menu.Label>Settings</Menu.Label>
                <CreateActions onCreateBoard={onCreateBoard} />
                <Menu.Item
                  leftSection={
                    <SlidersHorizontalIcon aria-hidden="true" size={16} />
                  }
                  onClick={onManageStages}
                >
                  Board settings
                </Menu.Item>
                {currentUser.role === "super_admin" ? (
                  <Menu.Item onClick={onExportForAI}>Export for AI</Menu.Item>
                ) : null}
                <Menu.Divider />
                <OperationsMenuItems currentUser={currentUser} />
              </>
            ) : null}
            <Menu.Divider />
            <Menu.Item
              color="red"
              leftSection={<SignOutIcon aria-hidden="true" size={16} />}
              onClick={onLogout}
            >
              {isLoggingOut ? "Logging out" : "Logout"}
            </Menu.Item>
          </ScrollArea.Autosize>
        </Menu.Dropdown>
      </Menu>
    </Group>
  );
}

function BoardViewToggle({ value, onChange }: { value: BoardView; onChange: (view: BoardView) => void }) {
  return (
    <SegmentedControl
      className={styles.boardViewToggle}
      data={[
        { label: "Cards", value: "cards" },
        { label: "Matrix", value: "matrix" },
      ]}
      size="sm"
      value={value}
      onChange={(next) => onChange(next as BoardView)}
    />
  );
}
