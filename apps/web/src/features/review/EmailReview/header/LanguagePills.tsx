import { Tooltip } from "@mantine/core";

import { formatEmailReviewStatus } from "../../../emails/reviewStatus";
import type { EmailReviewStatus } from "../../../emails/types";
import styles from "../EmailReview.module.css";

// The fixed set of languages a slot can have, EN (the master) first.
const languageOrder = ["en", "de", "es", "pl", "br"];

export type LanguagePillVersion = {
  id: string;
  language: string;
  review_status?: EmailReviewStatus;
  translation_stale?: boolean;
};

type PillState = "missing" | "stale" | EmailReviewStatus;

// LanguagePills lists every language of the slot as a pill with a dot for its
// state: no translation yet, outdated against the EN master, or its status.
export function LanguagePills({
  selectedEmailId,
  versions,
  onSelect,
}: {
  selectedEmailId: string;
  versions: LanguagePillVersion[];
  onSelect: (emailId: string) => void;
}) {
  const extraLanguages = versions
    .map((version) => version.language)
    .filter((language) => !languageOrder.includes(language));
  const languages = [...languageOrder, ...extraLanguages];

  return (
    <div aria-label="Email language" className={styles.languagePills} role="group">
      {languages.map((language) => {
        const version = versions.find((item) => item.language === language);
        const state: PillState = !version
          ? "missing"
          : version.translation_stale
            ? "stale"
            : (version.review_status ?? "draft");
        const label = language.toUpperCase();

        if (!version) {
          return (
            <Tooltip key={language} label={`${label}: no translation yet`}>
              <span
                aria-disabled="true"
                className={styles.languagePill}
                data-state="missing"
              >
                <span className={styles.languagePillDot} data-state={state} />
                {label}
              </span>
            </Tooltip>
          );
        }
        return (
          <Tooltip key={language} label={`${label}: ${stateLabel(state)}`}>
            <button
              aria-current={version.id === selectedEmailId ? "true" : undefined}
              className={styles.languagePill}
              data-active={version.id === selectedEmailId || undefined}
              type="button"
              onClick={() => onSelect(version.id)}
            >
              <span className={styles.languagePillDot} data-state={state} />
              {label}
            </button>
          </Tooltip>
        );
      })}
    </div>
  );
}

function stateLabel(state: PillState) {
  if (state === "stale") {
    return "outdated, the EN master changed";
  }
  if (state === "missing") {
    return "no translation yet";
  }
  return formatEmailReviewStatus(state);
}
