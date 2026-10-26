import type { EmailReviewStatus, RobotUpload } from "./types";

export const emailReviewStatusOptions: Array<{
  value: EmailReviewStatus;
  label: string;
}> = [
  { value: "draft", label: "Draft" },
  { value: "ready", label: "Ready" },
];

export function formatEmailReviewStatus(status: string) {
  return (
    emailReviewStatusOptions.find((option) => option.value === status)?.label ??
    status.replaceAll("_", " ")
  );
}

export function emailReviewStatusColor(status: EmailReviewStatus) {
  return status === "ready" ? "teal" : "gray";
}

// isApprovedEmailReviewStatus reports whether the text is final and can go
// to Bitrix24.
export function isApprovedEmailReviewStatus(status: string) {
  return status === "ready";
}

type UploadFields = {
  review_status: EmailReviewStatus;
  latest_version?: number | null;
  uploaded_version?: number | null;
  upload_robots?: RobotUpload[];
};

export type UploadState = "not_ready" | "to_upload" | "uploaded" | "changed_since_upload";

// uploadRobots lists the email's Bitrix24 robots with their marks. An entry
// without the list (an older response) has one robot with the email's mark.
export function uploadRobots(email: UploadFields): RobotUpload[] {
  if (email.upload_robots?.length) {
    return email.upload_robots;
  }
  return [{ robot: "", version: email.uploaded_version ?? null, uploaded_at: null, uploaded_by_email: null }];
}

// robotsBehind are the robots without the current version.
export function robotsBehind(email: UploadFields): RobotUpload[] {
  const latest = email.latest_version ?? null;
  return uploadRobots(email).filter((robot) => robot.version === null || (latest !== null && robot.version < latest));
}

// robotName names a robot in a sentence; the one robot of a region without
// managers has no name.
export function robotName(robot: string) {
  return robot === "" ? "the robot" : `${robot}'s robot`;
}

// emailUploadState compares the versions pasted into the Bitrix24 robots
// with the current one: changed once any robot that has it lags behind.
export function emailUploadState(email: UploadFields): UploadState {
  const robots = uploadRobots(email);
  if (robots.every((robot) => robot.version === null)) {
    return isApprovedEmailReviewStatus(email.review_status) ? "to_upload" : "not_ready";
  }
  return robotsBehind(email).length > 0 ? "changed_since_upload" : "uploaded";
}

// needsBitrixUpload: the email is ready and a robot does not have this
// version. Drafts never need an upload, even when Bitrix24 has an older one.
export function needsBitrixUpload(email: UploadFields) {
  const state = emailUploadState(email);
  return email.review_status === "ready" && (state === "to_upload" || state === "changed_since_upload");
}

// uploadTodo says what to paste where, for a ready email some robot lacks:
// "to upload" when none of those robots has had it, else "re-upload". The
// robots are named when only some of the region's robots need it.
export function uploadTodo(email: UploadFields): { label: string; color: string; detail: string } | null {
  if (!needsBitrixUpload(email)) {
    return null;
  }
  const robots = uploadRobots(email);
  const behind = robotsBehind(email);
  const latest = email.latest_version ?? null;
  const fresh = behind.every((robot) => robot.version === null);
  const who = behind.length < robots.length ? ` (${behind.map((robot) => robot.robot).join(", ")})` : "";
  if (fresh) {
    return {
      label: `to upload${who}`,
      color: "blue",
      detail: `Not pasted into ${behind.map((robot) => robotName(robot.robot)).join(", ")} in Bitrix24 yet.`,
    };
  }
  const each = behind
    .map((robot) =>
      robot.version === null ? `${robotName(robot.robot)} does not have it yet` : `${robotName(robot.robot)} has version ${robot.version}`
    )
    .join("; ");
  return { label: `re-upload${who}`, color: "orange", detail: `Paste version ${latest ?? "–"} into Bitrix24: ${each}.` };
}

export const uploadStateOptions: Array<{ label: string; value: UploadState }> = [
  { label: "To upload (ready, not in Bitrix24)", value: "to_upload" },
  { label: "Changed since upload", value: "changed_since_upload" },
  { label: "Uploaded, current", value: "uploaded" },
];
