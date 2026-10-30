import {
  Burger,
  Button,
  Group,
  Menu,
  ScrollArea,
  SegmentedControl,
  Select,
  Stack,
  Text,
  Title,
} from "@mantine/core";
import { CaretDownIcon, CheckIcon, KanbanIcon, SignOutIcon } from "@phosphor-icons/react";

import { demoMode } from "../../auth/api";
import type { AuthUser, Board } from "../../emails/types";

import {
  BoardFilterControls,
  BoardFilterMenu,
  BoardSearchInput,
  MyEmailsToggle,
} from "./BoardFilterMenu";
import {
  type BoardFilterChange,
  type BoardFilterCounts,
  type BoardFilterOptions,
  type BoardFilters,
  type BoardView,
} from "./BoardHome.types";
import {
  AccountMenu,
  CreateActions,
  HelpMenuItem,
  NewMenu,
  ReplayTourMenuItem,
  WhatsNewMenuItem,
  SettingsMenu,
  SettingsMenuItems,
  UserMenuLabel,
} from "./BoardUserMenus";
import { MyTasksButton } from "../../my-tasks/MyTasksButton";
import { useBoardTour } from "../../tour/useBoardTour";
import styles from "../../../App.module.css";

type BoardHeaderProps = {
  activeBoard: Board | undefined;
  boardView: BoardView;
  onBoardViewChange: (view: BoardView) => void;
  activeBoardFilterCount: number;
  boardFilterCounts: BoardFilterCounts;
  boardFilterOptions: BoardFilterOptions;
  boardFilters: BoardFilters;
  boardSearchQuery: string;
  boards: Board[];
  boardsLoading: boolean;
  currentUser: AuthUser;
  hasBoardFilters: boolean;
  headerSubtitle: string;
  // canManage is admin rights on this board; isAdmin is the global admin role
  // (new boards, people, logs).
  canManage: boolean;
  isAdmin: boolean;
  isCompactHeader: boolean;
  isLoggingOut: boolean;
  userMenuOpened: boolean;
  onBoardChange: (value: string | null) => void;
  onBoardFilterChange: BoardFilterChange;
  onBoardSearchQueryChange: (query: string) => void;
  onCreateBoard: () => void;
  onExport: () => void;
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
  boardFilterCounts,
  boardFilterOptions,
  boardFilters,
  boardSearchQuery,
  boards,
  boardsLoading,
  currentUser,
  hasBoardFilters,
  headerSubtitle,
  canManage,
  isAdmin,
  isCompactHeader,
  isLoggingOut,
  userMenuOpened,
  onBoardChange,
  onBoardFilterChange,
  onBoardSearchQueryChange,
  onCreateBoard,
  onExport,
  onLogout,
  onManageStages,
  onResetBoardFilters,
  onUserMenuOpenedChange,
}: BoardHeaderProps) {
  useBoardTour(currentUser, !boardsLoading && activeBoard != null);
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
          boardFilterCounts={boardFilterCounts}
          boardFilterOptions={boardFilterOptions}
          boardFilters={boardFilters}
          boardOptions={boardOptions}
          boardSearchQuery={boardSearchQuery}
          boardsLoading={boardsLoading}
          currentUser={currentUser}
          hasBoardFilters={hasBoardFilters}
          canManage={canManage}
          isAdmin={isAdmin}
          isLoggingOut={isLoggingOut}
          userMenuOpened={userMenuOpened}
          onBoardChange={onBoardChange}
          onBoardFilterChange={onBoardFilterChange}
          onBoardSearchQueryChange={onBoardSearchQueryChange}
          onCreateBoard={onCreateBoard}
          onExport={onExport}
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
              data-tour="board"
              // Room for the longest name, so the current one is not cut.
              style={{ width: `calc(${Math.max(12, ...boardOptions.map((option) => option.label.length))}ch + 48px)` }}
              data={boardOptions}
              disabled={boardsLoading}
              leftSection={<KanbanIcon aria-hidden="true" size={16} />}
              size="sm"
              value={activeBoard?.key ?? null}
              onChange={onBoardChange}
            />
            <MyTasksButton currentUser={currentUser} />
            <MyEmailsToggle
              boardFilters={boardFilters}
              currentUser={currentUser}
              onBoardFilterChange={onBoardFilterChange}
            />
            <BoardFilterMenu
              activeBoardFilterCount={activeBoardFilterCount}
              boardFilterCounts={boardFilterCounts}
              boardFilterOptions={boardFilterOptions}
              boardFilters={boardFilters}
                  onBoardFilterChange={onBoardFilterChange}
              onResetBoardFilters={onResetBoardFilters}
            />
            <BoardSearchInput
              boardSearchQuery={boardSearchQuery}
              size="sm"
              onBoardSearchQueryChange={onBoardSearchQueryChange}
            />
            <BoardViewToggle value={boardView} onChange={onBoardViewChange} />
            <BoardViewMenu value={boardView} onChange={onBoardViewChange} />
          </Group>

          <Group className={styles.headerActions} gap="xs" wrap="nowrap">
            {canManage ? (
              <>
                <NewMenu onCreateBoard={isAdmin ? onCreateBoard : undefined} />
                <SettingsMenu
                  boardKey={activeBoard?.key}
                  currentUser={currentUser}
                  onExport={onExport}
                  onManageStages={onManageStages}
                />
              </>
            ) : null}

            <AccountMenu
              currentUser={currentUser}
              isLoggingOut={isLoggingOut}
              onExport={canManage ? undefined : onExport}
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
  boardFilterCounts,
  boardFilterOptions,
  boardFilters,
  boardOptions,
  boardSearchQuery,
  boardsLoading,
  currentUser,
  hasBoardFilters,
  canManage,
  isAdmin,
  isLoggingOut,
  userMenuOpened,
  onBoardChange,
  onBoardFilterChange,
  onBoardSearchQueryChange,
  onCreateBoard,
  onExport,
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
            data-tour="account"
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
              <BoardViewToggle inMenu value={boardView} onChange={onBoardViewChange} />
            </div>
            <div className={styles.boardFilterMenuControl}>
              <MyTasksButton currentUser={currentUser} fullWidth />
            </div>
            <Menu.Label>Filters</Menu.Label>
            <div className={styles.boardFilterMenuControl}>
              <MyEmailsToggle
                boardFilters={boardFilters}
                currentUser={currentUser}
                fullWidth
                onBoardFilterChange={onBoardFilterChange}
              />
            </div>
            <div className={styles.compactFilterControls}>
              <BoardFilterControls
                boardFilterCounts={boardFilterCounts}
                boardFilterOptions={boardFilterOptions}
                boardFilters={boardFilters}
                onBoardFilterChange={onBoardFilterChange}
              />
            </div>
            {hasBoardFilters ? (
              <Menu.Item color="red" onClick={onResetBoardFilters}>
                Clear filters
              </Menu.Item>
            ) : null}
            <Menu.Divider />
            <UserMenuLabel currentUser={currentUser} />
            {!canManage ? <Menu.Item onClick={onExport}>Export…</Menu.Item> : null}
            <HelpMenuItem />
            <WhatsNewMenuItem currentUser={currentUser} />
            {demoMode ? null : <ReplayTourMenuItem />}
            {canManage ? (
              <>
                <Menu.Divider />
                <Menu.Label>New</Menu.Label>
                <CreateActions onCreateBoard={isAdmin ? onCreateBoard : undefined} />
                <Menu.Divider />
                <Menu.Label>Settings</Menu.Label>
                <SettingsMenuItems
                  boardKey={activeBoard?.key}
                  currentUser={currentUser}
                  onExport={onExport}
                  onManageStages={onManageStages}
                />
              </>
            ) : null}
            {/* In the demo a logout would only open a new demo session. */}
            {demoMode ? null : (
              <>
                <Menu.Divider />
                <Menu.Item
                  color="red"
                  leftSection={<SignOutIcon aria-hidden="true" size={16} />}
                  onClick={onLogout}
                >
                  {isLoggingOut ? "Logging out" : "Logout"}
                </Menu.Item>
              </>
            )}
          </ScrollArea.Autosize>
        </Menu.Dropdown>
      </Menu>
    </Group>
  );
}

const boardViews: Array<{ label: string; value: BoardView }> = [
  { label: "Cards", value: "cards" },
  { label: "Matrix", value: "matrix" },
  { label: "Timeline", value: "timeline" },
  { label: "Archive", value: "archive" },
];

// inMenu is the toggle in the compact header's menu: there the header's own
// rule that hides it for the BoardViewMenu must not apply.
function BoardViewToggle({
  value,
  onChange,
  inMenu = false,
}: {
  value: BoardView;
  onChange: (view: BoardView) => void;
  inMenu?: boolean;
}) {
  return (
    <SegmentedControl
      className={inMenu ? undefined : styles.boardViewToggle}
      data-tour={inMenu ? undefined : "views"}
      data={boardViews}
      fullWidth={inMenu}
      size="xs"
      value={value}
      onChange={(next) => onChange(next as BoardView)}
    />
  );
}

// BoardViewMenu stands in for the toggle where the header has no room for it.
function BoardViewMenu({ value, onChange }: { value: BoardView; onChange: (view: BoardView) => void }) {
  return (
    <div className={styles.boardViewMenu}>
      <Menu position="bottom-start" withinPortal>
        <Menu.Target>
          <Button
            aria-label="View"
            data-tour="views"
            rightSection={<CaretDownIcon aria-hidden="true" size={14} />}
            size="sm"
            variant="white"
          >
            {boardViews.find((view) => view.value === value)?.label ?? "Cards"}
          </Button>
        </Menu.Target>
        <Menu.Dropdown>
          {boardViews.map((view) => (
            <Menu.Item
              key={view.value}
              leftSection={view.value === value ? <CheckIcon aria-hidden="true" size={14} /> : <span style={{ width: 14 }} />}
              onClick={() => onChange(view.value)}
            >
              {view.label}
            </Menu.Item>
          ))}
        </Menu.Dropdown>
      </Menu>
    </div>
  );
}
