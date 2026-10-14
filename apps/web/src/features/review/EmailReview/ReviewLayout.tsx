import { Group, Stack, Tabs, Text } from "@mantine/core";

import {
  CaretDownIcon,
  ChatTextIcon,
  ChecksIcon,
  ClockCounterClockwiseIcon,
  EnvelopeSimpleIcon,
  HandshakeIcon,
  ListChecksIcon,
  SparkleIcon,
} from "@phosphor-icons/react";

import type { CSSProperties, PointerEvent, ReactNode, RefObject } from "react";

import type { ReviewContentTab, ReviewPanelTab, ReviewUtilityPanel } from "./EmailReview.types";
import { formatUtilityPanelLabel } from "./EmailReview.helpers";

import styles from "./EmailReview.module.css";

type ReviewLayoutProps = {
  activeContentTab: ReviewContentTab;
  activePanelTab: ReviewPanelTab;
  activeUtilityPanel: ReviewUtilityPanel | null;
  analysisContent: ReactNode;
  commentsContent: ReactNode;
  contentRef: RefObject<HTMLElement | null>;
  isCompactReview: boolean;
  isResizing: boolean;
  openCommentCount: number;
  previewContent: ReactNode;
  rightPanelPercent: number;
  selectedUtilityPanel: ReviewUtilityPanel;
  utilityContentByPanel: Record<ReviewUtilityPanel, ReactNode>;
  utilityPanelContent: ReactNode;
  onActiveContentTabChange: (tab: ReviewContentTab) => void;
  onActivePanelTabChange: (tab: ReviewPanelTab) => void;
  onResizeEnd: (event: PointerEvent<HTMLDivElement>) => void;
  onResizeMove: (event: PointerEvent<HTMLDivElement>) => void;
  onResizeStart: (event: PointerEvent<HTMLDivElement>) => void;
  onUtilityPanelChange: (panel: ReviewUtilityPanel | null) => void;
  onUtilityPanelSelect: (panel: ReviewUtilityPanel) => void;
};

export function ReviewLayout({
  activeContentTab,
  activePanelTab,
  activeUtilityPanel,
  analysisContent,
  commentsContent,
  contentRef,
  isCompactReview,
  isResizing,
  openCommentCount,
  previewContent,
  rightPanelPercent,
  selectedUtilityPanel,
  utilityContentByPanel,
  utilityPanelContent,
  onActiveContentTabChange,
  onActivePanelTabChange,
  onResizeEnd,
  onResizeMove,
  onResizeStart,
  onUtilityPanelChange,
  onUtilityPanelSelect,
}: ReviewLayoutProps) {
  const desktopPrimaryContent = activeUtilityPanel ? (
    <UtilityPanelFrame title={formatUtilityPanelLabel(activeUtilityPanel)}>
      {utilityContentByPanel[activeUtilityPanel]}
    </UtilityPanelFrame>
  ) : (
    previewContent
  );

  return (
    <>
      {isCompactReview ? (
        <main className={styles.mobileContent}>
          <Tabs
            value={activeContentTab}
            onChange={(value) => {
              const nextValue = (value as ReviewContentTab | null) ?? "email";
              onActiveContentTabChange(nextValue);
              if (nextValue === "email") {
                onUtilityPanelChange(null);
              }
              if (nextValue === "more" && !activeUtilityPanel) {
                onUtilityPanelChange("planning");
              }
            }}
          >
            <Tabs.List className={styles.mobileTabsList} grow>
              <Tabs.Tab
                value="email"
                leftSection={<EnvelopeSimpleIcon aria-hidden="true" size={15} />}
              >
                Email
              </Tabs.Tab>
              <Tabs.Tab
                value="comments"
                leftSection={
                  <ChatTextIcon aria-hidden="true" size={15} />
                }
              >
                {openCommentCount > 0
                  ? `Comments ${openCommentCount}`
                  : "Comments"}
              </Tabs.Tab>
              <Tabs.Tab
                value="ai"
                leftSection={<SparkleIcon aria-hidden="true" size={15} />}
              >
                AI
              </Tabs.Tab>
              <Tabs.Tab
                value="more"
                leftSection={<CaretDownIcon aria-hidden="true" size={15} />}
              >
                More
              </Tabs.Tab>
            </Tabs.List>

            <Tabs.Panel value="email" className={styles.mobileTabPanel}>
              {previewContent}
            </Tabs.Panel>

            <Tabs.Panel value="comments" className={styles.mobileTabPanel}>
              {commentsContent}
            </Tabs.Panel>
            <Tabs.Panel value="ai" className={styles.mobileTabPanel}>
              {analysisContent}
            </Tabs.Panel>
            <Tabs.Panel value="more" className={styles.mobileTabPanel}>
              <UtilityPanelFrame title={formatUtilityPanelLabel(selectedUtilityPanel)}>
                <Group className={styles.mobileUtilityNav} gap={2} wrap="nowrap">
                  <UtilityNavButton
                    active={selectedUtilityPanel === "planning"}
                    icon={<ListChecksIcon aria-hidden="true" size={15} />}
                    label="Plan"
                    onClick={() => onUtilityPanelSelect("planning")}
                  />
                  <UtilityNavButton
                    active={selectedUtilityPanel === "approvals"}
                    icon={<ChecksIcon aria-hidden="true" size={15} />}
                    label="Approvals"
                    onClick={() => onUtilityPanelSelect("approvals")}
                  />
                  <UtilityNavButton
                    active={selectedUtilityPanel === "handoff"}
                    icon={<HandshakeIcon aria-hidden="true" size={15} />}
                    label="Handoff"
                    onClick={() => onUtilityPanelSelect("handoff")}
                  />
                  <UtilityNavButton
                    active={selectedUtilityPanel === "history"}
                    icon={
                      <ClockCounterClockwiseIcon aria-hidden="true" size={15} />
                    }
                    label="History"
                    onClick={() => onUtilityPanelSelect("history")}
                  />
                  <UtilityNavButton
                    active={selectedUtilityPanel === "activity"}
                    icon={
                      <ClockCounterClockwiseIcon aria-hidden="true" size={15} />
                    }
                    label="Activity"
                    onClick={() => onUtilityPanelSelect("activity")}
                  />
                </Group>
                {utilityPanelContent}
              </UtilityPanelFrame>
            </Tabs.Panel>
          </Tabs>
        </main>
      ) : (
        <main
          className={styles.content}
          ref={contentRef}
          style={
            {
              "--review-panel-width": `${rightPanelPercent}%`,
            } as CSSProperties
          }
        >
          <section className={styles.previewColumn}>{desktopPrimaryContent}</section>

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
                  onActivePanelTabChange(
                    (value as ReviewPanelTab | null) ?? "comments"
                  )
                }
              >
                <Tabs.List className={styles.panelTabsList}>
                  <Tabs.Tab
                    className={styles.panelTab}
                    value="comments"
                    leftSection={
                      <ChatTextIcon aria-hidden="true" size={15} />
                    }
                  >
                    {openCommentCount > 0
                      ? `Comments ${openCommentCount}`
                      : "Comments"}
                  </Tabs.Tab>
                  <Tabs.Tab
                    className={styles.panelTab}
                    value="ai"
                    leftSection={<SparkleIcon aria-hidden="true" size={15} />}
                  >
                    AI
                  </Tabs.Tab>
                </Tabs.List>

                <Tabs.Panel value="comments" className={styles.tabPanel}>
                  {commentsContent}
                </Tabs.Panel>
                <Tabs.Panel value="ai" className={styles.tabPanel}>
                  {analysisContent}
                </Tabs.Panel>
              </Tabs>
            </Stack>
          </aside>
        </main>
      )}
    </>
  );
}

function UtilityNavButton({
  active,
  icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      className={styles.utilityNavButton}
      data-active={active || undefined}
      type="button"
      onClick={onClick}
    >
      {icon}
      <span>{label}</span>
    </button>
  );
}


function UtilityPanelFrame({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  return (
    <section className={styles.utilityPanelFrame}>
      <Group className={styles.utilityPanelHeader} justify="space-between">
        <Text fw={700} size="sm">
          {title}
        </Text>
      </Group>
      <div className={styles.utilityPanelBody}>{children}</div>
    </section>
  );
}
