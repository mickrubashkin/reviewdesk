import type {
  EmailListItem,
  EmailVariant,
  EmailVersionGroup,
  StageColumn,
} from "./types";

export const stageColors = [
  "#4A6FA5",
  "#5C85B3",
  "#6F9BBE",
  "#6A9FB5",
  "#8FB7A3",
  "#A7C48A",
  "#B8CF7A",
  "#8FBF6A",
  "#5FA85C",
  "#7BD500",
  "#FF5752",
  "#FF5752",
];

const preferredVersionOrder = ["en", "es", "br", "pl", "de", "old"];
const legacyVariantOrder: EmailVariant[] = ["new", "old"];

export function formatStageName(stage: string): string {
  return stage
    .split(/[-_]/)
    .filter(Boolean)
    .map((word) => `${word.charAt(0).toUpperCase()}${word.slice(1)}`)
    .join(" ");
}

export function sortEmailVersions(versions: EmailListItem[]): EmailListItem[] {
  return [...versions].sort((first, second) => {
    const firstVariantIndex = variantSortIndex(first.variant);
    const secondVariantIndex = variantSortIndex(second.variant);
    if (firstVariantIndex !== secondVariantIndex) {
      return firstVariantIndex - secondVariantIndex;
    }
    if (first.variant !== second.variant) {
      return first.variant.localeCompare(second.variant);
    }

    const firstIndex = preferredVersionOrder.indexOf(first.language);
    const secondIndex = preferredVersionOrder.indexOf(second.language);
    const normalizedFirstIndex =
      firstIndex === -1 ? preferredVersionOrder.length : firstIndex;
    const normalizedSecondIndex =
      secondIndex === -1 ? preferredVersionOrder.length : secondIndex;

    if (normalizedFirstIndex !== normalizedSecondIndex) {
      return normalizedFirstIndex - normalizedSecondIndex;
    }

    if (first.language !== second.language) {
      return first.language.localeCompare(second.language);
    }

    if (first.adaptation_key === "default" && second.adaptation_key !== "default") {
      return -1;
    }
    if (second.adaptation_key === "default" && first.adaptation_key !== "default") {
      return 1;
    }

    return first.adaptation_label.localeCompare(second.adaptation_label);
  });
}

export function getDefaultVersion(versions: EmailListItem[]): EmailListItem {
  return (
    getDefaultVersionForVariant(versions, "v1") ??
    getDefaultVersionForVariant(versions, "new") ??
    sortEmailVersions(versions)[0]
  );
}

export function getAvailableVariants(versions: EmailListItem[]): EmailVariant[] {
  return Array.from(new Set(versions.map((version) => version.variant))).sort(
    (first, second) => {
      const firstIndex = variantSortIndex(first);
      const secondIndex = variantSortIndex(second);
      if (firstIndex !== secondIndex) {
        return firstIndex - secondIndex;
      }

      return first.localeCompare(second);
    }
  );
}

export function getVersionsForVariant(
  versions: EmailListItem[],
  variant: EmailVariant
): EmailListItem[] {
  return sortEmailVersions(
    versions.filter((version) => version.variant === variant)
  );
}

function variantSortIndex(variant: EmailVariant) {
  const versionMatch = /^v(\d+)$/i.exec(variant);
  if (versionMatch) {
    return Number(versionMatch[1]);
  }

  const legacyIndex = legacyVariantOrder.indexOf(variant);
  if (legacyIndex >= 0) {
    return 10_000 + legacyIndex;
  }

  return 5_000;
}

export function getAvailableAdaptations(
  versions: EmailListItem[],
  variant: EmailVariant
): EmailListItem[] {
  const byKey = new Map<string, EmailListItem>();
  for (const version of getVersionsForVariant(versions, variant)) {
    if (!byKey.has(version.adaptation_key)) {
      byKey.set(version.adaptation_key, version);
    }
  }
  return [...byKey.values()];
}

export function getVersionsForVariantAndAdaptation(
  versions: EmailListItem[],
  variant: EmailVariant,
  adaptationKey: string
): EmailListItem[] {
  return getVersionsForVariant(versions, variant).filter(
    (version) => version.adaptation_key === adaptationKey
  );
}

export function getSelectedAdaptation(
  versions: EmailListItem[],
  selectedEmailId: string | undefined | null
): string {
  return (
    versions.find((version) => version.id === selectedEmailId)?.adaptation_key ??
    "default"
  );
}

export function getSelectedVariant(
  versions: EmailListItem[],
  selectedEmailId: string | undefined | null
): EmailVariant {
  const selectedEmail = versions.find((version) => version.id === selectedEmailId);
  return selectedEmail?.variant ?? getDefaultVersion(versions).variant;
}

export function getDefaultVersionForVariant(
  versions: EmailListItem[],
  variant: EmailVariant
): EmailListItem | undefined {
  const variantVersions = getVersionsForVariant(versions, variant);
  const defaultAdaptationVersions = variantVersions.filter(
    (version) => version.adaptation_key === "default"
  );
  return (
    defaultAdaptationVersions.find((version) => version.language === "en") ??
    defaultAdaptationVersions[0] ??
    variantVersions.find((version) => version.language === "en") ??
    variantVersions[0]
  );
}

export function getVersionForVariant(
  versions: EmailListItem[],
  variant: EmailVariant,
  preferredLanguage?: string,
  preferredAdaptation = "default"
): EmailListItem | undefined {
  const variantVersions = getVersionsForVariant(versions, variant);
  const adaptationVersions = variantVersions.filter(
    (version) => version.adaptation_key === preferredAdaptation
  );
  const defaultAdaptationVersions = variantVersions.filter(
    (version) => version.adaptation_key === "default"
  );
  return (
    adaptationVersions.find((version) => version.language === preferredLanguage) ??
    adaptationVersions.find((version) => version.language === "en") ??
    adaptationVersions[0] ??
    defaultAdaptationVersions.find((version) => version.language === preferredLanguage) ??
    defaultAdaptationVersions.find((version) => version.language === "en") ??
    defaultAdaptationVersions[0] ??
    getDefaultVersionForVariant(versions, variant)
  );
}

export function buildStageColumns(
  emails: EmailListItem[],
  stageOrder: string[] = []
): StageColumn[] {
  const groupsByKey = new Map<string, EmailVersionGroup>();

  for (const email of emails) {
    const stage = email.stage || "uncategorized";
    const groupKey = [email.sequence, stage, email.sort_order].join("/");
    const emailGroup = groupsByKey.get(groupKey) ?? {
      key: groupKey,
      stage,
      sortOrder: email.sort_order,
      versions: [],
    };

    emailGroup.versions.push(email);
    groupsByKey.set(groupKey, emailGroup);
  }

  const groupsByStage = new Map<string, EmailVersionGroup[]>();
  const explicitStageOrder = new Map(
    stageOrder.map((stage, index) => [stage, index])
  );

  for (const stage of stageOrder) {
    groupsByStage.set(stage, []);
  }

  for (const group of groupsByKey.values()) {
    const stageGroups = groupsByStage.get(group.stage) ?? [];

    stageGroups.push({
      ...group,
      versions: sortEmailVersions(group.versions),
    });
    groupsByStage.set(group.stage, stageGroups);
  }

  return Array.from(groupsByStage.entries())
    .map(([stage, emailGroups]) => {
      const sortedGroups = [...emailGroups].sort(
        (first, second) => first.sortOrder - second.sortOrder
      );
      return {
        stage,
        title: formatStageName(stage),
        sortOrder:
          sortedGroups[0]?.sortOrder ??
          explicitStageOrder.get(stage) ??
          0,
        emailGroups: sortedGroups,
      };
    })
    .sort((first, second) => {
      const firstIndex = explicitStageOrder.get(first.stage);
      const secondIndex = explicitStageOrder.get(second.stage);
      if (firstIndex !== undefined || secondIndex !== undefined) {
        return (
          (firstIndex ?? Number.MAX_SAFE_INTEGER) -
          (secondIndex ?? Number.MAX_SAFE_INTEGER)
        );
      }

      return first.sortOrder - second.sortOrder;
    });
}
