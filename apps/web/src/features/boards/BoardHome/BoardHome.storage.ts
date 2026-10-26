import {
  boardEmailStates,
  boardFilterStorageKey,
  boardScrollPositionStorageKey,
  boardSearchStorageKey,
  boardSelectedVersionsStorageKey,
  boardViewStorageKey,
  defaultBoardFilters,
  isBoardView,
  matrixByRegionStorageKey,
  type BoardEmailState,
  type BoardFilters,
  timelineByRegionStorageKey,
  timelineCollapsedStorageKey,
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
  const stored = readSessionStorageValue<unknown>(
    boardFilterStorageKey,
    defaultBoardFilters
  );
  if (typeof stored !== "object" || stored === null) {
    return defaultBoardFilters;
  }

  // Filters saved by an older build may have other keys or shapes; keep only
  // the values that still fit.
  const value = stored as Record<string, unknown>;
  const text = (key: string) =>
    typeof value[key] === "string" ? (value[key] as string) : "";
  const flag = (key: string) => value[key] === true;
  const list = (key: string) =>
    Array.isArray(value[key])
      ? (value[key] as unknown[]).filter((item) => typeof item === "string")
      : [];

  return {
    mine: text("mine"),
    openComments: flag("openComments"),
    staleTranslation: flag("staleTranslation"),
    needsUpload: flag("needsUpload"),
    languages: list("languages") as string[],
    states: list("states").filter((state): state is BoardEmailState =>
      boardEmailStates.includes(state as BoardEmailState)
    ),
    owner: text("owner"),
    variant: text("variant"),
    adaptation: text("adaptation"),
    region: text("region"),
  };
}


// The timeline is the default: it shows the whole chain and where each
// email sits in it.
export function readStoredBoardView(): BoardView {
  const stored = readSessionStorageValue<string>(boardViewStorageKey, "timeline");
  return isBoardView(stored) ? stored : "timeline";
}


export function readStoredMatrixByRegion() {
  return readSessionStorageValue<unknown>(matrixByRegionStorageKey, false) === true;
}


// On a board with regions the timeline shows a branch per region unless the
// reader chose languages.
export function readStoredTimelineByRegion() {
  return readSessionStorageValue<unknown>(timelineByRegionStorageKey, true) !== false;
}


// The stages folded away in the timeline, per board.
export function readStoredTimelineCollapsed(boardKey: string): string[] {
  const stored = readSessionStorageValue<unknown>(timelineCollapsedStorageKey, {});
  const list = typeof stored === "object" && stored !== null ? (stored as Record<string, unknown>)[boardKey] : null;
  return Array.isArray(list) ? list.filter((stage): stage is string => typeof stage === "string") : [];
}


export function writeStoredTimelineCollapsed(boardKey: string, stages: string[]) {
  const stored = readSessionStorageValue<Record<string, string[]>>(timelineCollapsedStorageKey, {});
  writeSessionStorageValue(timelineCollapsedStorageKey, { ...stored, [boardKey]: stages });
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

