import { timingDays } from "./firstSend";

// normalizeSendTiming writes a send timing one way, so variants people typed
// ("immediately", "+ 3 day") read as the same choice.
export function normalizeSendTiming(timing: string): string {
  const text = timing.trim().replace(/\s+/g, " ");
  if (text.toLowerCase() === "immediately") return "Immediately";
  const plus = text.match(/^\+ ?(\d+) days?$/i);
  if (plus) return `+${plus[1]} ${days(plus[1])}`;
  const afterPrevious = text.match(/^(\d+) days? after previous email$/i);
  if (afterPrevious) return `${afterPrevious[1]} ${days(afterPrevious[1])} after previous email`;
  return text;
}

function days(count: string) {
  return count === "1" ? "day" : "days";
}

// sendTimingOptions lists the timings in use once each, shortest delay
// first ("Immediately" is 0 days); timings without days come last.
export function sendTimingOptions(timings: (string | null | undefined)[]): string[] {
  const byKey = new Map<string, string>();
  for (const timing of timings) {
    if (!timing?.trim()) continue;
    const option = normalizeSendTiming(timing);
    if (!byKey.has(option.toLowerCase())) byKey.set(option.toLowerCase(), option);
  }
  return [...byKey.values()].sort((a, b) => {
    const dayA = timingDays(a) ?? Infinity;
    const dayB = timingDays(b) ?? Infinity;
    if (dayA !== dayB) return dayA - dayB;
    return a.localeCompare(b);
  });
}
