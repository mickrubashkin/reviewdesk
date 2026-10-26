import type { BoardRegion } from "../emails/types";

// On a board with regions an email's adaptation follows its region (see
// SyncRegionAdaptations in the API): the main region of a language is the
// default version, any other region an adaptation named after it.

// mainRegion is the region whose emails are the default version in a
// language: the reference for its language, else the first region with it.
export function mainRegion(regions: BoardRegion[], language: string): BoardRegion | undefined {
  const reference = regions.find((region) => region.reference && region.language === language);
  return reference ?? regions.find((region) => region.language === language);
}

// versionName names a version of a slot for people: on a board with regions
// the default version is the main region of the language shown.
export function versionName(
  version: { adaptation_key: string; adaptation_label: string },
  language: string,
  regions: BoardRegion[]
): string {
  if (regions.length > 0 && version.adaptation_key === "default") {
    return mainRegion(regions, language)?.name ?? version.adaptation_label;
  }
  return version.adaptation_label;
}

// regionName is the name of an email's region, "" without one.
export function regionName(email: { region_key?: string | null }, regions: BoardRegion[]): string {
  return regions.find((region) => region.key === email.region_key)?.name ?? "";
}
