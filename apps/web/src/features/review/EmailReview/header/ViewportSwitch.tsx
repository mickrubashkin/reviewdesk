import { Group } from "@mantine/core";
import { DeviceMobileIcon, MonitorIcon } from "@phosphor-icons/react";
import type { ReviewViewport } from "../EmailReview.types";
import styles from "../EmailReview.module.css";

export function ViewportSwitch({
  onChange,
  viewport,
}: {
  onChange: (viewport: ReviewViewport) => void;
  viewport: ReviewViewport;
}) {
  return (
    <Group className={styles.segmentedControl} gap={0}>
      <button
        aria-label="Desktop preview"
        className={styles.iconSegmentedButton}
        data-active={viewport === "desktop" || undefined}
        type="button"
        onClick={() => onChange("desktop")}
      >
        <MonitorIcon aria-hidden="true" size={16} />
      </button>

      <button
        aria-label="Mobile preview"
        className={styles.iconSegmentedButton}
        data-active={viewport === "mobile" || undefined}
        type="button"
        onClick={() => onChange("mobile")}
      >
        <DeviceMobileIcon aria-hidden="true" size={16} />
      </button>
    </Group>
  );
}
