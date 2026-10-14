import { Button, Group, Stack, Text, Title } from "@mantine/core";
import { HouseIcon } from "@phosphor-icons/react";
import { Link } from "react-router-dom";

import styles from "./EmailCreate.module.css";

export function EmailCreateHeader() {
  return (
    <header className={styles.header}>
      <div className={styles.headerInner}>
        <Group gap="sm" wrap="nowrap" className={styles.titleBlock}>
          <Button
            component={Link}
            to="/"
            leftSection={<HouseIcon aria-hidden="true" size={16} />}
            size="xs"
            variant="subtle"
          >
            Home
          </Button>
          <Stack gap={2} className={styles.titleBlock}>
            <Title className={styles.title} lineClamp={1} order={4}>
              Create email
            </Title>
            <Text c="dimmed" lineClamp={1} size="xs">
              Add a finished HTML email to the review board.
            </Text>
          </Stack>
        </Group>
      </div>
    </header>
  );
}
