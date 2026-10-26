export const boardSelectedVersionsStorageKey = "reviewdesk.board.selectedVersions";
export const boardScrollPositionStorageKey = "reviewdesk.board.scrollPosition";
export const boardFilterStorageKey = "reviewdesk.board.filter";
export const boardSearchStorageKey = "reviewdesk.board.search";
export const boardViewStorageKey = "reviewdesk.board.view";
export const timelineCollapsedStorageKey = "reviewdesk.board.timelineCollapsed";
export const timelineByRegionStorageKey = "reviewdesk.board.timelineByRegion";
export const matrixByRegionStorageKey = "reviewdesk.board.matrixByRegion";

// "archive" is the board's history: archived emails, which can be restored.
export const boardViews = ["cards", "matrix", "timeline", "archive"] as const;
export type BoardView = (typeof boardViews)[number];

export function isBoardView(value: string | null): value is BoardView {
  return boardViews.includes(value as BoardView);
}

// Draft and Ready are the email's own status; Live comes from the portal
// sync, so a live email being reworked matches both Live and Draft.
export const boardEmailStates = ["draft", "ready", "live"] as const;
export type BoardEmailState = (typeof boardEmailStates)[number];

export type BoardFilters = {
  // The current user's address when "My emails" is on.
  mine: string;
  openComments: boolean;
  staleTranslation: boolean;
  needsUpload: boolean;
  languages: string[];
  states: BoardEmailState[];
  owner: string;
  variant: string;
  adaptation: string;
  // A region key, or noRegionFilter; on boards with regions it replaces the
  // adaptation filter.
  region: string;
};

export const noRegionFilter = "__none__";

export type BoardFilterOption = {
  label: string;
  value: string;
};

export type BoardFilterOptions = {
  languages: BoardFilterOption[];
  adaptations: BoardFilterOption[];
  // Empty on boards without regions.
  regions: BoardFilterOption[];
  variants: BoardFilterOption[];
  owners: BoardFilterOption[];
};

// Board-wide counts shown next to the quick filters.
export type BoardFilterCounts = {
  openComments: number;
  staleTranslation: number;
  needsUpload: number;
};

export type BoardFilterChange = <K extends keyof BoardFilters>(
  key: K,
  value: BoardFilters[K]
) => void;

export const defaultBoardFilters: BoardFilters = {
  mine: "",
  openComments: false,
  staleTranslation: false,
  needsUpload: false,
  languages: [],
  states: [],
  owner: "",
  variant: "",
  adaptation: "",
  region: "",
};
