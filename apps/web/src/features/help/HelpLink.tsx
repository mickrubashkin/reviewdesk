import { ActionIcon, Tooltip } from "@mantine/core";
import { QuestionIcon } from "@phosphor-icons/react";

import { helpTopics, type HelpTopic } from "./help";

// HelpLink is the "?" next to a hard part of the app. It opens that part of
// the help in a new tab, so whatever is open here stays as it is.
export function HelpLink({ topic, onDark = false }: { topic: HelpTopic; onDark?: boolean }) {
  const { path, label } = helpTopics[topic];
  return (
    <Tooltip label={`${label} (opens Help)`}>
      <ActionIcon
        aria-label={`Help: ${label}`}
        c={onDark ? "rgba(255, 255, 255, 0.72)" : undefined}
        color="gray"
        component="a"
        href={`/help/${path}`}
        radius="xl"
        rel="noopener"
        size="sm"
        target="_blank"
        variant="subtle"
      >
        <QuestionIcon aria-hidden="true" size={14} />
      </ActionIcon>
    </Tooltip>
  );
}
