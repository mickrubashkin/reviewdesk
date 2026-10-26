import { Badge, Button, Menu, Stack, Text } from "@mantine/core";
import {
  CaretDownIcon,
  DownloadSimpleIcon,
  EnvelopeSimpleIcon,
  GearSixIcon,
  KanbanIcon,
  PlusIcon,
  SignOutIcon,
  SlidersHorizontalIcon,
  StackIcon,
  UserCircleIcon,
} from "@phosphor-icons/react";
import { Link } from "react-router-dom";

import { isAdminRole } from "../../emails/permissions";
import { demoMode } from "../../auth/api";
import type { AuthUser } from "../../emails/types";

import { AppVersionInfo } from "./AppVersionInfo";
import styles from "../../../App.module.css";

// onCreateBoard is left out for board managers: only admins add boards.
export function NewMenu({ onCreateBoard }: { onCreateBoard?: () => void }) {
  return (
    <Menu position="bottom-end" width={220} withinPortal>
      <Menu.Target>
        <Button
          aria-label="New"
          className={`${styles.headerMenuButton} ${styles.collapsibleButton}`}
          leftSection={<PlusIcon aria-hidden="true" size={16} />}
          rightSection={<CaretDownIcon aria-hidden="true" size={14} />}
          variant="white"
        >
          <span className={styles.collapsibleLabel}>New</span>
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <CreateActions onCreateBoard={onCreateBoard} />
      </Menu.Dropdown>
    </Menu>
  );
}

export function CreateActions({ onCreateBoard }: { onCreateBoard?: () => void }) {
  return (
    <>
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
      {onCreateBoard ? (
        <Menu.Item
          leftSection={<KanbanIcon aria-hidden="true" size={16} />}
          onClick={onCreateBoard}
        >
          New board
        </Menu.Item>
      ) : null}
    </>
  );
}

export function SettingsMenu({
  boardKey,
  currentUser,
  onExport,
  onManageStages,
}: {
  boardKey?: string;
  currentUser: AuthUser;
  onExport: () => void;
  onManageStages: () => void;
}) {
  return (
    <Menu position="bottom-end" width={260} withinPortal>
      <Menu.Target>
        <Button
          aria-label="Settings"
          className={`${styles.headerMenuButton} ${styles.collapsibleButton}`}
          leftSection={<GearSixIcon aria-hidden="true" size={16} />}
          rightSection={<CaretDownIcon aria-hidden="true" size={14} />}
          variant="white"
        >
          <span className={styles.collapsibleLabel}>Settings</span>
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <SettingsMenuItems
          boardKey={boardKey}
          currentUser={currentUser}
          onExport={onExport}
          onManageStages={onManageStages}
        />
      </Menu.Dropdown>
    </Menu>
  );
}

// Shared by the Settings dropdown and the compact (burger) menu.
export function SettingsMenuItems({
  boardKey,
  currentUser,
  onExport,
  onManageStages,
}: {
  boardKey?: string;
  currentUser: AuthUser;
  onExport: () => void;
  onManageStages: () => void;
}) {
  const isSuperAdmin = currentUser.role === "super_admin";
  // Board managers see the board items only; people and logs stay with admins.
  const isAdmin = isAdminRole(currentUser.role);

  return (
    <>
      <Menu.Item
        leftSection={<SlidersHorizontalIcon aria-hidden="true" size={16} />}
        onClick={onManageStages}
      >
        Board settings
      </Menu.Item>
      <Menu.Item
        leftSection={<DownloadSimpleIcon aria-hidden="true" size={16} />}
        onClick={onExport}
      >
        Export…
      </Menu.Item>
      {!isSuperAdmin && boardKey ? (
        <Menu.Item
          component={Link}
          to={`/admin/shared-texts?board=${encodeURIComponent(boardKey)}`}
        >
          Shared texts
        </Menu.Item>
      ) : null}
      {isSuperAdmin ? (
        <>
          <Menu.Divider />
          <Menu.Label>Content</Menu.Label>
          <Menu.Item component={Link} to="/admin/ai-config">
            AI rules
          </Menu.Item>
          <Menu.Item component={Link} to="/admin/variables">
            Bitrix variables
          </Menu.Item>
          <Menu.Item component={Link} to="/admin/shared-texts">
            Shared texts
          </Menu.Item>
          <Menu.Item component={Link} to="/admin/blocks">
            Blocks
          </Menu.Item>
          <Menu.Item component={Link} to="/admin/glossary">
            Translation glossary
          </Menu.Item>
          <Menu.Label>Integrations</Menu.Label>
          <Menu.Item component={Link} to="/admin/portal">
            Portal sync
          </Menu.Item>
        </>
      ) : null}
      {isAdmin ? (
        <>
          <Menu.Divider />
          <Menu.Label>People</Menu.Label>
          <Menu.Item component={Link} to="/admin/users">
            Users
          </Menu.Item>
          <Menu.Divider />
          <Menu.Sub position="left-start" openDelay={80}>
            <Menu.Sub.Target>
              <Menu.Sub.Item>Logs</Menu.Sub.Item>
            </Menu.Sub.Target>
            <Menu.Sub.Dropdown>
              <Menu.Item component={Link} to="/auth-events">
                Auth
              </Menu.Item>
              <Menu.Item component={Link} to="/ai-logs">
                AI
              </Menu.Item>
              <Menu.Item component={Link} to="/admin/operational-events">
                Operational
              </Menu.Item>
              {isSuperAdmin ? (
                <Menu.Item component={Link} to="/admin/email-events">
                  Email events
                </Menu.Item>
              ) : null}
            </Menu.Sub.Dropdown>
          </Menu.Sub>
        </>
      ) : null}
      {isSuperAdmin ? (
        <>
          <Menu.Divider />
          <Menu.Label>
            <AppVersionInfo />
          </Menu.Label>
        </>
      ) : null}
    </>
  );
}

export function AccountMenu({
  currentUser,
  isLoggingOut,
  onExport,
  onLogout,
}: {
  currentUser: AuthUser;
  isLoggingOut: boolean;
  // Set for people without the Settings menu, where admins find Export.
  onExport?: () => void;
  onLogout: () => void;
}) {
  return (
    <Menu position="bottom-end" width={260} withinPortal>
      <Menu.Target>
        <Button
          aria-label={`Account: ${accountLabel(currentUser)}`}
          className={`${styles.accountButton} ${styles.collapsibleButton}`}
          leftSection={<UserCircleIcon aria-hidden="true" size={18} />}
          rightSection={<CaretDownIcon aria-hidden="true" size={14} />}
          variant="white"
        >
          <span className={styles.collapsibleLabel}>
            <span className={styles.accountEmail}>{accountLabel(currentUser)}</span>
          </span>
        </Button>
      </Menu.Target>
      <Menu.Dropdown>
        <UserMenuLabel currentUser={currentUser} />
        <Menu.Divider />
        {onExport ? (
          <Menu.Item leftSection={<DownloadSimpleIcon aria-hidden="true" size={16} />} onClick={onExport}>
            Export…
          </Menu.Item>
        ) : null}
        {/* In the demo a logout would only open a new demo session. */}
        {demoMode ? null : (
          <Menu.Item
            color="red"
            disabled={isLoggingOut}
            leftSection={<SignOutIcon aria-hidden="true" size={16} />}
            onClick={onLogout}
          >
            {isLoggingOut ? "Logging out" : "Logout"}
          </Menu.Item>
        )}
      </Menu.Dropdown>
    </Menu>
  );
}

// accountLabel names the signed-in person; a demo visitor's address is a
// throwaway one.
function accountLabel(currentUser: AuthUser) {
  return demoMode ? "Demo visitor" : currentUser.email;
}

export function UserMenuLabel({ currentUser }: { currentUser: AuthUser }) {
  return (
    <Menu.Label>
      <Stack gap={4}>
        <Text className={styles.userMenuEmail} size="sm">
          {accountLabel(currentUser)}
        </Text>
        {demoMode ? null : (
          <Badge color={currentUser.role === "admin" ? "blue" : "gray"} size="sm">
            {currentUser.role}
          </Badge>
        )}
      </Stack>
    </Menu.Label>
  );
}
