import { fetchJson } from "../../shared/api";
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
  });
}

export function fetchCurrentUser(): Promise<AuthUser> {
  return fetchJson<AuthUser>("/api/auth/me");
}

export function logout(): Promise<{ ok: boolean }> {
  return fetchJson<{ ok: boolean }>("/api/auth/logout", {
    method: "POST",
  });
}
