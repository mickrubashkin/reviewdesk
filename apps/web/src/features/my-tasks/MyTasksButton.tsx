import { Badge, Button, Tooltip } from "@mantine/core";
import { ListChecksIcon } from "@phosphor-icons/react";
import { Link } from "react-router-dom";

import type { AuthUser } from "../emails/types";
import { useMyTasks } from "./tasks";
import styles from "../../App.module.css";

// MyTasksButton links to My tasks and shows how many emails wait there.
export function MyTasksButton({
  currentUser,
  fullWidth = false,
}: {
  currentUser: AuthUser;
  fullWidth?: boolean;
}) {
  const { count } = useMyTasks(currentUser);
  return (
    <Tooltip
      disabled={fullWidth}
      label={count > 0 ? `My tasks: ${count}` : "My tasks"}
    >
      <Button
        aria-label={count > 0 ? `My tasks: ${count}` : "My tasks"}
        className={fullWidth ? undefined : `${styles.myEmailsToggle} ${styles.collapsibleButton}`}
        component={Link}
        fullWidth={fullWidth}
        leftSection={<ListChecksIcon aria-hidden="true" size={16} />}
        rightSection={
          count > 0 ? (
            <Badge color="red" size="xs" variant="filled">
              {count}
            </Badge>
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
