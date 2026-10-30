import { Badge, Group, Loader, Modal, Text, Tooltip } from "@mantine/core";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { fetchEmailDetail } from "../../../emails/api";
import { emailStatusParts } from "../../../emails/statusSummary";
import type { BoardRegion, EmailListItem } from "../../../emails/types";
import { regionName } from "../../regions";
import { formatDay, formatDelay, type TimelineStep } from "./timeline";
import styles from "./Timeline.module.css";

type TimelineNodeProps = {
  step: TimelineStep;
  regions: BoardRegion[];
  selected: boolean;
  // The first email of a stage goes out when a deal enters it.
  first: boolean;
  // Name the email's region on the card; the branch label does it otherwise.
  showRegion: boolean;
  // Faded because a highlight is on and this email is not one it lights up.
  dimmed: boolean;
  // Present mode: the email as the recipient gets it, without its status,
  // comments or how it compares to the reference.
  present: boolean;
  onOpen: () => void;
};

export function TimelineNode({ step, regions, selected, first, showRegion, dimmed, present, onOpen }: TimelineNodeProps) {
  const { email } = step;
  const status = present ? undefined : emailStatusParts(email, step.versions)[0];
  const region = showRegion ? regionName(email, regions) : "";

  return (
    <div className={styles.node}>
      <div className={styles.dayRow}>
        {present ? (
          <span className={styles.dot} style={{ background: "var(--mantine-color-blue-5)" }} />
        ) : (
          <Tooltip label={status ? `${status.label}. ${status.detail}` : "No status"} multiline maw={320}>
            <span
              className={styles.dot}
              style={{ background: `var(--mantine-color-${status?.color === "dimmed" || !status ? "gray" : status.color}-5)` }}
            />
          </Tooltip>
        )}
        <span>Day {formatDay(step.day)}</span>
        {first && step.day === 0 ? <Text size="xs" c="dimmed">on entry</Text> : null}
        {step.referenceDelay !== undefined && !present ? (
          <Tooltip label={`The reference waits ${formatDelay(step.referenceDelay)} here`}>
            <Badge color="orange" size="xs" variant="light">
              timing differs
            </Badge>
          </Tooltip>
        ) : null}
      </div>
      <button
        className={`${styles.card} ${selected ? styles.cardSelected : ""} ${dimmed ? styles.cardDimmed : ""}`}
        type="button"
        onClick={onOpen}
      >
        <TimelineThumb emailId={email.id} title={email.title} />
        <Text className={styles.subject}>{email.subject || email.title}</Text>
        <Group gap={4}>
          {status ? (
            <Badge color={status.color === "dimmed" ? "gray" : status.color} size="xs" variant="light">
              {status.label}
            </Badge>
          ) : null}
          {email.open_comment_count > 0 && !present ? (
            <Badge color={email.open_blocking_comment_count > 0 ? "red" : "orange"} size="xs" variant="filled">
              {email.open_comment_count} open
            </Badge>
          ) : null}
          {email.send_condition ? (
            <Tooltip label={email.send_condition} multiline maw={320}>
              <Badge color="grape" size="xs" variant="light">
                condition
              </Badge>
            </Tooltip>
          ) : null}
          {email.based_on_stale && !present ? (
            <Tooltip label="The reference email changed since this one last matched it" multiline maw={320}>
              <Badge color="yellow" size="xs" variant="light">
                outdated
              </Badge>
            </Tooltip>
          ) : null}
          {region ? (
            <Badge color="gray" size="xs" variant="outline">
              {region}
            </Badge>
          ) : null}
        </Group>
      </button>
    </div>
  );
}

// TimelineEdge is the line to the next email, with the wait written on it. A
// wait that differs from the reference's is marked; a dashed line leads over
// a slot the branch has no email for.
export function TimelineEdge({
  delay,
  referenceDelay,
  dashed,
}: {
  delay: number | null;
  referenceDelay?: number;
  dashed?: boolean;
}) {
  const label = (
    <span className={`${styles.edgeLabel} ${referenceDelay !== undefined ? styles.edgeDiffers : ""}`}>
      {dashed ? "" : formatDelay(delay)}
    </span>
  );
  return (
    <div className={`${styles.edge} ${delay === null || dashed ? styles.edgeUnknown : ""}`}>
      {referenceDelay !== undefined ? (
        <Tooltip label={`The reference waits ${formatDelay(referenceDelay)}`}>{label}</Tooltip>
      ) : (
        label
      )}
    </div>
  );
}

// TimelineGap holds the place of a slot a branch has no email for, so the
// branches stay lined up.
export function TimelineGap({ name }: { name: string }) {
  return (
    <div className={styles.node}>
      <div className={styles.dayRow}>
        <span className={`${styles.dot} ${styles.dotHollow}`} />
      </div>
      <div className={styles.gap}>
        <Text c="dimmed" size="xs">
          No email in {name}
        </Text>
      </div>
    </div>
  );
}

// TimelinePreview shows one email large, as the recipient sees it, for
// walking someone through the sequence in present mode.
export function TimelinePreview({ email, onClose }: { email: EmailListItem; onClose: () => void }) {
  const html = useEmailHtml(email.id, true);

  return (
    <Modal opened size="xl" title={email.subject || email.title} onClose={onClose}>
      {html.data ? (
        // Third-party HTML, sandboxed as on the cards.
        <iframe className={styles.previewFrame} sandbox="" srcDoc={html.data} title={email.title} />
      ) : (
        <Group className={styles.previewFrame} justify="center">
          {html.isError ? <Text c="dimmed">No preview</Text> : <Loader size="sm" />}
        </Group>
      )}
    </Modal>
  );
}

// How far past the edge of the timeline a thumbnail is still kept ready.
const thumbMargin = 600;

// TimelineThumb renders the email only while it is on screen or close to it. A
// journey can hold dozens of emails and every live frame parses the HTML and
// fetches its images, so frames far off screen are taken down; the HTML itself
// stays in the query cache and comes back without a request.
function TimelineThumb({ emailId, title }: { emailId: string; title: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [near, setNear] = useState(false);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    // The timeline scrolls inside its own box, so the margin must be measured
    // from that box and not from the window.
    const root = node.closest<HTMLElement>("[data-timeline-root]");
    const observer = new IntersectionObserver((entries) => setNear(entries.some((entry) => entry.isIntersecting)), {
      root,
      rootMargin: `${thumbMargin}px`,
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  const detail = useEmailHtml(emailId, near);
  const html = detail.data ?? null;

  return (
    <div className={styles.thumb} ref={ref}>
      {near && html ? (
        // Third-party HTML: no allow-scripts, and no allow-same-origin either,
        // since nothing here has to read the frame.
        <iframe className={styles.thumbFrame} sandbox="" srcDoc={html} tabIndex={-1} title={title} />
      ) : (
        <div className={styles.thumbPlaceholder}>
          {detail.isError ? (
            <Text c="dimmed" size="xs">
              No preview
            </Text>
          ) : near ? (
            <Loader size="xs" />
          ) : null}
        </div>
      )}
    </div>
  );
}

// useEmailHtml reads the email's HTML through the review page's query, so the
// cards, the preview and the review page share one copy of it.
function useEmailHtml(emailId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["emails", emailId, "review"],
    queryFn: () => fetchEmailDetail(emailId),
    enabled,
    staleTime: 60_000,
    select: (detail) => detail.review_html || detail.original_html,
  });
}
