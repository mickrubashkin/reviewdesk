import type { ReviewTextSelection } from "../MailPreview.types";
import type { EditableField, EditableFields, EmailDetail } from "../types";

import type { InlineEditInput, InlineEditableTarget, SelectionMenuState } from "./MailPreview";

// Sent on the parent window when the input layer scrolls the frame: WebKit
// does not run the frame's own scroll listeners either.
export const FRAME_SCROLL_EVENT = "reviewframescroll";

const frameHeights = new WeakMap<HTMLIFrameElement, number>();

export function installReviewSelectionMenu(
  frame: HTMLIFrameElement,
  email: EmailDetail,
  setSelectionMenu: (selection: SelectionMenuState | null) => void,
  inputLayer: HTMLElement | null
) {
  const frameDocument = frame.contentDocument;
  const frameWindow = frame.contentWindow;
  if (!frameDocument || !frameWindow) {
    return;
  }

  // On a phone a long press selects text without a mouseup, so the menu
  // follows the selection once it settles, and opens below it to keep clear
  // of the system Copy/Look Up callout.
  const isTouch = frameWindow.matchMedia("(pointer: coarse)").matches;

  // A block click leaves a collapsed selection behind, and the selectionchange
  // that follows it must not close the composer the click just opened.
  let blockComposerOpen = false;
  // A tap on the menu, outside the frame, can drop the frame's selection; only
  // a touch in the frame closes the menu on a phone.
  let lastFrameTouch = 0;
  let settleTimer: number | undefined;
  const updateSelectionMenu = () => {
    blockComposerOpen = false;
    const selection = frameWindow.getSelection();
    const reviewSelection = getReviewSelection(selection);
    if (!reviewSelection) {
      setSelectionMenu(null);
      return;
    }

    const range = selection?.getRangeAt(0);
    const rangeRect = range?.getBoundingClientRect();
    if (!range || !rangeRect || (rangeRect.width === 0 && rangeRect.height === 0)) {
      setSelectionMenu(null);
      return;
    }

    const frameRect = frame.getBoundingClientRect();
    setSelectionMenu({
      ...reviewSelection,
      editableTarget: getEditableTargetFromNode(
        range.startContainer,
        email.editable_fields
      ),
      mode: "actions",
      placement: isTouch ? "bottom" : "top",
      x: frameRect.left + rangeRect.left + rangeRect.width / 2,
      y: frameRect.top + (isTouch ? rangeRect.bottom : rangeRect.top),
    });
  };
  const openBlockSelectionComposer = (target: EventTarget | null) => {
    const selection = frameWindow.getSelection();
    if (selection && !selection.isCollapsed) {
      return false;
    }

    if (!isElementLike(target)) {
      return false;
    }

    const block = getCommentableReviewBlockFromClick(target);
    if (!block) {
      return false;
    }

    const reviewSelection = getWholeBlockSelection(block);
    if (!reviewSelection) {
      return false;
    }

    const frameRect = frame.getBoundingClientRect();
    const blockRect = block.getBoundingClientRect();
    blockComposerOpen = true;
    setSelectionMenu({
      ...reviewSelection,
      editableTarget:
        getEditableTargetFromElement(target, email.editable_fields) ??
        getEditableTargetFromElement(block, email.editable_fields),
      mode: "actions",
      x: frameRect.left + blockRect.left + blockRect.width / 2,
      y: frameRect.top + blockRect.top,
    });
    return true;
  };

  // WebKit (every browser on iOS, Safari on a Mac) does not call the page's
  // listeners inside a frame sandboxed without allow-scripts, so there the
  // layer over the frame takes the input instead.
  if (inputLayer) {
    const useLayer = !frameRunsParentListeners(frameDocument);
    inputLayer.hidden = !useLayer;
    if (useLayer) {
      installInputLayer(inputLayer, frame, {
        isTouch,
        onBlockTap: (target) => {
          frameWindow.getSelection()?.removeAllRanges();
          if (!openBlockSelectionComposer(target)) {
            setSelectionMenu(null);
          }
        },
        onSelect: updateSelectionMenu,
        onScroll: () => setSelectionMenu(null),
      });
      return;
    }
  }

  frameDocument.addEventListener("mouseup", updateSelectionMenu);
  frameDocument.addEventListener("click", (event) => {
    openBlockSelectionComposer(event.target);
  });
  frameDocument.addEventListener("keyup", updateSelectionMenu);
  frameDocument.addEventListener(
    "touchstart",
    () => {
      lastFrameTouch = Date.now();
    },
    { passive: true }
  );
  frameDocument.addEventListener("selectionchange", () => {
    window.clearTimeout(settleTimer);
    const selection = frameWindow.getSelection();
    if (!selection || selection.isCollapsed) {
      const touchedFrame = Date.now() - lastFrameTouch < 1000;
      if (!blockComposerOpen && (!isTouch || touchedFrame)) {
        setSelectionMenu(null);
      }
      return;
    }
    if (isTouch) {
      settleTimer = window.setTimeout(updateSelectionMenu, 400);
    }
  });
  frameWindow.addEventListener("scroll", () => setSelectionMenu(null), true);
}

function frameRunsParentListeners(frameDocument: Document) {
  let ran = false;
  const probe = () => {
    ran = true;
  };
  frameDocument.addEventListener("reviewprobe", probe);
  frameDocument.dispatchEvent(new Event("reviewprobe"));
  frameDocument.removeEventListener("reviewprobe", probe);
  return ran;
}

function installInputLayer(
  layer: HTMLElement,
  frame: HTMLIFrameElement,
  handlers: {
    isTouch: boolean;
    onBlockTap: (target: Element | null) => void;
    onSelect: () => void;
    onScroll: () => void;
  }
) {
  const frameDocument = frame.contentDocument;
  const frameWindow = frame.contentWindow;
  if (!frameDocument || !frameWindow) {
    return;
  }

  // A finger cannot scroll the frame through the layer, so on a touch screen
  // the frame grows to the email and the page scrolls instead.
  if (handlers.isTouch) {
    window.clearInterval(frameHeights.get(frame));
    const fitHeight = () => {
      if (!frame.isConnected || frame.contentDocument !== frameDocument) {
        window.clearInterval(frameHeights.get(frame));
        return;
      }
      const height = frameDocument.documentElement.scrollHeight;
      if (height > 0 && frame.style.height !== `${height}px`) {
        frame.style.height = `${height}px`;
      }
    };
    fitHeight();
    frameHeights.set(frame, window.setInterval(fitHeight, 1000));
  }

  const pointInFrame = (event: PointerEvent) => {
    const rect = frame.getBoundingClientRect();
    return { x: event.clientX - rect.left, y: event.clientY - rect.top };
  };
  let press: { x: number; y: number; start: Range | null; dragging: boolean } | null = null;

  // Properties, not listeners: the layer outlives the frame document, and a
  // reload installs the handlers again.
  layer.onpointerdown = (event) => {
    if (event.pointerType === "mouse" && event.button !== 0) {
      return;
    }
    const point = pointInFrame(event);
    press = {
      x: event.clientX,
      y: event.clientY,
      start: caretRangeAt(frameDocument, point.x, point.y),
      dragging: false,
    };
    if (event.pointerType === "mouse") {
      event.preventDefault();
      layer.setPointerCapture(event.pointerId);
      frameWindow.getSelection()?.removeAllRanges();
    }
  };
  layer.onpointermove = (event) => {
    if (!press || event.pointerType !== "mouse") {
      return;
    }
    if (!press.dragging && Math.hypot(event.clientX - press.x, event.clientY - press.y) < 4) {
      return;
    }
    press.dragging = true;
    const point = pointInFrame(event);
    const end = caretRangeAt(frameDocument, point.x, point.y);
    if (press.start && end) {
      frameWindow
        .getSelection()
        ?.setBaseAndExtent(
          press.start.startContainer,
          press.start.startOffset,
          end.startContainer,
          end.startOffset
        );
    }
  };
  layer.onpointerup = (event) => {
    const current = press;
    press = null;
    if (!current) {
      return;
    }
    if (current.dragging) {
      handlers.onSelect();
      return;
    }
    if (Math.hypot(event.clientX - current.x, event.clientY - current.y) > 10) {
      return;
    }
    const point = pointInFrame(event);
    handlers.onBlockTap(frameDocument.elementFromPoint(point.x, point.y));
  };
  layer.onpointercancel = () => {
    press = null;
  };
  layer.onwheel = (event) => {
    event.preventDefault();
    frameWindow.scrollBy(event.deltaX, event.deltaY);
    handlers.onScroll();
    window.dispatchEvent(new Event(FRAME_SCROLL_EVENT));
  };
}

function caretRangeAt(frameDocument: Document, x: number, y: number): Range | null {
  if (typeof frameDocument.caretRangeFromPoint === "function") {
    return frameDocument.caretRangeFromPoint(x, y);
  }
  const position = frameDocument.caretPositionFromPoint?.(x, y);
  if (!position) {
    return null;
  }
  const range = frameDocument.createRange();
  range.setStart(position.offsetNode, position.offset);
  return range;
}

function getReviewSelection(
  selection: Selection | null
): ReviewTextSelection | null {
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
    return null;
  }

  const selectedText = selection.toString();
  if (!selectedText.trim()) {
    return null;
  }

  const range = selection.getRangeAt(0);
  const startBlock = getClosestReviewBlock(range.startContainer);
  const endBlock = getClosestReviewBlock(range.endContainer);
  if (!startBlock || startBlock !== endBlock) {
    return null;
  }

  const reviewBlock = startBlock.getAttribute("data-review-block");
  if (!reviewBlock) {
    return null;
  }

  const startOffset = getTextOffset(startBlock, range.startContainer, range.startOffset);
  const endOffset = startOffset + selectedText.length;

  return {
    reviewBlock,
    selectedText,
    startOffset,
    endOffset,
  };
}

function getWholeBlockSelection(block: Element): ReviewTextSelection | null {
  const reviewBlock = block.getAttribute("data-review-block");
  const selectedText = getReviewBlockSelectedText(block);
  if (!reviewBlock || !selectedText.trim()) {
    return null;
  }

  return {
    reviewBlock,
    selectedText,
    startOffset: 0,
    endOffset: selectedText.length,
  };
}

function getCommentableReviewBlockFromClick(target: Element) {
  const block = target.closest("[data-review-block]");
  if (!block || isNonCommentableReviewContainer(block)) {
    return null;
  }

  return block;
}

function isNonCommentableReviewContainer(block: Element) {
  const tagName = block.tagName.toLowerCase();
  if (tagName === "html" || tagName === "body") {
    return true;
  }

  if (block.querySelector("[data-review-block]")) {
    return true;
  }

  const text = getReviewBlockSelectedText(block);

  return !text;
}

function getReviewBlockSelectedText(block: Element) {
  const text = block.textContent?.trim();
  if (text) {
    return text;
  }

  const image = block.matches("img")
    ? block
    : block.querySelector("img");
  if (!image) {
    return "";
  }

  const imageLabel =
    image.getAttribute("alt")?.trim() ||
    image.getAttribute("aria-label")?.trim() ||
    image.getAttribute("title")?.trim();
  if (imageLabel) {
    return imageLabel;
  }

  const imageSource = image.getAttribute("src")?.trim();
  if (imageSource) {
    return `Image: ${imageSource.split("/").pop() ?? imageSource}`;
  }

  return `Image: ${block.getAttribute("data-review-block") ?? "review block"}`;
}

function getClosestReviewBlock(node: Node) {
  const element =
    node.nodeType === 1 ? (node as Element) : node.parentElement;
  return element?.closest("[data-review-block]") ?? null;
}

function getTextOffset(root: Element, targetNode: Node, targetOffset: number) {
  const range = root.ownerDocument.createRange();
  range.selectNodeContents(root);
  range.setEnd(targetNode, targetOffset);

  return range.toString().length;
}

function isElementLike(value: EventTarget | null): value is Element {
  return value !== null && "closest" in value;
}

export function getHeaderEditableTarget(
  reviewBlock: string,
  selectedText: string
): InlineEditableTarget | null {
  if (reviewBlock !== "subject" && reviewBlock !== "preheader") {
    return null;
  }

  return {
    label: formatInlineEditLabel(reviewBlock),
    inputs: [
      {
        key: reviewBlock,
        label: formatInlineEditLabel(reviewBlock),
        type: "text",
        value: selectedText,
      },
    ],
  };
}

function getEditableTargetFromNode(
  node: Node,
  editableFields: EditableFields
): InlineEditableTarget | null {
  const element = node.nodeType === 1 ? (node as Element) : node.parentElement;
  if (!element) {
    return null;
  }

  return getEditableTargetFromElement(element, editableFields);
}

function getEditableTargetFromElement(
  element: Element,
  editableFields: EditableFields
): InlineEditableTarget | null {
  const editableElement =
    element.closest(editableSelector) ??
    (element.matches("[data-review-block]")
      ? element.querySelector(editableSelector)
      : null);
  if (!editableElement) {
    return null;
  }

  const inputs: InlineEditInput[] = [];
  appendInlineEditInput(inputs, editableElement, editableFields, "data-edit-text");
  appendInlineEditInput(inputs, editableElement, editableFields, "data-edit-attr-href");
  appendInlineEditInput(inputs, editableElement, editableFields, "data-edit-attr-src");
  appendInlineEditInput(inputs, editableElement, editableFields, "data-edit-attr-alt");
  appendInlineEditInput(inputs, editableElement, editableFields, "data-edit-style-width-px");

  if (inputs.length === 0) {
    return null;
  }

  const reviewBlock =
    editableElement.closest("[data-review-block]")?.getAttribute("data-review-block") ??
    inputs[0].key;
  return {
    inputs,
    label: formatInlineEditLabel(reviewBlock),
  };
}

const editableSelector = [
  "[data-edit-text]",
  "[data-edit-attr-href]",
  "[data-edit-attr-src]",
  "[data-edit-attr-alt]",
  "[data-edit-style-width-px]",
].join(",");

// Mirrors emailedit.CanFormat: headings and lists need a block element.
const formattableTags = new Set(["td", "th", "div"]);

function appendInlineEditInput(
  inputs: InlineEditInput[],
  element: Element,
  editableFields: EditableFields,
  attribute: string
) {
  const key = element.getAttribute(attribute);
  if (!key || inputs.some((input) => input.key === key)) {
    return;
  }

  const field = editableFields[key];
  if (!field) {
    return;
  }

  inputs.push({
    field,
    formattable:
      attribute === "data-edit-text" &&
      field.type === "text" &&
      formattableTags.has(element.tagName.toLowerCase()),
    key,
    label: formatInlineEditLabel(getInlineEditFieldLabel(key)),
    type: inlineInputTypeForField(field),
    value: field.value,
  });
}

function inlineInputTypeForField(field: EditableField) {
  if (field.type === "url" || field.type === "image") {
    return field.type;
  }
  if (field.type === "number") {
    return "number";
  }
  return "text";
}

export function shouldUseInlineTextarea(input: InlineEditInput) {
  if (input.formattable) {
    return true;
  }
  const value = String(input.value ?? "");
  return value.includes("\n") || value.length > 100;
}

function getInlineEditFieldLabel(key: string) {
  for (const suffix of ["_width_px", "_text", "_url", "_src", "_alt"]) {
    if (key.endsWith(suffix)) {
      return suffix.slice(1);
    }
  }

  return key;
}

function formatInlineEditLabel(value: string) {
  return value
    .split(/[_-]/)
    .filter(Boolean)
    .map((word) => `${word.charAt(0).toUpperCase()}${word.slice(1)}`)
    .join(" ");
}

