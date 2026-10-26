import { useState } from "react";
import {
  ActionIcon,
  Badge,
  Button,
  Chip,
  Collapse,
  Group,
  Popover,
  Select,
  Stack,
  Text,
  TextInput,
  Tooltip,
} from "@mantine/core";
import {
  CaretDownIcon,
  CaretRightIcon,
  MagnifyingGlassIcon,
  SlidersHorizontalIcon,
  UserIcon,
  XIcon,
} from "@phosphor-icons/react";

import { useUserName } from "../../../shared/userNames";
import type { AuthUser } from "../../emails/types";

import type {
  BoardEmailState,
  BoardFilterChange,
  BoardFilterCounts,
  BoardFilterOptions,
  BoardFilters,
} from "./BoardHome.types";
import styles from "../../../App.module.css";

const stateOptions: Array<{ label: string; value: BoardEmailState }> = [
  { label: "Draft", value: "draft" },
  { label: "Ready", value: "ready" },
  { label: "Live", value: "live" },
];

export function BoardFilterMenu({
  activeBoardFilterCount,
  boardFilterCounts,
  boardFilterOptions,
  boardFilters,
  onBoardFilterChange,
  onResetBoardFilters,
}: {
  activeBoardFilterCount: number;
  boardFilterCounts: BoardFilterCounts;
  boardFilterOptions: BoardFilterOptions;
  boardFilters: BoardFilters;
  onBoardFilterChange: BoardFilterChange;
  onResetBoardFilters: () => void;
}) {
  const filterButtonLabel =
    activeBoardFilterCount > 0
      ? `Filters ${activeBoardFilterCount}`
      : "Filters";

  return (
    <Popover position="bottom-end" width={300} shadow="md" withinPortal>
      <Popover.Target>
        <div className={styles.boardFilterButtonWrap}>
          <ActionIcon
            aria-label={filterButtonLabel}
            className={styles.boardFilterButton}
            size="lg"
            variant="white"
          >
            <SlidersHorizontalIcon aria-hidden="true" size={18} />
          </ActionIcon>
          {activeBoardFilterCount > 0 ? (
            <Badge
              className={styles.boardFilterBadge}
              color="blue"
              radius="xl"
              size="xs"
              variant="filled"
            >
              {activeBoardFilterCount}
            </Badge>
          ) : null}
        </div>
      </Popover.Target>
      <Popover.Dropdown p="sm">
        <Group justify="space-between" mb="xs">
          <Text fw={650} size="sm">
            Filters
          </Text>
          {activeBoardFilterCount > 0 ? (
            <Button
              color="red"
              size="compact-xs"
              variant="subtle"
              onClick={onResetBoardFilters}
            >
              Clear all
            </Button>
          ) : null}
        </Group>
        <BoardFilterControls
          boardFilterCounts={boardFilterCounts}
          boardFilterOptions={boardFilterOptions}
          boardFilters={boardFilters}
          onBoardFilterChange={onBoardFilterChange}
        />
      </Popover.Dropdown>
    </Popover>
  );
}

export function MyEmailsToggle({
  boardFilters,
  currentUser,
  fullWidth = false,
  onBoardFilterChange,
}: {
  boardFilters: BoardFilters;
  currentUser: AuthUser;
  fullWidth?: boolean;
  onBoardFilterChange: BoardFilterChange;
}) {
  const active = Boolean(boardFilters.mine);

  return (
    <Tooltip disabled={fullWidth} label="My emails">
      <Button
        aria-label="My emails"
        aria-pressed={active}
        className={fullWidth ? undefined : `${styles.myEmailsToggle} ${styles.collapsibleButton}`}
        fullWidth={fullWidth}
        leftSection={<UserIcon aria-hidden="true" size={16} />}
        size={fullWidth ? "xs" : "sm"}
        variant={active ? "filled" : "white"}
        onClick={() =>
          onBoardFilterChange("mine", active ? "" : currentUser.email.trim())
        }
      >
        <span className={fullWidth ? undefined : styles.collapsibleLabel}>My emails</span>
      </Button>
    </Tooltip>
  );
}

export function BoardFilterControls({
  boardFilterCounts,
  boardFilterOptions,
  boardFilters,
  onBoardFilterChange,
}: {
  boardFilterCounts: BoardFilterCounts;
  boardFilterOptions: BoardFilterOptions;
  boardFilters: BoardFilters;
  onBoardFilterChange: BoardFilterChange;
}) {
  const userName = useUserName();
  const hasRegions = boardFilterOptions.regions.length > 0;
  const hasMoreOptions =
    boardFilterOptions.variants.length > 1 ||
    boardFilterOptions.adaptations.length > 1 ||
    hasRegions;
  const [moreOpened, setMoreOpened] = useState(
    Boolean(boardFilters.variant || boardFilters.adaptation || boardFilters.region)
  );

  return (
    <Stack className={styles.boardFilterControls} gap="sm">
      <FilterSection label="Needs attention">
        <Group gap={6}>
          <QuickFilterChip
            checked={boardFilters.openComments}
            count={boardFilterCounts.openComments}
            label="Open comments"
            onChange={(checked) => onBoardFilterChange("openComments", checked)}
          />
          <QuickFilterChip
            checked={boardFilters.staleTranslation}
            count={boardFilterCounts.staleTranslation}
            label="Stale translation"
            onChange={(checked) =>
              onBoardFilterChange("staleTranslation", checked)
            }
          />
          <QuickFilterChip
            checked={boardFilters.needsUpload}
            count={boardFilterCounts.needsUpload}
            label="To upload"
            onChange={(checked) => onBoardFilterChange("needsUpload", checked)}
          />
        </Group>
      </FilterSection>

      {boardFilterOptions.languages.length > 1 ? (
        <FilterSection label="Language">
          <Chip.Group
            multiple
            value={boardFilters.languages}
            onChange={(value) => onBoardFilterChange("languages", value)}
          >
            <Group gap={6}>
              {boardFilterOptions.languages.map((option) => (
                <Chip key={option.value} size="xs" value={option.value}>
                  {option.label}
                </Chip>
              ))}
            </Group>
          </Chip.Group>
        </FilterSection>
      ) : null}

      <FilterSection label="State">
        <Chip.Group
          multiple
          value={boardFilters.states}
          onChange={(value) =>
            onBoardFilterChange("states", value as BoardEmailState[])
          }
        >
          <Group gap={6}>
            {stateOptions.map((option) => (
              <Chip key={option.value} size="xs" value={option.value}>
                {option.label}
              </Chip>
            ))}
          </Group>
        </Chip.Group>
      </FilterSection>

      <FilterSection label="Owner">
        <Select
          clearable
          comboboxProps={{ withinPortal: false }}
          data={boardFilterOptions.owners.map((option) => ({
            label:
              option.value === "__unassigned__"
                ? option.label
                : userName(option.value),
            value: option.value,
          }))}
          placeholder="Any owner"
          searchable
          size="xs"
          value={boardFilters.owner || null}
          onChange={(value) => onBoardFilterChange("owner", value ?? "")}
        />
      </FilterSection>

      {hasMoreOptions ? (
        <div>
          <Button
            color="gray"
            leftSection={
              moreOpened ? (
                <CaretDownIcon aria-hidden="true" size={12} />
              ) : (
                <CaretRightIcon aria-hidden="true" size={12} />
              )
            }
            px={4}
            size="compact-xs"
            variant="subtle"
            onClick={() => setMoreOpened((opened) => !opened)}
          >
            {hasRegions ? "Version and region" : "Version and adaptation"}
          </Button>
          <Collapse expanded={moreOpened}>
            <Stack gap="xs" pt="xs">
              {boardFilterOptions.variants.length > 1 ? (
                <Select
                  clearable
                  comboboxProps={{ withinPortal: false }}
                  data={boardFilterOptions.variants}
                  label="Version"
                  placeholder="Any version"
                  size="xs"
                  value={boardFilters.variant || null}
                  onChange={(value) =>
                    onBoardFilterChange("variant", value ?? "")
                  }
                />
              ) : null}
              {hasRegions ? (
                <Select
                  clearable
                  comboboxProps={{ withinPortal: false }}
                  data={boardFilterOptions.regions}
                  label="Region"
                  placeholder="Any region"
                  size="xs"
                  value={boardFilters.region || null}
                  onChange={(value) =>
                    onBoardFilterChange("region", value ?? "")
                  }
                />
              ) : null}
              {boardFilterOptions.adaptations.length > 1 ? (
                <Select
                  clearable
                  comboboxProps={{ withinPortal: false }}
                  data={boardFilterOptions.adaptations}
                  label="Adaptation"
                  placeholder="Any adaptation"
                  size="xs"
                  value={boardFilters.adaptation || null}
                  onChange={(value) =>
                    onBoardFilterChange("adaptation", value ?? "")
                  }
                />
              ) : null}
            </Stack>
          </Collapse>
        </div>
      ) : null}
    </Stack>
  );
}

function FilterSection({
  children,
  label,
}: {
  children: React.ReactNode;
  label: string;
}) {
  return (
    <div>
      <Text c="dimmed" fw={600} mb={6} size="xs">
        {label}
      </Text>
      {children}
    </div>
  );
}

function QuickFilterChip({
  checked,
  count,
  label,
  onChange,
}: {
  checked: boolean;
  count: number;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <Chip
      checked={checked}
      disabled={count === 0 && !checked}
      size="xs"
      onChange={onChange}
    >
      {label} {count}
    </Chip>
  );
}

export function BoardSearchInput({
  boardSearchQuery,
  size,
  onBoardSearchQueryChange,
}: {
  boardSearchQuery: string;
  size: "xs" | "sm";
  onBoardSearchQueryChange: (query: string) => void;
}) {
  return (
    <TextInput
      className={size === "sm" ? styles.boardSearch : undefined}
      leftSection={<MagnifyingGlassIcon aria-hidden="true" size={16} />}
      placeholder="Search"
      size={size}
      value={boardSearchQuery}
      rightSection={
        boardSearchQuery ? (
          <ActionIcon
            aria-label="Clear search"
            size={size}
            variant="subtle"
            onClick={() => onBoardSearchQueryChange("")}
          >
            <XIcon aria-hidden="true" size={size === "sm" ? 14 : 12} />
          </ActionIcon>
        ) : null
      }
      onChange={(event) => onBoardSearchQueryChange(event.currentTarget.value)}
    />
  );
}
