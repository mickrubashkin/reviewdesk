import {
  boardFilterStorageKey,
  boardScrollPositionStorageKey,
  boardSearchStorageKey,
  boardSelectedVersionsStorageKey,
  boardViewStorageKey,
  defaultBoardFilters,
  type BoardFilters,
  type BoardView,
} from "./BoardHome.types";

export function readStoredSelectedVersions() {
  return readSessionStorageValue<Record<string, string>>(
    boardSelectedVersionsStorageKey,
    {}
  );
}


export function readStoredBoardScrollPosition() {
  return readSessionStorageValue<{ x: number; y: number }>(
    boardScrollPositionStorageKey,
    { x: 0, y: 0 }
  );
}


export function readStoredBoardFilters(): BoardFilters {
  const storedFilters = readSessionStorageValue<string | Partial<BoardFilters>>(
    boardFilterStorageKey,
    defaultBoardFilters
  );

  if (storedFilters === "open" || storedFilters === "all") {
    return { ...defaultBoardFilters, comments: storedFilters };
  }

  if (typeof storedFilters !== "object" || storedFilters === null) {
    return defaultBoardFilters;
  }

  return {
    ...defaultBoardFilters,
    ...storedFilters,
    comments: storedFilters.comments === "open" ? "open" : "all",
  };
}


export function readStoredBoardView(): BoardView {
  return readSessionStorageValue<string>(boardViewStorageKey, "cards") === "matrix" ? "matrix" : "cards";
}


export function readStoredBoardSearch() {
  return readSessionStorageValue<string>(boardSearchStorageKey, "");
}


function readSessionStorageValue<T>(key: string, fallback: T): T {
  try {
    const storedValue = window.sessionStorage.getItem(key);
    return storedValue ? (JSON.parse(storedValue) as T) : fallback;
  } catch {
    return fallback;
  }
}


export function writeSessionStorageValue<T>(key: string, value: T) {
  try {
    window.sessionStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Session storage may be unavailable in restricted browser contexts.
  }
}

