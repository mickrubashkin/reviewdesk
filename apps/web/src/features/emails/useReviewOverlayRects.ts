import { useCallback, useEffect, useState } from "react";

import {
  buildChangedBlockFrameRects,
  buildExternalChangedBlockRects,
  buildReviewOverlayBadges,
  buildExternalReviewOverlayRects,
  buildReviewOverlayFrameRects,
  scrollExternalReviewTargetIntoView,
  scrollReviewTargetIntoView,
} from "./reviewOverlayGeometry";
import type {
  ReviewChangedBlockRect,
  ReviewChangedBlockTarget,
  ReviewOverlayBadge,
  ReviewCommentTarget,
  ReviewOverlayRect,
} from "./reviewOverlayTypes";

type ReviewOverlayState = {
  externalChangedBlockRects: ReviewChangedBlockRect[];
  externalBadges: ReviewOverlayBadge[];
  externalRects: ReviewOverlayRect[];
  frameChangedBlockRects: ReviewChangedBlockRect[];
  frameBadges: ReviewOverlayBadge[];
  frameRects: ReviewOverlayRect[];
};

export function useReviewOverlayRects({
  activeCommentId,
  changedBlockTargets,
  commentTargets,
  frameOverlayLayerRef,
  frameLoadVersion,
  frameRef,
  overlayRootRef,
  viewport,
}: {
  activeCommentId: string | null;
  changedBlockTargets: ReviewChangedBlockTarget[];
  commentTargets: ReviewCommentTarget[];
  frameOverlayLayerRef: React.RefObject<HTMLElement | null>;
  frameLoadVersion: number;
  frameRef: React.RefObject<HTMLIFrameElement | null>;
  overlayRootRef: React.RefObject<HTMLElement | null>;
  viewport: string;
}) {
  const [overlayState, setOverlayState] = useState<ReviewOverlayState>({
    externalChangedBlockRects: [],
    externalBadges: [],
    externalRects: [],
    frameChangedBlockRects: [],
    frameBadges: [],
    frameRects: [],
  });
  const syncFrameOverlayScroll = useCallback(() => {
    const frameWindow = frameRef.current?.contentWindow;
    const frameOverlayLayer = frameOverlayLayerRef.current;
    if (!frameWindow || !frameOverlayLayer) {
      return;
    }

    frameOverlayLayer.style.transform = `translate3d(${-frameWindow.scrollX}px, ${-frameWindow.scrollY}px, 0)`;
  }, [frameOverlayLayerRef, frameRef]);
  const updateOverlayRects = useCallback(() => {
    const frame = frameRef.current;
    const overlayRoot = overlayRootRef.current;
    if (!frame || !overlayRoot) {
      setOverlayState({
        externalChangedBlockRects: [],
        externalBadges: [],
        externalRects: [],
        frameChangedBlockRects: [],
        frameBadges: [],
        frameRects: [],
      });
      return;
    }

    const frameRects = buildReviewOverlayFrameRects(frame, commentTargets);
    const frameChangedBlockRects = buildChangedBlockFrameRects(
      frame,
      changedBlockTargets
    );
    const externalRects = buildExternalReviewOverlayRects(
      overlayRoot,
      commentTargets
    );
    const externalChangedBlockRects = buildExternalChangedBlockRects(
      overlayRoot,
      changedBlockTargets
    );
    syncFrameOverlayScroll();
    setOverlayState({
      externalChangedBlockRects,
      externalBadges: buildReviewOverlayBadges(externalRects),
      externalRects,
      frameChangedBlockRects,
      frameBadges: buildReviewOverlayBadges(frameRects),
      frameRects,
    });
  }, [
    changedBlockTargets,
    commentTargets,
    frameRef,
    overlayRootRef,
    syncFrameOverlayScroll,
  ]);

  useEffect(() => {
    const frame = frameRef.current;
    const frameWindow = frame?.contentWindow;
    const frameDocument = frame?.contentDocument;
    const overlayRoot = overlayRootRef.current;
    if (!frame || !frameWindow || !overlayRoot) {
      setOverlayState({
        externalChangedBlockRects: [],
        externalBadges: [],
        externalRects: [],
        frameChangedBlockRects: [],
        frameBadges: [],
        frameRects: [],
      });
      return;
    }

    let animationFrame = 0;
    const scheduleOverlayUpdate = () => {
      window.cancelAnimationFrame(animationFrame);
      animationFrame = window.requestAnimationFrame(updateOverlayRects);
    };
    const handleFrameScroll = () => {
      syncFrameOverlayScroll();
    };
    const resizeObserver = new ResizeObserver(scheduleOverlayUpdate);

    scheduleOverlayUpdate();
    frameWindow.addEventListener("scroll", handleFrameScroll, true);
    window.addEventListener("resize", scheduleOverlayUpdate);
    resizeObserver.observe(frame);
    resizeObserver.observe(overlayRoot);
    if (frameDocument?.documentElement) {
      resizeObserver.observe(frameDocument.documentElement);
    }
    if (frameDocument?.body) {
      resizeObserver.observe(frameDocument.body);
    }

    return () => {
      window.cancelAnimationFrame(animationFrame);
      frameWindow.removeEventListener("scroll", handleFrameScroll, true);
      window.removeEventListener("resize", scheduleOverlayUpdate);
      resizeObserver.disconnect();
    };
  }, [
    frameLoadVersion,
    frameRef,
    overlayRootRef,
    syncFrameOverlayScroll,
    updateOverlayRects,
    viewport,
  ]);

  useEffect(() => {
    if (!activeCommentId) {
      return;
    }

    const frame = frameRef.current;
    const overlayRoot = overlayRootRef.current;
    const target = commentTargets.find((comment) => comment.id === activeCommentId);
    if (!frame || !overlayRoot || !target) {
      return;
    }

    const didScrollFrame = scrollReviewTargetIntoView(frame, target);
    if (!didScrollFrame) {
      scrollExternalReviewTargetIntoView(overlayRoot, target);
    }
    const timeout = window.setTimeout(updateOverlayRects, 120);
    return () => window.clearTimeout(timeout);
  }, [activeCommentId, commentTargets, frameRef, overlayRootRef, updateOverlayRects]);

  return overlayState;
}
