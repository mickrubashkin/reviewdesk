import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";

import { fetchJson } from "./api";

export const userNamesQueryKey = ["users", "names"];

// useUserName returns a function that turns an address (main or alias) into
// the person's name, falling back to the address itself.
export function useUserName() {
  const namesQuery = useQuery({
    queryKey: userNamesQueryKey,
    queryFn: () => fetchJson<Record<string, string>>("/api/users/names"),
    staleTime: 5 * 60 * 1000,
  });
  const names = namesQuery.data;
  return useCallback(
    (email: string | null | undefined, fallback = "") => {
      if (!email) {
        return fallback;
      }
      return names?.[email.trim().toLowerCase()] ?? email;
    },
    [names]
  );
}
