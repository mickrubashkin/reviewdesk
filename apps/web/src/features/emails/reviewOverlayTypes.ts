import type { ReviewTextSelection } from "./MailPreview.types";
import type { EmailCommentSeverity } from "./types";

export type ReviewCommentTarget = ReviewTextSelection & {
  authorKey: string;
  id: string;
  severity: EmailCommentSeverity;
  staleAfterEdit?: boolean;
  status: "open" | "resolved";
};

export type ReviewChangedBlockReason =
  | "ai"
  | "approval"
  | "comment"
  | "comment_and_approval";

// A block outlined in the preview: edited since a comment or approval, or
// the subject of AI recommendations ("ai", labelled with their numbers).
export type ReviewChangedBlockTarget = {
  label?: string;
  reason: ReviewChangedBlockReason;
  reviewBlock: string;
};

export type ReviewCommentColor = {
  accent: string;
  background: string;
  highlight: string;
  ring: string;
};

export type ReviewOverlayRect = {
  color: ReviewCommentColor;
  commentId: string;
  height: number;
  kind: "block" | "text";
  left: number;
  reviewBlock: string;
  status: ReviewCommentTarget["status"];
  top: number;
  width: number;
};

export type ReviewOverlayBadge = {
  color: ReviewCommentColor;
  commentIds: string[];
  count: number;
  kind: "block" | "text";
  left: number;
  reviewBlock: string;
  status: ReviewCommentTarget["status"];
  top: number;
};

export type ReviewChangedBlockRect = {
  height: number;
  label?: string;
  left: number;
  reason: ReviewChangedBlockReason;
  reviewBlock: string;
  top: number;
  width: number;
};
