import { ApiError } from "../../../shared/api";

// gatewayAccessState reads why a gateway sign-in has no user yet: the server
// is still linking a first visit (401 "access pending"), refused it (403), or
// got no gateway identity at all (the app was opened outside Bitrix24).
export function gatewayAccessState(error: unknown): "pending" | "refused" | "outside" {
  if (error instanceof ApiError && error.status === 403) {
    return "refused";
  }
  if (error instanceof ApiError && error.status === 401 && error.message.trim() === "access pending") {
    return "pending";
  }
  return "outside";
}
