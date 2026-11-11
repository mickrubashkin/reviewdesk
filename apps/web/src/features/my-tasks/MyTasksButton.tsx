import { Badge, Button, Group, Tooltip } from "@mantine/core";
import { ListChecksIcon } from "@phosphor-icons/react";
import { Link } from "react-router-dom";

import type { AuthUser } from "../emails/types";
import { useMyTasks } from "./tasks";
import styles from "../../App.module.css";

// MyTasksButton links to My tasks and shows how many emails wait there, and
// how many of them came up since it was last opened.
export function MyTasksButton({
  currentUser,
  fullWidth = false,
}: {
  currentUser: AuthUser;
  fullWidth?: boolean;
}) {
  const { count, freshCount } = useMyTasks(currentUser, currentUser.my_tasks_seen_at ?? null);
  const label = count > 0 ? `My tasks: ${count}${freshCount > 0 ? `, ${freshCount} new` : ""}` : "My tasks";
  return (
    <Tooltip
      disabled={fullWidth}
      label={label}
    >
      <Button
        aria-label={label}
        className={fullWidth ? undefined : `${styles.myEmailsToggle} ${styles.collapsibleButton}`}
        component={Link}
        data-tour="my-tasks"
        fullWidth={fullWidth}
        leftSection={<ListChecksIcon aria-hidden="true" size={16} />}
        rightSection={
          count > 0 ? (
            <Group gap={2} wrap="nowrap">
              <Badge color="red" size="xs" variant="filled">
                {count}
              </Badge>
              {freshCount > 0 ? (
                <Badge color="blue" size="xs" variant="filled">
                  {freshCount} new
                </Badge>
              ) : null}
            </Group>
          ) : undefined
        }
        size={fullWidth ? "xs" : "sm"}
        to="/my-tasks"
        variant="white"
      >
        <span className={fullWidth ? undefined : styles.collapsibleLabel}>
          My tasks
        </span>
      </Button>
    </Tooltip>
  );
}
