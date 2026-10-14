import type { Dispatch, SetStateAction } from "react";
import { Badge, Button, Menu, Stack, Text } from "@mantine/core";
import {
  CaretDownIcon,
  EnvelopeSimpleIcon,
  GearSixIcon,
  KanbanIcon,
  SignOutIcon,
  SlidersHorizontalIcon,
  StackIcon,
  UserCircleIcon,
} from "@phosphor-icons/react";
import { Link } from "react-router-dom";

import type { AuthUser, Board } from "../../emails/types";

import type { BoardFilterOptions, HandoffFilters } from "./BoardHome.types";
import { HandoffMenuContent } from "./HandoffMenu";
import styles from "../../../App.module.css";

export function SettingsMenu({
  currentUser,
  onCreateBoard,
  onExportForAI,
  onManageStages,
}: {
  currentUser: AuthUser;
  onCreateBoard: () => void;
  onExportForAI: () => void;
  onManageStages: () => void;
}) {
  return (
    <Menu position="bottom-end" width={220} withinPortal>
      <Menu.Target>
        <Button
          className={styles.headerMenuButton}
          leftSection={<GearSixIcon aria-hidden="true" size={16} />}
          rightSection={<CaretDownIcon aria-hidden="true" size={14} />}
          variant="white"
        >
          Settings
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>Create</Menu.Label>
        <CreateActions onCreateBoard={onCreateBoard} />
        <Menu.Divider />
        <Menu.Label>Board</Menu.Label>
        <Menu.Item
          leftSection={<SlidersHorizontalIcon aria-hidden="true" size={16} />}
          onClick={onManageStages}
        >
          Board settings
        </Menu.Item>
        {currentUser.role === "super_admin" ? (
          <Menu.Item onClick={onExportForAI}>Export for AI</Menu.Item>
        ) : null}
        <Menu.Divider />
        <OperationsMenuItems currentUser={currentUser} includeDividers />
      </Menu.Dropdown>
    </Menu>
  );
}

export function CreateActions({ onCreateBoard }: { onCreateBoard: () => void }) {
  return (
    <>
      <Menu.Item
        leftSection={<KanbanIcon aria-hidden="true" size={16} />}
        onClick={onCreateBoard}
      >
        New board
      </Menu.Item>
      <Menu.Item
        component={Link}
        leftSection={<StackIcon aria-hidden="true" size={16} />}
        to="/slots/new"
      >
        New slot (all languages)
      </Menu.Item>
      <Menu.Item
        component={Link}
        leftSection={<EnvelopeSimpleIcon aria-hidden="true" size={16} />}
        to="/emails/new"
      >
        New email
      </Menu.Item>
    </>
  );
}

export function OperationsMenuItems({
  currentUser,
  includeDividers = false,
}: {
  currentUser: AuthUser;
  includeDividers?: boolean;
}) {
  return (
    <>
      <Menu.Label>Operations</Menu.Label>
      <Menu.Item component={Link} to="/auth-events">
        Auth events
      </Menu.Item>
      <Menu.Item component={Link} to="/ai-logs">
        AI logs
      </Menu.Item>
      <Menu.Item component={Link} to="/admin/operational-events">
        Operational events
      </Menu.Item>
      {currentUser.role === "super_admin" ? (
        <>
          {includeDividers ? <Menu.Divider /> : null}
          <Menu.Item component={Link} to="/admin/email-events">
            Email events
          </Menu.Item>
          <Menu.Item component={Link} to="/admin/ai-config">
            AI rules
          </Menu.Item>
          <Menu.Item component={Link} to="/admin/variables">
            Bitrix variables
          </Menu.Item>
          <Menu.Item component={Link} to="/admin/portal">
            Portal sync
          </Menu.Item>
        </>
      ) : null}
      {currentUser.role === "admin" || currentUser.role === "super_admin" ? (
        <>
          {includeDividers && currentUser.role !== "super_admin" ? (
            <Menu.Divider />
          ) : null}
          <Menu.Item component={Link} to="/admin/users">
            Users
          </Menu.Item>
        </>
      ) : null}
    </>
  );
}

export function AccountMenu({
  activeBoard,
  activeHandoffFilterCount,
  boardFilterOptions,
  boardKey,
  canExportSequenceHandoff,
  currentUser,
  handoffFilters,
  isLoggingOut,
  sequenceHandoffEmailCount,
  sequenceHandoffJSON,
  onHandoffFiltersChange,
  onLogout,
}: {
  activeBoard: Board | undefined;
  activeHandoffFilterCount: number;
  boardFilterOptions: BoardFilterOptions;
  boardKey: string;
  canExportSequenceHandoff: boolean;
  currentUser: AuthUser;
  handoffFilters: HandoffFilters;
  isLoggingOut: boolean;
  sequenceHandoffEmailCount: number;
  sequenceHandoffJSON: string;
  onHandoffFiltersChange: Dispatch<SetStateAction<HandoffFilters>>;
  onLogout: () => void;
}) {
  return (
    <Menu position="bottom-end" width={260} withinPortal>
      <Menu.Target>
        <Button
          className={styles.accountButton}
          leftSection={<UserCircleIcon aria-hidden="true" size={18} />}
          rightSection={<CaretDownIcon aria-hidden="true" size={14} />}
          variant="white"
        >
          <span className={styles.accountEmail}>{currentUser.email}</span>
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <UserMenuLabel currentUser={currentUser} />
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
        <Menu.Item
          color="red"
          disabled={isLoggingOut}
          leftSection={<SignOutIcon aria-hidden="true" size={16} />}
          onClick={onLogout}
        >
          {isLoggingOut ? "Logging out" : "Logout"}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  );
}

export function UserMenuLabel({ currentUser }: { currentUser: AuthUser }) {
  return (
    <Menu.Label>
      <Stack gap={4}>
        <Text className={styles.userMenuEmail} size="sm">
          {currentUser.email}
        </Text>
        <Badge color={currentUser.role === "admin" ? "blue" : "gray"} size="sm">
          {currentUser.role}
        </Badge>
      </Stack>
    </Menu.Label>
  );
}
