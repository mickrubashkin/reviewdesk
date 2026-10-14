import { Button, Select, Stack, Text, Title } from "@mantine/core";
import { HouseIcon } from "@phosphor-icons/react";
import { Link, useLocation, useNavigate } from "react-router-dom";

import styles from "./AdminTableHeader.module.css";

const adminScreenOptions = [
  { label: "AI analysis logs", value: "/ai-logs" },
  { label: "Auth events", value: "/auth-events" },
  { label: "Operational events", value: "/admin/operational-events" },
  { label: "Email events", value: "/admin/email-events" },
  { label: "Users", value: "/admin/users" },
];

export function AdminTableHeader({
  subtitle,
  title,
}: {
  subtitle: string;
  title: string;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const currentScreen =
    adminScreenOptions.find((option) => option.value === location.pathname)
      ?.value ?? null;

  return (
    <header className={styles.header}>
      <Button
        className={styles.homeButton}
        component={Link}
        leftSection={<HouseIcon aria-hidden="true" size={16} />}
        to="/"
        variant="light"
      >
        Home
      </Button>
      <Stack className={styles.titleBlock} gap={4}>
        <Title order={2}>{title}</Title>
        <Text c="dimmed" size="sm">
          {subtitle}
        </Text>
      </Stack>
      <Select
        allowDeselect={false}
        className={styles.screenSelect}
        data={adminScreenOptions}
        label="Admin screen"
        placeholder="Choose screen"
        value={currentScreen}
        onChange={(value) => {
          if (value && value !== location.pathname) {
            navigate(value);
          }
        }}
      />
    </header>
  );
}
