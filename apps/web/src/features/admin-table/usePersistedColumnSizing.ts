import { useCallback, useState } from "react";
import type { ColumnSizingState, OnChangeFn, Updater } from "@tanstack/react-table";

export function usePersistedColumnSizing(storageKey: string) {
  const [columnSizing, setColumnSizingState] = useState<ColumnSizingState>(() =>
    readStoredColumnSizing(storageKey)
  );

  const setColumnSizing: OnChangeFn<ColumnSizingState> = useCallback(
    (updater) => {
      setColumnSizingState((current) => {
        const next = applyUpdater(updater, current);
        if (isSameColumnSizing(current, next)) {
          return current;
        }
        writeStoredColumnSizing(storageKey, next);
        return next;
      });
    },
    [storageKey]
  );

  return [columnSizing, setColumnSizing] as const;
}

function applyUpdater<T>(updater: Updater<T>, current: T) {
  return typeof updater === "function"
    ? (updater as (old: T) => T)(current)
    : updater;
}

function readStoredColumnSizing(storageKey: string): ColumnSizingState {
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (!raw) {
      return {};
    }
    return normalizeColumnSizing(JSON.parse(raw));
  } catch {
    return {};
  }
}

function writeStoredColumnSizing(
  storageKey: string,
  columnSizing: ColumnSizingState
) {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(columnSizing));
  } catch {
    // Resizing should still work if localStorage is blocked or full.
  }
}

function isSameColumnSizing(
  current: ColumnSizingState,
  next: ColumnSizingState
) {
  const currentEntries = Object.entries(current);
  const nextEntries = Object.entries(next);
  if (currentEntries.length !== nextEntries.length) {
    return false;
  }

  return currentEntries.every(([key, value]) => next[key] === value);
}

function normalizeColumnSizing(value: unknown): ColumnSizingState {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return {};
  }

  return Object.fromEntries(
    Object.entries(value).filter((entry): entry is [string, number] => {
      const [, size] = entry;
      return typeof size === "number" && Number.isFinite(size) && size > 0;
    })
  );
}
