import type { EmailReviewStatus } from "./types";

export const emailReviewStatusOptions: Array<{
  value: EmailReviewStatus;
  label: string;
}> = [
  { value: "draft", label: "Draft" },
  { value: "in_review", label: "In review" },
  { value: "changes_requested", label: "Changes requested" },
  { value: "approved", label: "Review approved" },
  { value: "production_approved", label: "Production approved" },
];

export function formatEmailReviewStatus(status: string) {
  return (
    emailReviewStatusOptions.find((option) => option.value === status)?.label ??
    status.replaceAll("_", " ")
  );
}

export function emailReviewStatusColor(status: EmailReviewStatus) {
  switch (status) {
    case "draft":
      return "gray";
    case "in_review":
      return "blue";
    case "changes_requested":
      return "yellow";
    case "approved":
      return "teal";
    case "production_approved":
      return "green";
    default:
      return "gray";
  }
}

export function isApprovedEmailReviewStatus(status: string) {
  return status === "approved" || status === "production_approved";
}

export function isProductionApprovedEmailReviewStatus(status: string) {
  return status === "production_approved";
}
