import { Badge, Button, CopyButton, Group, ScrollArea, SegmentedControl, Select, Text, Tooltip } from "@mantine/core";
import { Fragment, useMemo, useState, type CSSProperties } from "react";
import { useSearchParams } from "react-router-dom";

import { HelpLink } from "../../../help/HelpLink";
import type { BoardRegion, StageColumn } from "../../../emails/types";
import { timelineByRegionStorageKey } from "../BoardHome.types";
import {
  readStoredTimelineByRegion,
  readStoredTimelineCollapsed,
  writeSessionStorageValue,
  writeStoredTimelineCollapsed,
} from "../BoardHome.storage";
import { matrixLanguages } from "../slotHelpers";
import { TimelineEdge, TimelineGap, TimelineNode, TimelinePreview } from "./TimelineNode";
import {
  buildTimeline,
  countMatches,
  formatDay,
  highlightOptions,
  isHighlight,
  matchesHighlight,
  summarizeStage,
  type Highlight,
  type TimelineBranch,
  type TimelineStage,
} from "./timeline";
import styles from "./Timeline.module.css";

type BoardTimelineProps = {
  boardKey: string;
  boardName: string;
  columns: StageColumn[];
  // Present mode leaves only the sequence itself: no statuses, comments or
  // review tools, and an email opens as a large preview.
  present: boolean;
  onPresentChange: (present: boolean) => void;
  regions: BoardRegion[];
  selectedEmailId: string | null;
  onOpenEmail: (emailId: string) => void;
};

// BoardTimeline draws the board like a branch graph: the stages are the nodes
// of the trunk, a deal's way through the pipeline, and from each node runs
// the branch of emails the robots send while the deal sits in that stage. On a
// board with regions a stage has a branch per region, lined up slot by slot
// to compare them; otherwise one branch of the language chosen. It works on
// the board's filtered columns, so the region, owner and state filters narrow
// it like the other views; present mode gets the whole board.
export function BoardTimeline({
  boardKey,
  boardName,
  columns,
  present,
  onPresentChange,
  regions,
  selectedEmailId,
  onOpenEmail,
}: BoardTimelineProps) {
  const languages = useMemo(() => matrixLanguages(columns), [columns]);
  const [picked, setPicked] = useState("en");
  const language = languages.includes(picked) ? picked : (languages[0] ?? "en");
  const hasRegions = regions.length > 0;
  const [byRegionPicked, setByRegionPicked] = useState(readStoredTimelineByRegion);
  const byRegion = hasRegions && byRegionPicked;
  const stages = useMemo(
    () => buildTimeline(columns, byRegion ? { kind: "region", regions } : { kind: "language", language }),
    [columns, language, byRegion, regions]
  );
  const [collapsed, setCollapsed] = useState(() => new Set(readStoredTimelineCollapsed(boardKey)));

  const updateCollapsed = (next: Set<string>) => {
    setCollapsed(next);
    writeStoredTimelineCollapsed(boardKey, [...next]);
  };
  const toggle = (stage: string) => {
    const next = new Set(collapsed);
    if (!next.delete(stage)) next.add(stage);
    updateCollapsed(next);
  };
  const [pickedHighlight, setHighlight] = useState<Highlight>("none");
  const highlight = present ? "none" : pickedHighlight;
  const [previewId, setPreviewId] = useState<string | null>(null);
  const preview = previewId ? findEmail(stages, previewId) : undefined;
  const matches = useMemo(() => countMatches(stages, highlight), [stages, highlight]);

  // A link with ?stage= (from the review page or "Copy link") brings that
  // stage into view; BoardHome scrolls to it, here it is unfolded.
  const [searchParams] = useSearchParams();
  const stageToReveal = searchParams.get("stage");
  const [revealed, setRevealed] = useState<string | null>(null);
  if (stageToReveal && stageToReveal !== revealed) {
    setRevealed(stageToReveal);
    if (collapsed.has(stageToReveal)) {
      const next = new Set(collapsed);
      next.delete(stageToReveal);
      setCollapsed(next);
    }
  }

  const allCollapsed = stages.every((stage) => collapsed.has(stage.stage));

  return (
    <div className={styles.timeline} data-timeline-root>
      <Group justify="space-between" mb="sm">
        {present ? (
          <Text c="white" fw={700} size="lg">
            {boardName}
          </Text>
        ) : (
          <Text c="rgba(255, 255, 255, 0.72)" size="sm">
            Stages along the trunk; each branch is what a recipient gets there, in send order{byRegion ? ", one branch per region" : ""}. Days count from a deal entering the stage.
          </Text>
        )}
        <Group gap="xs">
          {present ? null : (
            <>
              <HelpLink onDark topic="timeline" />
              {highlight !== "none" ? (
                <Text c="rgba(255, 255, 255, 0.85)" size="xs">
                  {matches.matching} of {matches.total} emails
                </Text>
              ) : null}
              <Select
                allowDeselect={false}
                aria-label="Highlight"
                data={highlightOptions.map((option) => ({
                  value: option.value,
                  label: option.value === "none" ? "Highlight: nothing" : `Highlight: ${option.label.toLowerCase()}`,
                }))}
                size="xs"
                value={highlight}
                w={190}
                onChange={(value) => setHighlight(isHighlight(value) ? value : "none")}
              />
              <Button
                color="gray"
                size="compact-xs"
                variant="default"
                onClick={() => updateCollapsed(allCollapsed ? new Set() : new Set(stages.map((stage) => stage.stage)))}
              >
                {allCollapsed ? "Expand all" : "Collapse all"}
              </Button>
            </>
          )}
          {hasRegions ? (
            <SegmentedControl
              aria-label="Branches"
              data={[
                { label: "By region", value: "regions" },
                { label: "By language", value: "languages" },
              ]}
              size="xs"
              value={byRegion ? "regions" : "languages"}
              onChange={(value) => {
                setByRegionPicked(value === "regions");
                writeSessionStorageValue(timelineByRegionStorageKey, value === "regions");
              }}
            />
          ) : null}
          {!byRegion && languages.length > 1 ? (
            <SegmentedControl
              aria-label="Language"
              data={languages.map((l) => ({ label: l.toUpperCase(), value: l }))}
              size="xs"
              value={language}
              onChange={setPicked}
            />
          ) : null}
          <Button
            color="gray"
            size="compact-xs"
            variant={present ? "white" : "default"}
            onClick={() => onPresentChange(!present)}
          >
            {present ? "Exit present" : "Present"}
          </Button>
        </Group>
      </Group>
      <div className={styles.graph}>
        {stages.map((stage, index) => (
          <StageBranch
            collapsed={collapsed.has(stage.stage)}
            first={index === 0}
            highlight={highlight}
            key={stage.stage}
            boardKey={boardKey}
            byRegion={byRegion}
            language={language}
            last={index === stages.length - 1}
            present={present}
            regions={regions}
            selectedEmailId={present ? null : selectedEmailId}
            stage={stage}
            onOpenEmail={present ? setPreviewId : onOpenEmail}
            onToggle={() => toggle(stage.stage)}
          />
        ))}
      </div>
      {preview ? <TimelinePreview email={preview} onClose={() => setPreviewId(null)} /> : null}
    </div>
  );
}

function findEmail(stages: TimelineStage[], emailId: string) {
  for (const stage of stages) {
    for (const branch of stage.branches) {
      const cell = branch.cells.find((c) => c?.email.id === emailId);
      if (cell) return cell.email;
    }
  }
  return undefined;
}

type StageBranchProps = {
  stage: TimelineStage;
  boardKey: string;
  language: string;
  byRegion: boolean;
  first: boolean;
  last: boolean;
  collapsed: boolean;
  highlight: Highlight;
  present: boolean;
  regions: BoardRegion[];
  selectedEmailId: string | null;
  onOpenEmail: (emailId: string) => void;
  onToggle: () => void;
};

function StageBranch({
  stage,
  boardKey,
  language,
  byRegion,
  first,
  last,
  collapsed,
  highlight,
  present,
  regions,
  selectedEmailId,
  onOpenEmail,
  onToggle,
}: StageBranchProps) {
  const summary = summarizeStage(stage);
  const hasBranch = stage.branches.length > 0;
  const stageMatches = stage.branches.reduce(
    (sum, branch) => sum + branch.cells.filter((cell) => cell && matchesHighlight(cell.email, highlight)).length,
    0
  );
  const allLive = summary.emails > 0 && summary.live === summary.emails;

  return (
    <section data-stage={stage.stage} className={`${styles.stage} ${first ? styles.stageFirst : ""} ${last ? styles.stageLast : ""}`}>
      <span className={styles.stageDot} />
      <div className={styles.stageRow}>
        <button
          aria-expanded={hasBranch ? !collapsed : undefined}
          className={styles.stageHead}
          disabled={!hasBranch}
          type="button"
          onClick={onToggle}
        >
          <span className={styles.chevron}>{hasBranch ? (collapsed ? "▸" : "▾") : ""}</span>
          <Text fw={700}>{stage.title}</Text>
          {hasBranch ? (
            <Group gap={6}>
              <Text c="dimmed" size="xs">
                {summary.emails} {summary.emails === 1 ? "email" : "emails"}
                {byRegion ? ` in ${summary.branches} ${summary.branches === 1 ? "region" : "regions"}` : ""} · Day 0–
                {formatDay(summary.lastDay)}
              </Text>
              {present ? null : (
                <>
                  <Tooltip label={`${summary.live} of ${summary.emails} emails are sent by the robots now`}>
                    <Badge color={allLive ? "green" : summary.live > 0 ? "yellow" : "gray"} size="xs" variant="light">
                      {summary.live}/{summary.emails} live
                    </Badge>
                  </Tooltip>
                  {summary.toUpload > 0 ? (
                    <Tooltip label="Ready emails the Bitrix24 robots do not have in this version yet">
                      <Badge color="orange" size="xs" variant="light">
                        {summary.toUpload} to upload
                      </Badge>
                    </Tooltip>
                  ) : null}
                  {highlight !== "none" ? (
                    <Badge color={stageMatches > 0 ? "violet" : "gray"} size="xs" variant={stageMatches > 0 ? "filled" : "outline"}>
                      {stageMatches} match
                    </Badge>
                  ) : null}
                  {summary.openComments > 0 ? (
                    <Badge color={summary.blockingComments > 0 ? "red" : "orange"} size="xs" variant="filled">
                      {summary.openComments} open
                    </Badge>
                  ) : null}
                </>
              )}
            </Group>
          ) : (
            <Text c="dimmed" size="xs">
              {byRegion ? "No emails" : `No emails in ${language.toUpperCase()}`}
            </Text>
          )}
          {!present && !byRegion && stage.missing > 0 && hasBranch ? (
            <Text c="dimmed" size="xs">
              {stage.missing} without {language.toUpperCase()}
            </Text>
          ) : null}
        </button>
        {present ? null : (
          <CopyButton value={stageLink(boardKey, stage.stage)} timeout={1500}>
            {({ copied, copy }) => (
              <Button className={styles.stageLink} color="gray" size="compact-xs" variant="subtle" onClick={copy}>
                {copied ? "Link copied" : "Copy link"}
              </Button>
            )}
          </CopyButton>
        )}
      </div>

      {hasBranch && !collapsed
        ? stage.branches.map((branch) => (
            <Branch
              branch={branch}
              highlight={highlight}
              key={branch.key}
              labelled={byRegion}
              present={present}
              regions={regions}
              selectedEmailId={selectedEmailId}
              onOpenEmail={onOpenEmail}
            />
          ))
        : null}
    </section>
  );
}

type BranchProps = {
  branch: TimelineBranch;
  highlight: Highlight;
  // Name the branch above its first email (the branches of regions).
  labelled: boolean;
  present: boolean;
  regions: BoardRegion[];
  selectedEmailId: string | null;
  onOpenEmail: (emailId: string) => void;
};

function Branch({ branch, highlight, labelled, present, regions, selectedEmailId, onOpenEmail }: BranchProps) {
  const firstIndex = branch.cells.findIndex((cell) => cell !== null);
  const style = {
    "--branch": `var(--mantine-color-${branch.color}-6)`,
    "--branch-line": `var(--mantine-color-${branch.color}-3)`,
  } as CSSProperties;

  return (
    <div className={styles.branch} style={style}>
      <span className={`${styles.elbow} ${labelled ? styles.elbowLabelled : ""}`} />
      {labelled ? (
        <div className={styles.branchLabel}>
          <Text fw={700} size="xs" style={{ color: "var(--branch)" }}>
            {branch.name.toLowerCase() === branch.language ? branch.name : `${branch.name} · ${branch.language.toUpperCase()}`}
          </Text>
          {branch.reference && !present ? (
            <Badge color="blue" size="xs" variant="light">
              reference
            </Badge>
          ) : null}
          {branch.managers.length > 0 && !present ? (
            <Text c="dimmed" size="xs">
              {branch.managers.join(", ")}
            </Text>
          ) : null}
        </div>
      ) : null}
      <ScrollArea type="auto">
        <div className={styles.journey}>
          {branch.cells.map((cell, index) => (
            <Fragment key={cell?.key ?? `gap-${index}`}>
              {index > 0 ? (
                <TimelineEdge
                  dashed={cell === null || branch.cells[index - 1] === null}
                  delay={cell?.delay ?? null}
                  referenceDelay={present ? undefined : cell?.referenceDelay}
                />
              ) : null}
              <div className={styles.step}>
                {cell ? (
                  <TimelineNode
                    dimmed={!matchesHighlight(cell.email, highlight)}
                    first={index === firstIndex}
                    present={present}
                    regions={regions}
                    selected={cell.versions.some((v) => v.id === selectedEmailId)}
                    showRegion={!labelled}
                    step={cell}
                    onOpen={() => onOpenEmail(cell.email.id)}
                  />
                ) : (
                  <TimelineGap name={branch.name} />
                )}
              </div>
            </Fragment>
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

// stageLink opens the board's timeline with the stage in view and unfolded.
function stageLink(boardKey: string, stage: string) {
  return `${window.location.origin}/boards/${encodeURIComponent(boardKey)}?view=timeline&stage=${encodeURIComponent(stage)}`;
}
