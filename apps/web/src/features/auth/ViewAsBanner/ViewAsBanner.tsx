import { Affix, Button, Group, Paper, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { EyeIcon } from "@phosphor-icons/react";
import { useMutation } from "@tanstack/react-query";

import type { AuthUser } from "../../emails/types";
import { stopViewAs } from "../api";

// ViewAsBanner stays on screen while a CRM operator views ReviewDesk as
// someone else, says whom, and leads back. A full page load drops what the
// cache holds as that person.
export function ViewAsBanner({ currentUser }: { currentUser: AuthUser }) {
  const mutation = useMutation({
    mutationFn: stopViewAs,
    onSuccess: () => window.location.assign("/admin/users"),
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not closed" }),
  });
  return (
    <Affix position={{ bottom: 16, left: "50%" }} style={{ transform: "translateX(-50%)" }} zIndex={1000}>
      <Paper bg="var(--mantine-color-body)" maw="calc(100vw - 32px)" px="md" py="xs" radius="xl" shadow="md" style={{ borderColor: "var(--mantine-color-orange-filled)" }} withBorder>
        <Group gap="sm" wrap="nowrap">
          <EyeIcon aria-hidden="true" size={18} />
          <Text size="sm">
            Viewing as <b>{currentUser.email}</b>, read-only
          </Text>
          <Button loading={mutation.isPending} size="compact-sm" variant="filled" color="orange" onClick={() => mutation.mutate()}>
            Exit
          </Button>
        </Group>
      </Paper>
    </Affix>
  );
}
