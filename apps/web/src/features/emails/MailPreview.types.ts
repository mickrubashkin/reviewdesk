import type {
  EditableFields,
  EmailCommentSeverity,
} from "./types";

export type PreviewViewport = "desktop" | "mobile";

export type ReviewTextSelection = {
  reviewBlock: string;
  selectedText: string;
  startOffset: number;
  endOffset: number;
};

export type InlineEditUpdate = {
  editableFields?: EditableFields;
  preheader?: string;
  subject?: string;
};

export type CreateReviewCommentHandler = (
  selection: ReviewTextSelection,
  body: string,
  severity: EmailCommentSeverity
) => void;
