import { Button, Group, Pill } from "@mantine/core";

import { useUserName } from "../../../shared/userNames";

import type {
  BoardFilterChange,
  BoardFilterOptions,
  BoardFilters,
} from "./BoardHome.types";
import styles from "../../../App.module.css";

const stateLabels = { draft: "Draft", ready: "Ready", live: "Live" };

// ActiveBoardFilters lists the applied filters above the board so they stay
// visible after the filter popover closes.
export function ActiveBoardFilters({
  boardFilterOptions,
  boardFilters,
  onBoardFilterChange,
  onResetBoardFilters,
}: {
  boardFilterOptions: BoardFilterOptions;
  boardFilters: BoardFilters;
  onBoardFilterChange: BoardFilterChange;
  onResetBoardFilters: () => void;
}) {
  const userName = useUserName();
  const optionLabel = (
    options: Array<{ label: string; value: string }>,
    value: string
  ) => options.find((option) => option.value === value)?.label ?? value;

  const pills: Array<{ key: string; label: string; onRemove: () => void }> = [];
  if (boardFilters.mine) {
    pills.push({
      key: "mine",
      label: "My emails",
      onRemove: () => onBoardFilterChange("mine", ""),
    });
  }
  if (boardFilters.openComments) {
    pills.push({
      key: "openComments",
      label: "Open comments",
      onRemove: () => onBoardFilterChange("openComments", false),
    });
  }
  if (boardFilters.staleTranslation) {
    pills.push({
      key: "staleTranslation",
      label: "Stale translation",
      onRemove: () => onBoardFilterChange("staleTranslation", false),
    });
  }
  if (boardFilters.needsUpload) {
    pills.push({
      key: "needsUpload",
      label: "To upload",
      onRemove: () => onBoardFilterChange("needsUpload", false),
    });
  }
  for (const language of boardFilters.languages) {
    pills.push({
      key: `language-${language}`,
      label: language.toUpperCase(),
      onRemove: () =>
        onBoardFilterChange(
          "languages",
          boardFilters.languages.filter((value) => value !== language)
        ),
    });
  }
  for (const state of boardFilters.states) {
    pills.push({
      key: `state-${state}`,
      label: stateLabels[state],
      onRemove: () =>
        onBoardFilterChange(
          "states",
          boardFilters.states.filter((value) => value !== state)
        ),
    });
  }
  if (boardFilters.owner) {
    pills.push({
      key: "owner",
      label: `Owner: ${
        boardFilters.owner === "__unassigned__"
          ? "Unassigned"
          : userName(boardFilters.owner)
      }`,
      onRemove: () => onBoardFilterChange("owner", ""),
    });
  }
  if (boardFilters.variant) {
    pills.push({
      key: "variant",
      label: `Version: ${optionLabel(boardFilterOptions.variants, boardFilters.variant)}`,
      onRemove: () => onBoardFilterChange("variant", ""),
    });
  }
  if (boardFilters.region) {
    pills.push({
      key: "region",
      label: `Region: ${optionLabel(boardFilterOptions.regions, boardFilters.region)}`,
      onRemove: () => onBoardFilterChange("region", ""),
    });
  }
  if (boardFilters.adaptation) {
    pills.push({
      key: "adaptation",
      label: `Adaptation: ${optionLabel(boardFilterOptions.adaptations, boardFilters.adaptation)}`,
      onRemove: () => onBoardFilterChange("adaptation", ""),
    });
  }

  if (pills.length === 0) {
    return null;
  }

  return (
    <Group className={styles.activeBoardFilters} gap={6}>
      {pills.map((pill) => (
        <Pill
          className={styles.activeBoardFilterPill}
          key={pill.key}
          withRemoveButton
          onRemove={pill.onRemove}
        >
          {pill.label}
        </Pill>
      ))}
      {pills.length > 1 ? (
        <Button
          className={styles.activeBoardFiltersClear}
          size="compact-xs"
          variant="subtle"
          onClick={onResetBoardFilters}
        >
          Clear all
        </Button>
      ) : null}
    </Group>
  );
}
