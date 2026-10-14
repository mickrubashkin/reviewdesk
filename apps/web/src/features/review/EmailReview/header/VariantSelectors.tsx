import { Select } from "@mantine/core";
import type { EmailVariant } from "../../../emails/types";
import styles from "../EmailReview.module.css";

export function LanguageSelect({
  onSelect,
  selectedEmailId,
  versions,
}: {
  onSelect: (emailId: string) => void;
  selectedEmailId: string;
  versions: Array<{ id: string; language: string }>;
}) {
  return (
    <Select
      allowDeselect={false}
      aria-label="Email language"
      className={styles.headerCompactSelect}
      classNames={{ input: styles.headerSelectInput }}
      data={versions.map((version) => ({
        label: version.language.toUpperCase(),
        value: version.id,
      }))}
      disabled={versions.length <= 1}
      size="xs"
      value={selectedEmailId}
      onChange={(value) => {
        if (value) {
          onSelect(value);
        }
      }}
    />
  );
}

export function AdaptationSelect({
  adaptations,
  onSelect,
  selectedAdaptation,
}: {
  adaptations: Array<{ adaptation_key: string; adaptation_label: string }>;
  onSelect: (adaptationKey: string) => void;
  selectedAdaptation: string;
}) {
  return (
    <Select
      allowDeselect={false}
      aria-label="Email adaptation"
      className={styles.headerVersionSelect}
      classNames={{ input: styles.headerSelectInput }}
      data={adaptations.map((adaptation) => ({
        label: adaptation.adaptation_label,
        value: adaptation.adaptation_key,
      }))}
      size="xs"
      value={selectedAdaptation}
      onChange={(value) => {
        if (value) {
          onSelect(value);
        }
      }}
    />
  );
}

export function VariantSelect({
  availableVariants,
  onSelect,
  selectedVariant,
}: {
  availableVariants: EmailVariant[];
  onSelect: (variant: EmailVariant) => void;
  selectedVariant: EmailVariant;
}) {
  return (
    <Select
      allowDeselect={false}
      aria-label="Email version"
      className={styles.headerVersionSelect}
      classNames={{ input: styles.headerSelectInput }}
      data={availableVariants.map((variant) => ({
        label: variant,
        value: variant,
      }))}
      size="xs"
      value={selectedVariant}
      onChange={(value) => {
        if (value) {
          onSelect(value);
        }
      }}
    />
  );
}

export function ReadOnlyVersionValue({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <span aria-label={label} className={styles.readOnlySelect}>
      {value || "None"}
    </span>
  );
}
