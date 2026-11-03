import type { EmailSends, SentText, SlotVersionSends } from "./api";

// Series colors: the first three slots of the categorical palette, which stay
// apart for color-blind readers in any pair. Texts folded together are gray.
const textColors = ["#2a78d6", "#eb6834", "#1baf7a"];
const earlierColor = "#8a8984";
export const earlierKey = "earlier";

export type SendsSeries = {
  key: string;
  label: string;
  color: string;
  // The texts behind the series: one, or the folded earlier ones.
  texts: SentText[];
};

export type SendsChart = {
  series: SendsSeries[];
  // One row per day of the period, oldest first: date plus sends per series.
  rows: Array<Record<string, string | number>>;
};

// buildSendsChart turns the API's counts into stacked bars. Texts keep the
// order the server gives (oldest first), so a text keeps its color while the
// period stays the same. More than three texts: the two newest keep their
// own, the rest fold into one gray series.
export function buildSendsChart(sends: EmailSends): SendsChart {
  const texts = sends.texts;
  const folded = texts.length > textColors.length ? texts.slice(0, texts.length - 2) : [];
  const own = texts.slice(folded.length);
  const series: SendsSeries[] = [];
  if (folded.length > 0) {
    series.push({ key: earlierKey, label: `${folded.length} earlier texts`, color: earlierColor, texts: folded });
  }
  own.forEach((text, index) => {
    series.push({ key: text.group_id, label: textLabel(text), color: textColors[index], texts: [text] });
  });
  const seriesOf = new Map<string, string>();
  for (const item of series) {
    for (const text of item.texts) {
      seriesOf.set(text.group_id, item.key);
    }
  }

  const byDate = new Map<string, Record<string, string | number>>();
  for (const date of periodDays(sends.since, sends.until)) {
    const row: Record<string, string | number> = { date };
    for (const item of series) {
      row[item.key] = 0;
    }
    byDate.set(date, row);
  }
  for (const day of sends.daily) {
    const row = byDate.get(day.date);
    const key = seriesOf.get(day.group_id);
    if (row && key) {
      row[key] = (row[key] as number) + day.sends;
    }
  }
  return { series, rows: [...byDate.values()] };
}

// periodDays lists the days from since to until (YYYY-MM-DD), both included.
export function periodDays(since: string, until: string): string[] {
  const days: string[] = [];
  const end = Date.parse(`${until}T00:00:00Z`);
  for (let day = Date.parse(`${since}T00:00:00Z`); day <= end; day += 24 * 60 * 60 * 1000) {
    days.push(new Date(day).toISOString().slice(0, 10));
  }
  return days;
}

// formatDay shows a YYYY-MM-DD day as "12 Oct" in the viewer's locale.
export function formatDay(date: string) {
  return new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short", timeZone: "UTC" }).format(
    new Date(`${date}T00:00:00Z`)
  );
}

// A text is named by the day the portal first sent it: that is when the
// upload carrying it reached the robot.
function textLabel(text: SentText) {
  return `Text since ${new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" }).format(
    new Date(text.first_sent_at)
  )}`;
}

// seriesTotals adds up a series' sends in the period and finds its last send.
export function seriesTotals(series: SendsSeries) {
  return {
    sends: series.texts.reduce((sum, text) => sum + text.sends, 0),
    lastSentAt: series.texts.reduce((last, text) => (text.last_sent_at > last ? text.last_sent_at : last), ""),
  };
}

// versionLabel names an email among the versions of its slot: by region on a
// board with regions, by language otherwise; old variants say so.
export function versionLabel(version: SlotVersionSends): string {
  const old = version.variant === "old" ? " · old" : "";
  if (version.region_name) {
    return `${version.region_name}${version.reference ? " (reference)" : ""}${old}`;
  }
  return `${version.language.toUpperCase()}${old || " · new"}`;
}

// The nightly sync leaves at most a day between runs; older than this, the
// latest sends are likely missing.
const staleSyncHours = 36;

// syncNote says how recent the counts are, so a quiet chart can be told apart
// from a sync that has not run.
export function syncNote(syncedAt: string | null, now: Date): { text: string; stale: boolean } {
  if (!syncedAt) {
    return { text: "The portal sync has not run for this board yet.", stale: true };
  }
  const at = new Date(syncedAt);
  const when = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(at);
  if (now.getTime() - at.getTime() > staleSyncHours * 3_600_000) {
    return { text: `Portal sync last ran ${when}: later sends are not counted yet.`, stale: true };
  }
  return { text: `Portal sync last ran ${when}.`, stale: false };
}
