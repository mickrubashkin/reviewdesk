import { ScrollArea } from "@mantine/core";

import { StageColumn } from "../StageColumn";
import type { EmailReviewStatus, StageColumn as StageColumnData } from "../types";
import styles from "./EmailBoard.module.css";

type EmailBoardProps = {
  columns: StageColumnData[];
  scrollPosition: { x: number; y: number };
  selectedVersionByGroup: Record<string, string>;
  onOpenVersionGroup: (groupKey: string, emailId: string) => void;
  onReviewStatusChange: (emailId: string, reviewStatus: EmailReviewStatus) => void;
  onScrollPositionChange: (position: { x: number; y: number }) => void;
  onSelectVersion: (groupKey: string, emailId: string) => void;
};

export function EmailBoard({
  columns,
  scrollPosition,
  selectedVersionByGroup,
  onOpenVersionGroup,
  onReviewStatusChange,
  onScrollPositionChange,
  onSelectVersion,
}: EmailBoardProps) {
  return (
    <ScrollArea
      className={styles.boardScroll}
      onScrollPositionChange={onScrollPositionChange}
      startScrollPosition={scrollPosition}
      type="auto"
    >
      <div className={styles.board}>
        {columns.map((column, columnIndex) => (
          <StageColumn
            column={column}
            columnIndex={columnIndex}
            key={column.stage}
            selectedVersionByGroup={selectedVersionByGroup}
            onOpenVersionGroup={onOpenVersionGroup}
            onReviewStatusChange={onReviewStatusChange}
            onSelectVersion={onSelectVersion}
          />
        ))}
      </div>
    </ScrollArea>
  );
}

export default EmailBoard;
