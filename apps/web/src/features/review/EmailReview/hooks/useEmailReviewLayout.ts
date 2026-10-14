import { type PointerEvent, useRef, useState } from "react";
import type {
  CommentStatusFilter,
  ReviewContentTab,
  ReviewPanelTab,
  ReviewUtilityPanel,
  ReviewViewport,
} from "../EmailReview.types";

const minRightPanelPercent = 24;
const maxRightPanelPercent = 48;

export function useEmailReviewLayout() {
  const [viewport, setViewport] = useState<ReviewViewport>("desktop");
  const [actionMenuOpened, setActionMenuOpened] = useState(false);
  const [activeContentTab, setActiveContentTab] =
    useState<ReviewContentTab>("email");
  const [activeUtilityPanel, setActiveUtilityPanel] =
    useState<ReviewUtilityPanel | null>(null);
  const [archiveModalOpened, setArchiveModalOpened] = useState(false);
  const [duplicateModalOpened, setDuplicateModalOpened] = useState(false);
  const [sourceHTMLModalOpened, setSourceHTMLModalOpened] = useState(false);
  const [sourceHTMLDraft, setSourceHTMLDraft] = useState("");
  const [isInlineEditPreviewRefreshing, setIsInlineEditPreviewRefreshing] =
    useState(false);
  const [rightPanelPercent, setRightPanelPercent] = useState(30);
  const [activePanelTab, setActivePanelTab] =
    useState<ReviewPanelTab>("comments");
  const [activeCommentId, setActiveCommentId] = useState<string | null>(null);
  const [commentStatusFilter, setCommentStatusFilter] =
    useState<CommentStatusFilter>("open");
  const [hoveredCommentId, setHoveredCommentId] = useState<string | null>(null);

  const isResizingRef = useRef(false);
  const [isResizing, setIsResizing] = useState(false);
  const contentRef = useRef<HTMLElement | null>(null);

  function clampPercent(value: number) {
    return Math.min(
      maxRightPanelPercent,
      Math.max(minRightPanelPercent, value)
    );
  }

  function handleResizeStart(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();
    isResizingRef.current = true;
    setIsResizing(true);

    const handlePointerMove = (moveEvent: globalThis.PointerEvent) => {
      if (!isResizingRef.current || !contentRef.current) {
        return;
      }
      const bounds = contentRef.current.getBoundingClientRect();
      if (bounds.width <= 0) {
        return;
      }
      const pointerOffset = bounds.right - moveEvent.clientX;
      const calculatedPercent = (pointerOffset / bounds.width) * 100;
      setRightPanelPercent(clampPercent(calculatedPercent));
    };

    const handlePointerUp = () => {
      isResizingRef.current = false;
      setIsResizing(false);
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
    };

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp);
  }

  return {
    viewport,
    setViewport,
    actionMenuOpened,
    setActionMenuOpened,
    activeContentTab,
    setActiveContentTab,
    activeUtilityPanel,
    setActiveUtilityPanel,
    archiveModalOpened,
    setArchiveModalOpened,
    duplicateModalOpened,
    setDuplicateModalOpened,
    sourceHTMLModalOpened,
    setSourceHTMLModalOpened,
    sourceHTMLDraft,
    setSourceHTMLDraft,
    isInlineEditPreviewRefreshing,
    setIsInlineEditPreviewRefreshing,
    rightPanelPercent,
    setRightPanelPercent,
    activePanelTab,
    setActivePanelTab,
    activeCommentId,
    setActiveCommentId,
    commentStatusFilter,
    setCommentStatusFilter,
    hoveredCommentId,
    setHoveredCommentId,
    isResizing,
    contentRef,
    handleResizeStart,
  };
}
