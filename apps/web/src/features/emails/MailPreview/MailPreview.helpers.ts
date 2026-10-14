import type { ReviewTextSelection } from "../MailPreview.types";
import type { EditableField, EditableFields, EmailDetail } from "../types";

import type { InlineEditInput, InlineEditableTarget, SelectionMenuState } from "./MailPreview";

export function installReviewSelectionMenu(
  frame: HTMLIFrameElement,
  email: EmailDetail,
  setSelectionMenu: (selection: SelectionMenuState | null) => void
) {
  const frameDocument = frame.contentDocument;
  const frameWindow = frame.contentWindow;
  if (!frameDocument || !frameWindow) {
    return;
  }

  const updateSelectionMenu = () => {
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
      x: frameRect.left + rangeRect.left + rangeRect.width / 2,
      y: frameRect.top + rangeRect.top,
    });
  };
  const openBlockSelectionComposer = (event: MouseEvent) => {
    const selection = frameWindow.getSelection();
    if (selection && !selection.isCollapsed) {
      return;
    }

    const target = event.target;
    if (!isElementLike(target)) {
      return;
    }

    const block = getCommentableReviewBlockFromClick(target);
    if (!block) {
      return;
    }

    const reviewSelection = getWholeBlockSelection(block);
    if (!reviewSelection) {
      return;
    }

    const frameRect = frame.getBoundingClientRect();
    const blockRect = block.getBoundingClientRect();
    setSelectionMenu({
      ...reviewSelection,
      editableTarget:
        getEditableTargetFromElement(target, email.editable_fields) ??
        getEditableTargetFromElement(block, email.editable_fields),
      mode: "actions",
      x: frameRect.left + blockRect.left + blockRect.width / 2,
      y: frameRect.top + blockRect.top,
    });
  };

  frameDocument.addEventListener("mouseup", updateSelectionMenu);
  frameDocument.addEventListener("click", openBlockSelectionComposer);
  frameDocument.addEventListener("keyup", updateSelectionMenu);
  frameDocument.addEventListener("selectionchange", () => {
    const selection = frameWindow.getSelection();
    if (!selection || selection.isCollapsed) {
      setSelectionMenu(null);
    }
  });
  frameWindow.addEventListener("scroll", () => setSelectionMenu(null), true);
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

