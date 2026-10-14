export const boardSelectedVersionsStorageKey = "reviewdesk.board.selectedVersions";
export const boardScrollPositionStorageKey = "reviewdesk.board.scrollPosition";
export const boardFilterStorageKey = "reviewdesk.board.filter";
export const boardSearchStorageKey = "reviewdesk.board.search";
export const boardViewStorageKey = "reviewdesk.board.view";

export type BoardView = "cards" | "matrix";

export type BoardCommentFilter = "all" | "open";

export type BoardFilters = {
  owner: string;
  production: string;
  translation: string;
  comments: BoardCommentFilter;
  language: string;
  adaptation: string;
  reviewer: string;
  variant: string;
  status: string;
};

export type BoardFilterOption = {
  label: string;
  value: string;
};

export type BoardFilterOptions = {
  languages: BoardFilterOption[];
  adaptations: BoardFilterOption[];
  variants: BoardFilterOption[];
  owners: BoardFilterOption[];
  reviewers: BoardFilterOption[];
};

export const defaultBoardFilters: BoardFilters = {
  owner: "",
  production: "",
  translation: "",
  comments: "all",
  language: "",
  adaptation: "",
  reviewer: "",
  variant: "",
  status: "",
};

export type HandoffFilters = {
  adaptations: string[];
  languages: string[];
  stages: string[];
};

export const defaultHandoffFilters: HandoffFilters = {
  adaptations: [],
  languages: [],
  stages: [],
};
