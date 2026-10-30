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

// Where the visitor came from, for the demo visit stats.
function demoVisitEntry() {
  const referrer = document.referrer;
  const external = referrer !== "" && !referrer.startsWith(window.location.origin);
  return {
    referrer: external ? referrer : "",
    utm_source: new URLSearchParams(window.location.search).get("utm_source") ?? "",
  };
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
