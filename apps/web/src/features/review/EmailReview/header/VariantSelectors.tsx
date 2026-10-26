import { Select } from "@mantine/core";
import { formatVariantOptionLabel } from "../../../emails/EmailCard/EmailCard.helpers";
import type { EmailVariant } from "../../../emails/types";
import styles from "../EmailReview.module.css";

export function AdaptationSelect({
  adaptations,
  hasRegions,
  onSelect,
  selectedAdaptation,
}: {
  adaptations: Array<{ adaptation_key: string; adaptation_label: string }>;
  // On a board with regions the adaptations are its regions.
  hasRegions: boolean;
  onSelect: (adaptationKey: string) => void;
  selectedAdaptation: string;
}) {
  return (
    <Select
      allowDeselect={false}
      aria-label={hasRegions ? "Email region" : "Email adaptation"}
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
        label: formatVariantOptionLabel(variant, availableVariants),
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
