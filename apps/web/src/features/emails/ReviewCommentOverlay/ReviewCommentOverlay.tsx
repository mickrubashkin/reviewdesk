import type { CSSProperties, RefObject } from "react";

import styles from "../EmailPreviewDrawer.module.css";
import type {
  ReviewChangedBlockRect,
  ReviewOverlayBadge,
  ReviewOverlayRect,
} from "../reviewOverlayTypes";

type ReviewCommentOverlayProps = {
  activeCommentId?: string | null;
  badges: ReviewOverlayBadge[];
  changedBlockRects?: ReviewChangedBlockRect[];
  hoveredCommentId?: string | null;
  layerRef?: RefObject<HTMLDivElement | null>;
  onBadgeClick?: (commentIds: string[]) => void;
  onBadgeHover?: (commentIds: string[] | null) => void;
  rects: ReviewOverlayRect[];
};

export function ReviewCommentOverlay({
  activeCommentId = null,
  badges,
  changedBlockRects = [],
  hoveredCommentId = null,
  layerRef,
  onBadgeClick,
  onBadgeHover,
  rects,
}: ReviewCommentOverlayProps) {
  return (
    <div className={styles.reviewOverlay} aria-hidden="true">
      <div className={styles.reviewOverlayLayer} ref={layerRef}>
        {changedBlockRects.map((rect) => (
          <div
            className={styles.changedBlockRect}
            data-reason={rect.reason}
            key={`${rect.reviewBlock}-${rect.reason}`}
            style={
              {
                height: rect.height,
                left: rect.left,
                top: rect.top,
                width: rect.width,
              } as CSSProperties
            }
          >
            <span className={styles.changedBlockBadge}>
              {changedBlockLabel(rect.reason)}
            </span>
          </div>
        ))}
        {rects.map((rect, index) => {
          const isActive = rect.commentId === activeCommentId;
          const isHovered = rect.commentId === hoveredCommentId;
          const shouldShow = isActive || isHovered;
          if (!shouldShow) {
            return null;
          }

          return (
            <div
              className={styles.reviewOverlayRect}
              data-active={isActive || undefined}
              data-hovered={isHovered || undefined}
              data-kind={rect.kind}
              key={`${rect.commentId}-${rect.kind}-${index}`}
              style={
                {
                  "--review-highlight-bg": rect.color.background,
                  "--review-highlight-strong": rect.color.highlight,
                  "--review-highlight-ring": rect.color.ring,
                  "--review-highlight-stroke": rect.color.accent,
                  height: rect.height,
                  left: rect.left,
                  top: rect.top,
                  width: rect.width,
                } as CSSProperties
              }
            />
          );
        })}
        {badges.map((badge) => {
          const isActive = badge.commentIds.includes(activeCommentId ?? "");
          const isHovered = badge.commentIds.includes(hoveredCommentId ?? "");
          const label = badge.count > 2 ? "3+" : String(badge.count);

          return (
            <div
              className={styles.reviewOverlayBadge}
              data-active={isActive || undefined}
              data-hovered={isHovered || undefined}
              data-kind={badge.kind}
              key={`${badge.reviewBlock}-${badge.commentIds.join("-")}`}
              role="button"
              style={
                {
                  "--review-highlight-bg": badge.color.background,
                  "--review-highlight-ring": badge.color.ring,
                  "--review-highlight-stroke": badge.color.accent,
                  left: badge.left,
                  top: badge.top,
                } as CSSProperties
              }
              tabIndex={0}
              onClick={() => onBadgeClick?.(badge.commentIds)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  onBadgeClick?.(badge.commentIds);
                }
              }}
              onMouseEnter={() => onBadgeHover?.(badge.commentIds)}
              onMouseLeave={() => onBadgeHover?.(null)}
            >
              {label}
            </div>
          );
        })}
      </div>
    </div>
  );
}

export default ReviewCommentOverlay;

function changedBlockLabel(reason: ReviewChangedBlockRect["reason"]) {
  switch (reason) {
    case "approval":
      return "Edited after approval";
    case "comment":
      return "Edited after comment";
    case "comment_and_approval":
      return "Edited after comment and approval";
  }
}
