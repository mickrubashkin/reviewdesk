import { Text } from "@mantine/core";

import styles from "../../../App.module.css";

const updatedAt = new Date(__APP_UPDATED_AT__);
const updatedLabel = Number.isNaN(updatedAt.getTime())
  ? ""
  : updatedAt.toLocaleDateString(undefined, {
      day: "numeric",
      month: "short",
    });

export function AppVersionInfo({ onDark = false }: { onDark?: boolean }) {
  // Kept short for the crowded board header; the commit and exact time are
  // in the tooltip.
  const parts = [`v${__APP_VERSION__}`, updatedLabel].filter(Boolean);

  return (
    <Text
      c={onDark ? undefined : "dimmed"}
      className={onDark ? styles.appVersionInfo : undefined}
      size="xs"
      title={`Commit ${__APP_COMMIT__ || "unknown"} · last update ${updatedAt.toLocaleString()}`}
    >
      {parts.join(" · ")}
    </Text>
  );
}
