import type { Dispatch, SetStateAction } from "react";
import { Menu, MultiSelect, Text } from "@mantine/core";

import { formatStageName } from "../../emails/stages";
import type { Board } from "../../emails/types";

import { copyPlainText, downloadJSON } from "./BoardHome.helpers";
import {
  defaultHandoffFilters,
  type BoardFilterOptions,
  type HandoffFilters,
} from "./BoardHome.types";
import styles from "../../../App.module.css";

export function HandoffMenuContent({
  activeBoard,
  activeHandoffFilterCount,
  boardFilterOptions,
  boardKey,
  canExportSequenceHandoff,
  handoffFilters,
  sequenceHandoffEmailCount,
  sequenceHandoffJSON,
  onHandoffFiltersChange,
}: {
  activeBoard: Board | undefined;
  activeHandoffFilterCount: number;
  boardFilterOptions: BoardFilterOptions;
  boardKey: string;
  canExportSequenceHandoff: boolean;
  handoffFilters: HandoffFilters;
  sequenceHandoffEmailCount: number;
  sequenceHandoffJSON: string;
  onHandoffFiltersChange: Dispatch<SetStateAction<HandoffFilters>>;
}) {
  return (
    <>
      <Menu.Label>Handoff</Menu.Label>
      <div className={styles.boardFilterMenuControl}>
        <MultiSelect
          clearable
          data={boardFilterOptions.languages}
          placeholder="All languages"
          size="xs"
          value={handoffFilters.languages}
          onChange={(languages) =>
            onHandoffFiltersChange((current) => ({
              ...current,
              languages,
            }))
          }
        />
      </div>
      <div className={styles.boardFilterMenuControl}>
        <MultiSelect
          clearable
          data={boardFilterOptions.adaptations}
          placeholder="All adaptations"
          size="xs"
          value={handoffFilters.adaptations}
          onChange={(adaptations) =>
            onHandoffFiltersChange((current) => ({
              ...current,
              adaptations,
            }))
          }
        />
      </div>
      <div className={styles.boardFilterMenuControl}>
        <MultiSelect
          clearable
          data={(activeBoard?.stages ?? []).map((stage) => ({
            label: formatStageName(stage),
            value: stage,
          }))}
          placeholder="All stages"
          size="xs"
          value={handoffFilters.stages}
          onChange={(stages) =>
            onHandoffFiltersChange((current) => ({
              ...current,
              stages,
            }))
          }
        />
      </div>
      <Text c="dimmed" px="xs" size="xs">
        {sequenceHandoffEmailCount} emails selected
      </Text>
      {activeHandoffFilterCount > 0 ? (
        <Menu.Item
          color="red"
          onClick={() => onHandoffFiltersChange(defaultHandoffFilters)}
        >
          Clear handoff filters
        </Menu.Item>
      ) : null}
      <Menu.Item
        disabled={!canExportSequenceHandoff}
        onClick={() => void copyPlainText(sequenceHandoffJSON, "Sequence handoff")}
      >
        Copy sequence handoff
      </Menu.Item>
      <Menu.Item
        disabled={!canExportSequenceHandoff}
        onClick={() => downloadJSON(sequenceHandoffJSON, `${boardKey}-handoff.json`)}
      >
        Download sequence handoff
      </Menu.Item>
    </>
  );
}
