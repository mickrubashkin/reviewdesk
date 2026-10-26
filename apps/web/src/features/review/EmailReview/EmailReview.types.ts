import type { AuthUser } from "../../emails/types";

export type EmailReviewProps = {
  // Role, languages the user translates and boards they manage decide what
  // they may change here.
  currentUser: AuthUser;
  emailId: string;
};

export type ReviewViewport = "desktop" | "mobile";

export type ReviewPanelTab = "comments" | "ai" | "details" | "history";
// Compact layout adds the email preview as its own tab.
export type ReviewContentTab = ReviewPanelTab | "email";
export type CommentStatusFilter = "open" | "all";
export type CommentBlockOption = {
  label: string;
  reviewBlock: string;
  selectedText: string;
  value: string;
};
