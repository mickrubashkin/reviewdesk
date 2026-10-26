import { Stack, Tabs } from "@mantine/core";

import {
  ChatTextIcon,
  ClockCounterClockwiseIcon,
  EnvelopeSimpleIcon,
  ListChecksIcon,
  SparkleIcon,
} from "@phosphor-icons/react";

import type { CSSProperties, PointerEvent, ReactNode, RefObject } from "react";

import type { ReviewContentTab, ReviewPanelTab } from "./EmailReview.types";

import styles from "./EmailReview.module.css";

type ReviewLayoutProps = {
  activeContentTab: ReviewContentTab;
  activePanelTab: ReviewPanelTab;
  analysisContent: ReactNode;
  commentsContent: ReactNode;
  contentRef: RefObject<HTMLElement | null>;
  detailsContent: ReactNode;
  historyContent: ReactNode;
  isCompactReview: boolean;
  isResizing: boolean;
  openCommentCount: number;
  previewContent: ReactNode;
  previewToolbar: ReactNode;
  rightPanelPercent: number;
  onActiveContentTabChange: (tab: ReviewContentTab) => void;
  onActivePanelTabChange: (tab: ReviewPanelTab) => void;
  onResizeEnd: (event: PointerEvent<HTMLDivElement>) => void;
  onResizeMove: (event: PointerEvent<HTMLDivElement>) => void;
  onResizeStart: (event: PointerEvent<HTMLDivElement>) => void;
};

export function ReviewLayout({
  activeContentTab,
  activePanelTab,
  analysisContent,
  commentsContent,
  contentRef,
  detailsContent,
  historyContent,
  isCompactReview,
  isResizing,
  openCommentCount,
  previewContent,
  previewToolbar,
  rightPanelPercent,
  onActiveContentTabChange,
  onActivePanelTabChange,
  onResizeEnd,
  onResizeMove,
  onResizeStart,
}: ReviewLayoutProps) {
  const commentsLabel =
    openCommentCount > 0 ? `Comments ${openCommentCount}` : "Comments";
  const panelTabs = (
    tabClassName: string | undefined
  ): ReactNode[] => [
    <Tabs.Tab
      className={tabClassName}
      key="comments"
      leftSection={<ChatTextIcon aria-hidden="true" size={15} />}
      value="comments"
    >
      {commentsLabel}
    </Tabs.Tab>,
    <Tabs.Tab
      className={tabClassName}
      key="ai"
      leftSection={<SparkleIcon aria-hidden="true" size={15} />}
      value="ai"
    >
      AI
    </Tabs.Tab>,
    <Tabs.Tab
      className={tabClassName}
      key="details"
      leftSection={<ListChecksIcon aria-hidden="true" size={15} />}
      value="details"
    >
      Details
    </Tabs.Tab>,
    <Tabs.Tab
      className={tabClassName}
      key="history"
      leftSection={<ClockCounterClockwiseIcon aria-hidden="true" size={15} />}
      value="history"
    >
      History
    </Tabs.Tab>,
  ];
  const panelContents = (panelClassName: string) => (
    <>
      <Tabs.Panel value="comments" className={panelClassName}>
        {commentsContent}
      </Tabs.Panel>
      <Tabs.Panel value="ai" className={panelClassName}>
        {analysisContent}
      </Tabs.Panel>
      <Tabs.Panel value="details" className={panelClassName}>
        {detailsContent}
      </Tabs.Panel>
      <Tabs.Panel value="history" className={panelClassName}>
        {historyContent}
      </Tabs.Panel>
    </>
  );

  if (isCompactReview) {
    return (
      <main className={styles.mobileContent}>
        <Tabs
          value={activeContentTab}
          onChange={(value) =>
            onActiveContentTabChange((value as ReviewContentTab | null) ?? "email")
          }
        >
          <Tabs.List className={styles.mobileTabsList} grow>
            <Tabs.Tab
              value="email"
              leftSection={<EnvelopeSimpleIcon aria-hidden="true" size={15} />}
            >
              Email
            </Tabs.Tab>
            {panelTabs(undefined)}
          </Tabs.List>

          <Tabs.Panel value="email" className={styles.mobileTabPanel}>
            {previewContent}
          </Tabs.Panel>
          {panelContents(styles.mobileTabPanel)}
        </Tabs>
      </main>
    );
  }

  return (
    <main
      className={styles.content}
      ref={contentRef}
      style={
        {
          "--review-panel-width": `${rightPanelPercent}%`,
        } as CSSProperties
      }
    >
      <section className={styles.previewColumn}>
        <div className={styles.previewToolbar}>{previewToolbar}</div>
        {previewContent}
      </section>

      <div
        aria-label="Resize review panel"
        className={styles.splitter}
        data-active={isResizing || undefined}
        onPointerCancel={onResizeEnd}
        onPointerDown={onResizeStart}
        onPointerMove={onResizeMove}
        onPointerUp={onResizeEnd}
        role="separator"
      />

      <aside className={styles.reviewPanel}>
        <Stack gap={0}>
          <Tabs
            value={activePanelTab}
            onChange={(value) =>
              onActivePanelTabChange((value as ReviewPanelTab | null) ?? "comments")
            }
          >
            <Tabs.List className={styles.panelTabsList}>
              {panelTabs(styles.panelTab)}
            </Tabs.List>
            {panelContents(styles.tabPanel)}
          </Tabs>
        </Stack>
      </aside>
    </main>
  );
}
