import { uploadTodo } from "./reviewStatus";
import type { FirstSendEstimate } from "./firstSend";
import type { EmailListItem } from "./types";

export type StatusPart = {
  label: string;
  // Mantine color name; dimmed parts use "dimmed".
  color: string;
  // One sentence for the tooltip.
  detail: string;
};

function liveOthersLabel(email: EmailListItem, versions: EmailListItem[]) {
  return versions
    .filter((v) => v.id !== email.id && (v.production_status === "live" || v.production_status === "live_manual"))
    .map((v) => `${v.language.toUpperCase()} ${v.variant}${v.adaptation_key !== "default" ? ` (${v.adaptation_label})` : ""}`)
    .join(", ");
}

// emailStatusParts sums up an email in one line, read left to right: what
// goes out now, what is being edited, what someone has to do. versions are the
// other emails of the same slot and language, to tell when another one is
// the version robots send.
//
//   Live v3                       robots send it; Bitrix24 has the current version
//   Live (manual)                 marked live, one version, no upload mark
//   Live v3 · v4 draft            v4 is being edited
//   Live v3 · v4 ready · re-upload
//   Live v4 · re-upload (Diego)   one of the region's robots lags behind
//   Not sent · v1 draft
const shortDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, { day: "numeric", month: "short" });

// firstSendPart tells a robot that has not sent yet from one that should
// have: before the date the wait is expected, after it someone checks.
export function firstSendPart(email: EmailListItem, estimate: FirstSendEstimate, now = new Date()): StatusPart {
  const from = shortDate(estimate.from);
  const delay =
    estimate.days === 0
      ? "right away"
      : `${estimate.days} ${estimate.days === 1 ? "day" : "days"} after (the delays of the stage's emails up to this one)`;
  const why = `Pasted into Bitrix24 on ${shortDate(email.uploaded_at ?? estimate.from)}. A new robot acts only on deals that enter the stage after that and sends this email ${delay}, so the portal can show the first send from ${from}, after the nightly sync.`;
  if (now < new Date(estimate.from)) {
    return { label: `Expected from ${from}`, color: "blue", detail: why };
  }
  return {
    label: `No send since ${from}`,
    color: "orange",
    detail: `${why} None so far: maybe no deal has reached the stage, the robot's condition did not hold, or the robot is not set up. Check it in Bitrix24.`,
  };
}

// sendsInLabel off leaves the sends out of the Live label, for a view that
// shows them on its own (the timeline).
export function emailStatusParts(
  email: EmailListItem,
  versions: EmailListItem[] = [],
  { sendsInLabel = true }: { sendsInLabel?: boolean } = {}
): StatusPart[] {
  const parts: StatusPart[] = [];
  const uploaded = email.uploaded_version ?? null;
  const latest = email.latest_version ?? null;
  const status = email.production_status;

  if (status === "live" || status === "live_manual") {
    const quiet = status === "live" ? (email.portal_quiet_days ?? null) : null;
    const sends =
      status === "live"
        ? `${quiet !== null ? `${quietText(quiet)} ` : ""}${portalSendsText(email)}`
        : "Marked as live by hand."
    parts.push({
      label: `Live${uploaded !== null ? ` v${uploaded}` : ""}${status === "live_manual" ? " (manual)" : sendsInLabel ? ` · ${liveSendsLabel(email)}` : ""}`,
      color: status === "live_manual" ? "teal" : quiet !== null ? "yellow" : "green",
      detail:
        uploaded !== null
          ? `${sends} Bitrix24 has version ${uploaded}.`
          : `${sends} Which version the robot has is not marked.`,
    });
  } else if (status === "not_seen") {
    const others = liveOthersLabel(email, versions);
    parts.push(
      others
        ? { label: "Other version live", color: "orange", detail: `This version is not sent; live instead: ${others}.` }
        : email.first_send
          ? firstSendPart(email, email.first_send)
          : { label: "Not sent", color: "dimmed", detail: "The portal sync has not seen CRM robots send it." }
    );
    if (uploaded !== null) {
      parts.push({ label: `in Bitrix v${uploaded}`, color: "dimmed", detail: `Bitrix24 has version ${uploaded}.` });
    }
  } else if (uploaded !== null) {
    parts.push({ label: `In Bitrix v${uploaded}`, color: "dimmed", detail: `Bitrix24 has version ${uploaded}.` });
  }

  // A live email that has only ever had one version is that version, even
  // without an upload mark: "Live · v1 draft" would read as a contradiction.
  const onlyVersionLive = (status === "live" || status === "live_manual") && uploaded === null && (latest ?? 1) <= 1;

  // The version being worked on, unless it is the one Bitrix24 already has:
  // then its draft/ready status says nothing new.
  if (!onlyVersionLive && (uploaded === null || latest === null || latest > uploaded)) {
    const ready = email.review_status === "ready";
    parts.push({
      label: `${latest !== null ? `v${latest} ` : ""}${ready ? "ready" : "draft"}`,
      color: ready ? "teal" : "gray",
      detail: ready
        ? `Version ${latest ?? "–"} is ready to go to Bitrix24.`
        : `Version ${latest ?? "–"} is a draft, still being edited.`,
    });
  }

  const todo = onlyVersionLive ? null : uploadTodo(email);
  if (todo) {
    parts.push(todo);
  }
  if (email.translation_stale) {
    parts.push({ label: "EN changed", color: "red", detail: "The EN text changed after this translation; open it to see what changed." });
  }
  if (email.changed_since_my_visit) {
    parts.push({
      label: "changed",
      color: "blue",
      detail: "Someone else edited its text since you last opened it; open it to see what changed.",
    });
  }
  if (email.based_on_stale) {
    parts.push({
      label: "reference changed",
      color: "orange",
      detail: "The reference email changed after this one last matched it; open it to see what to take.",
    });
  }
  return parts;
}

type SendFields = Pick<EmailListItem, "portal_send_count" | "portal_sends_30d" | "portal_last_sent_at" | "portal_quiet_days">;

// liveSendsLabel is the short form on a live email: its sends in the last 30
// days, or how long it has been quiet.
export function liveSendsLabel(email: SendFields) {
  const quiet = email.portal_quiet_days ?? null;
  return quiet !== null ? `quiet ${quiet}d` : `${email.portal_sends_30d ?? 0}/30d`;
}

// lastSendLine is the timeline's line under a live email: when it was last
// sent, or how long it has been quiet.
export function lastSendLine(email: SendFields, now = new Date()) {
  const quiet = email.portal_quiet_days ?? null;
  if (quiet !== null) return `no sends for ${quiet} days`;
  return email.portal_last_sent_at ? `last sent ${timeAgo(email.portal_last_sent_at, now)}` : "";
}

function quietText(days: number) {
  return `No sends for ${days} days before the last portal sync, far longer than usual: is the robot still on?`;
}

// portalSendsText sums up what the portal sync saw sent: the last send, the
// last 30 days and everything it keeps.
export function portalSendsText(email: SendFields, now = new Date()) {
  const last = email.portal_last_sent_at
    ? `Last sent ${timeAgo(email.portal_last_sent_at, now)} (${new Date(email.portal_last_sent_at).toLocaleString(undefined, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}). `
    : "";
  return `${last}Sent ${email.portal_sends_30d ?? 0} times in the last 30 days, ${email.portal_send_count} in the last ${sendRetentionDays} days.`;
}

// timeAgo says how long ago, in the largest whole unit.
export function timeAgo(value: string, now = new Date()) {
  const minutes = Math.max(0, Math.round((now.getTime() - Date.parse(value)) / 60000));
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.round(hours / 24);
  return `${days} ${days === 1 ? "day" : "days"} ago`;
}

// sendRetentionDays mirrors portal.SendRetentionDays on the server.
const sendRetentionDays = 180;
