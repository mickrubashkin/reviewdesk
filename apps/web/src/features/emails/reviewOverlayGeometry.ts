import type { ReviewTextSelection } from "./MailPreview.types";
import type {
  ReviewOverlayBadge,
  ReviewChangedBlockRect,
  ReviewChangedBlockTarget,
  ReviewCommentColor,
  ReviewCommentTarget,
  ReviewOverlayRect,
} from "./reviewOverlayTypes";

const reviewCommentColors: ReviewCommentColor[] = [
  {
    accent: "rgb(250, 176, 5)",
    background: "rgba(255, 212, 59, 0.14)",
    highlight: "rgba(255, 212, 59, 0.48)",
    ring: "rgba(250, 176, 5, 0.32)",
  },
  {
    accent: "rgb(55, 178, 77)",
    background: "rgba(81, 207, 102, 0.13)",
    highlight: "rgba(81, 207, 102, 0.4)",
    ring: "rgba(55, 178, 77, 0.28)",
  },
  {
    accent: "rgb(34, 139, 230)",
    background: "rgba(116, 192, 252, 0.14)",
    highlight: "rgba(116, 192, 252, 0.42)",
    ring: "rgba(34, 139, 230, 0.3)",
  },
  {
    accent: "rgb(121, 80, 242)",
    background: "rgba(177, 151, 252, 0.14)",
    highlight: "rgba(177, 151, 252, 0.42)",
    ring: "rgba(121, 80, 242, 0.3)",
  },
  {
    accent: "rgb(245, 124, 0)",
    background: "rgba(255, 169, 77, 0.14)",
    highlight: "rgba(255, 169, 77, 0.42)",
    ring: "rgba(245, 124, 0, 0.3)",
  },
  {
    accent: "rgb(12, 166, 120)",
    background: "rgba(99, 230, 190, 0.13)",
    highlight: "rgba(99, 230, 190, 0.38)",
    ring: "rgba(12, 166, 120, 0.26)",
  },
];

export function buildReviewOverlayFrameRects(
  frame: HTMLIFrameElement,
  targets: ReviewCommentTarget[]
): ReviewOverlayRect[] {
  const frameDocument = frame.contentDocument;
  const frameWindow = frame.contentWindow;
  if (!frameDocument || !frameWindow) {
    return [];
  }

  return targets.flatMap((target) => {
    const block = findReviewBlock(frameDocument, target.reviewBlock);
    if (!block) {
      return [];
    }

    const color = getCommentColor(target.authorKey);
    const range = getTargetTextRange(block, target);
    const rects = range
      ? Array.from(range.getClientRects()).filter(
          (rect) => rect.width > 0 && rect.height > 0
        )
      : [];

    if (rects.length > 0 && !isWholeBlockTarget(block, target)) {
      return rects.map((rect) =>
        createOverlayRectFromFrameDocumentRect(
          rect,
          frameWindow,
          target,
          color,
          "text"
        )
      );
    }

    return [
      createOverlayRectFromFrameDocumentRect(
        block.getBoundingClientRect(),
        frameWindow,
        target,
        color,
        "block"
      ),
    ];
  });
}

export function buildChangedBlockFrameRects(
  frame: HTMLIFrameElement,
  targets: ReviewChangedBlockTarget[]
): ReviewChangedBlockRect[] {
  const frameDocument = frame.contentDocument;
  const frameWindow = frame.contentWindow;
  if (!frameDocument || !frameWindow) {
    return [];
  }

  return targets.flatMap((target) => {
    const block = findReviewBlock(frameDocument, target.reviewBlock);
    if (!block) {
      return [];
    }

    return [
      createChangedBlockRectFromFrameDocumentRect(
        block.getBoundingClientRect(),
        frameWindow,
        target
      ),
    ];
  });
}

export function buildExternalReviewOverlayRects(
  overlayRoot: HTMLElement,
  targets: ReviewCommentTarget[]
): ReviewOverlayRect[] {
  const overlayRootRect = overlayRoot.getBoundingClientRect();

  return targets.flatMap((target) => {
    const block = findExternalReviewBlock(overlayRoot, target.reviewBlock);
    if (!block) {
      return [];
    }

    return [
      createOverlayRectFromRootRect(
        overlayRootRect,
        block.getBoundingClientRect(),
        target,
        getCommentColor(target.authorKey),
        "block"
      ),
    ];
  });
}

export function buildExternalChangedBlockRects(
  overlayRoot: HTMLElement,
  targets: ReviewChangedBlockTarget[]
): ReviewChangedBlockRect[] {
  const overlayRootRect = overlayRoot.getBoundingClientRect();

  return targets.flatMap((target) => {
    const block = findExternalReviewBlock(overlayRoot, target.reviewBlock);
    if (!block) {
      return [];
    }

    return [
      createChangedBlockRectFromRootRect(
        overlayRootRect,
        block.getBoundingClientRect(),
        target
      ),
    ];
  });
}

export function scrollReviewTargetIntoView(
  frame: HTMLIFrameElement,
  target: ReviewCommentTarget
) {
  const frameDocument = frame.contentDocument;
  if (!frameDocument) {
    return false;
  }

  const block = findReviewBlock(frameDocument, target.reviewBlock);
  if (!block) {
    return false;
  }

  const range = getTargetTextRange(block, target);
  if (range && !isWholeBlockTarget(block, target)) {
    const rangeRect = range.getBoundingClientRect();
    frame.contentWindow?.scrollBy({
      behavior: "smooth",
      top: rangeRect.top - frame.clientHeight / 2 + rangeRect.height / 2,
    });
    return true;
  }

  block.scrollIntoView({
    behavior: "smooth",
    block: "center",
    inline: "nearest",
  });
  return true;
}

export function scrollExternalReviewTargetIntoView(
  overlayRoot: HTMLElement,
  target: ReviewCommentTarget
) {
  const block = findExternalReviewBlock(overlayRoot, target.reviewBlock);
  if (!block) {
    return false;
  }

  block.scrollIntoView({
    behavior: "smooth",
    block: "center",
    inline: "nearest",
  });
  return true;
}

function createChangedBlockRectFromFrameDocumentRect(
  rect: DOMRect,
  frameWindow: Window,
  target: ReviewChangedBlockTarget
): ReviewChangedBlockRect {
  return {
    height: rect.height,
    left: rect.left + frameWindow.scrollX,
    reason: target.reason,
    reviewBlock: target.reviewBlock,
    top: rect.top + frameWindow.scrollY,
    width: rect.width,
  };
}

function createChangedBlockRectFromRootRect(
  overlayRootRect: DOMRect,
  rect: Pick<DOMRect, "height" | "left" | "top" | "width">,
  target: ReviewChangedBlockTarget
): ReviewChangedBlockRect {
  return {
    height: rect.height,
    left: rect.left - overlayRootRect.left,
    reason: target.reason,
    reviewBlock: target.reviewBlock,
    top: rect.top - overlayRootRect.top,
    width: rect.width,
  };
}

function createOverlayRectFromFrameDocumentRect(
  rect: DOMRect,
  frameWindow: Window,
  target: ReviewCommentTarget,
  color: ReviewCommentColor,
  kind: ReviewOverlayRect["kind"]
): ReviewOverlayRect {
  return {
    color,
    commentId: target.id,
    height: rect.height,
    kind,
    left: rect.left + frameWindow.scrollX,
    reviewBlock: target.reviewBlock,
    status: target.status,
    top: rect.top + frameWindow.scrollY,
    width: rect.width,
  };
}

function createOverlayRectFromRootRect(
  overlayRootRect: DOMRect,
  rect: Pick<DOMRect, "height" | "left" | "top" | "width">,
  target: ReviewCommentTarget,
  color: ReviewCommentColor,
  kind: ReviewOverlayRect["kind"]
): ReviewOverlayRect {
  return {
    color,
    commentId: target.id,
    height: rect.height,
    kind,
    left: rect.left - overlayRootRect.left,
    reviewBlock: target.reviewBlock,
    status: target.status,
    top: rect.top - overlayRootRect.top,
    width: rect.width,
  };
}

export function buildReviewOverlayBadges(
  rects: ReviewOverlayRect[]
): ReviewOverlayBadge[] {
  const openRects = rects.filter((rect) => rect.status === "open");
  const rectsByBlock = new Map<string, ReviewOverlayRect[]>();

  openRects.forEach((rect) => {
    rectsByBlock.set(rect.reviewBlock, [
      ...(rectsByBlock.get(rect.reviewBlock) ?? []),
      rect,
    ]);
  });

  return Array.from(rectsByBlock.entries()).flatMap(
    ([reviewBlock, blockRects]) => {
      const firstRectsByComment = new Map<string, ReviewOverlayRect>();

      blockRects.forEach((rect) => {
        if (!firstRectsByComment.has(rect.commentId)) {
          firstRectsByComment.set(rect.commentId, rect);
        }
      });

      const commentRects = Array.from(firstRectsByComment.values());
      const anchorRect =
        commentRects.find((rect) => rect.kind === "block") ?? commentRects[0];
      if (!anchorRect) {
        return [];
      }

      const isBlockBadge = commentRects.some((rect) => rect.kind === "block");
      return [
        {
          color: anchorRect.color,
          commentIds: commentRects.map((rect) => rect.commentId),
          count: commentRects.length,
          kind: isBlockBadge ? "block" : "text",
          left: anchorRect.left + anchorRect.width,
          reviewBlock,
          status: "open",
          top: anchorRect.top,
        },
      ];
    }
  );
}

function getTargetTextRange(block: Element, target: ReviewTextSelection) {
  if (isWholeBlockTarget(block, target)) {
    return null;
  }

  const rangeFromOffsets = createRangeFromTextOffsets(
    block,
    target.startOffset,
    target.endOffset
  );
  if (rangeFromOffsets) {
    return rangeFromOffsets;
  }

  const blockText = block.textContent ?? "";
  const fallbackStart = target.selectedText
    ? blockText.indexOf(target.selectedText)
    : -1;
  if (fallbackStart < 0) {
    return null;
  }

  return createRangeFromTextOffsets(
    block,
    fallbackStart,
    fallbackStart + target.selectedText.length
  );
}

function createRangeFromTextOffsets(
  root: Element,
  startOffset: number,
  endOffset: number
) {
  if (startOffset < 0 || endOffset <= startOffset) {
    return null;
  }

  const rootDocument = root.ownerDocument;
  const walker = rootDocument.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let currentOffset = 0;
  let startPosition: TextPosition | null = null;
  let endPosition: TextPosition | null = null;

  while (walker.nextNode()) {
    const node = walker.currentNode;
    const textLength = node.textContent?.length ?? 0;
    const nextOffset = currentOffset + textLength;

    if (!startPosition && startOffset <= nextOffset) {
      startPosition = {
        node,
        offset: Math.max(0, startOffset - currentOffset),
      };
    }

    if (!endPosition && endOffset <= nextOffset) {
      endPosition = {
        node,
        offset: Math.max(0, endOffset - currentOffset),
      };
      break;
    }

    currentOffset = nextOffset;
  }

  if (!startPosition || !endPosition) {
    return null;
  }

  const range = rootDocument.createRange();
  range.setStart(startPosition.node, startPosition.offset);
  range.setEnd(endPosition.node, endPosition.offset);
  return range;
}

type TextPosition = {
  node: Node;
  offset: number;
};

function isWholeBlockTarget(block: Element, target: ReviewTextSelection) {
  const blockText = block.textContent ?? "";
  return target.startOffset === 0 && target.endOffset >= blockText.length;
}

function findReviewBlock(frameDocument: Document, reviewBlock: string) {
  return (
    Array.from(frameDocument.querySelectorAll("[data-review-block]")).find(
      (element) => element.getAttribute("data-review-block") === reviewBlock
    ) ?? null
  );
}

function findExternalReviewBlock(root: HTMLElement, reviewBlock: string) {
  return (
    Array.from(root.querySelectorAll("[data-review-block]")).find(
      (element) => element.getAttribute("data-review-block") === reviewBlock
    ) ?? null
  );
}

function getCommentColor(authorKey: string) {
  const hash = Array.from(authorKey).reduce(
    (currentHash, char) => currentHash + char.charCodeAt(0),
    0
  );
  return reviewCommentColors[hash % reviewCommentColors.length];
}
