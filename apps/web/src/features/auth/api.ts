import { ApiError, fetchJson } from "../../shared/api";
import type { AuthUser } from "../emails/types";

export function requestLoginCode(email: string): Promise<{ ok: boolean }> {
  return fetchJson<{ ok: boolean }>("/api/auth/request-code", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ email }),
  });
}

export function verifyLoginCode(
  email: string,
  code: string
): Promise<{ ok: boolean }> {
  return fetchJson<{ ok: boolean }>("/api/auth/verify-code", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ email, code }),
  });
}

export function devLogin(email: string): Promise<{ ok: boolean }> {
  return fetchJson<{ ok: boolean }>("/api/auth/dev-login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ email }),
  });
}

export function demoLogin(): Promise<{ ok: boolean }> {
  return fetchJson<{ ok: boolean }>("/api/auth/demo-login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(demoVisitEntry()),
  });
}

const demoVisitorIdKey = "reviewdesk_demo_visitor_id";
const demoOwnerKeyKey = "reviewdesk_demo_owner_key";

// Where the visitor came from and what their browser tells, for the demo
// visit stats: a random id kept across visits, the time zone and language
// (a rough place, no IP), and the owner's key, which `?owner=<key>` keeps
// in this browser once.
function demoVisitEntry() {
  const referrer = document.referrer;
  const external = referrer !== "" && !referrer.startsWith(window.location.origin);
  const params = new URLSearchParams(window.location.search);
  const ownerParam = params.get("owner");
  if (ownerParam) {
    writeStored(demoOwnerKeyKey, ownerParam);
    // Kept out of links copied from the address bar.
    const url = new URL(window.location.href);
    url.searchParams.delete("owner");
    window.history.replaceState(window.history.state, "", url);
  }
  let visitorId = readStored(demoVisitorIdKey);
  if (!visitorId && typeof crypto.randomUUID === "function") {
    visitorId = crypto.randomUUID();
    writeStored(demoVisitorIdKey, visitorId);
  }
  return {
    referrer: external ? referrer : "",
    utm_source: params.get("utm_source") ?? "",
    visitor_id: visitorId,
    owner_key: ownerParam ?? readStored(demoOwnerKeyKey),
    time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone ?? "",
    language: navigator.language ?? "",
  };
}

// Storage can be blocked (private windows, site data off); the stats then
// go without it.
function readStored(key: string) {
  try {
    return window.localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

function writeStored(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Not kept; the next visit counts as a new one.
  }
}

export function fetchCurrentUser(): Promise<AuthUser> {
  return fetchJson<AuthUser>("/api/auth/me");
}

// The public demo build has no sign-in: a visitor opens the board as the
// shared demo reviewer, and the login page shows only if that fails.
export const demoMode = import.meta.env.VITE_AUTH_DEMO_LOGIN_ENABLED === "true";

// fetchCurrentUserOrEnterDemo is fetchCurrentUser that, in the demo, enters
// the demo when there is no session yet.
export async function fetchCurrentUserOrEnterDemo(): Promise<AuthUser> {
  try {
    return await fetchCurrentUser();
  } catch (error) {
    if (!demoMode || !(error instanceof ApiError) || error.status !== 401) {
      throw error;
    }
  }
  await demoLogin();
  return fetchCurrentUser();
}

// finishTour records that the tour of this version was finished or skipped.
export function finishTour(version: number): Promise<{ tour_version: number }> {
  return fetchJson<{ tour_version: number }>("/api/users/me/tour", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ version }),
  });
}

// markWhatsNewSeen records that "What's new" was opened up to this entry.
export function markWhatsNewSeen(seen: string): Promise<{ whats_new_seen: string }> {
  return fetchJson<{ whats_new_seen: string }>("/api/users/me/whats-new", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ seen }),
  });
}

export function logout(): Promise<{ ok: boolean }> {
  return fetchJson<{ ok: boolean }>("/api/auth/logout", {
    method: "POST",
  });
}
