import { useCallback, useState, type Dispatch, type SetStateAction } from "react";

export type SortDirection = "asc" | "desc";
export type PersistedSort<SortKey extends string> = {
  direction: SortDirection;
  key: SortKey;
};

export function usePersistedSort<SortKey extends string>(
  storageKey: string,
  defaultSort: PersistedSort<SortKey>,
  allowedKeys: readonly SortKey[]
) {
  const [sort, setSortState] = useState<PersistedSort<SortKey>>(() =>
    readStoredSort(storageKey, defaultSort, allowedKeys)
  );

  const setSort: Dispatch<SetStateAction<PersistedSort<SortKey>>> = useCallback(
    (value) => {
      setSortState((current) => {
        const next =
          typeof value === "function"
            ? value(current)
            : value;
        const normalized = normalizeSort(next, defaultSort, allowedKeys);
        if (
          normalized.direction === current.direction &&
          normalized.key === current.key
        ) {
          return current;
        }
        writeStoredSort(storageKey, normalized);
        return normalized;
      });
    },
    [allowedKeys, defaultSort, storageKey]
  );

  return [sort, setSort] as const;
}

function readStoredSort<SortKey extends string>(
  storageKey: string,
  defaultSort: PersistedSort<SortKey>,
  allowedKeys: readonly SortKey[]
): PersistedSort<SortKey> {
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (!raw) {
      return defaultSort;
    }
    return normalizeSort(JSON.parse(raw), defaultSort, allowedKeys);
  } catch {
    return defaultSort;
  }
}

function writeStoredSort<SortKey extends string>(
  storageKey: string,
  sort: PersistedSort<SortKey>
) {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(sort));
  } catch {
    // Sorting should still work if localStorage is blocked or full.
  }
}

function normalizeSort<SortKey extends string>(
  value: unknown,
  defaultSort: PersistedSort<SortKey>,
  allowedKeys: readonly SortKey[]
): PersistedSort<SortKey> {
  if (!isSortRecord(value)) {
    return defaultSort;
  }

  if (value.direction !== "asc" && value.direction !== "desc") {
    return defaultSort;
  }

  if (!allowedKeys.includes(value.key as SortKey)) {
    return defaultSort;
  }

  return {
    direction: value.direction,
    key: value.key as SortKey,
  };
}

function isSortRecord(value: unknown): value is {
  direction: unknown;
  key: unknown;
} {
  return typeof value === "object" && value !== null;
}
