import { useState } from "react";
import { Button, Group, NumberInput, Select, Switch, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { savePortalSyncSettings, type PortalCategory, type PortalSyncSettings } from "../api";

// Mount with key={boardKey} so the form starts from the saved settings of
// the board on screen.
export function AutoSyncSettings({
  boardKey,
  categories,
  settings,
}: {
  boardKey: string;
  categories: PortalCategory[];
  settings: PortalSyncSettings;
}) {
  const queryClient = useQueryClient();
  const [autoSync, setAutoSync] = useState(settings.auto_sync);
  const [categoryID, setCategoryID] = useState<string | null>(
    settings.category_id !== null ? String(settings.category_id) : null
  );
  const [days, setDays] = useState<number | string>(settings.days);

  const next: PortalSyncSettings = {
    auto_sync: autoSync,
    category_id: categoryID !== null ? Number(categoryID) : null,
    days: Number(days) || 0,
  };
  const dirty =
    next.auto_sync !== settings.auto_sync ||
    next.category_id !== settings.category_id ||
    next.days !== settings.days;
  const invalid = next.days < 1 || next.days > 365 || (next.auto_sync && next.category_id === null);

  const saveMutation = useMutation({
    mutationFn: () => savePortalSyncSettings(boardKey, next),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["portal-overview", boardKey] });
      notifications.show({ color: "green", message: "Nightly sync settings saved." });
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });

  return (
    <div>
      <Group align="flex-end">
        <Switch
          checked={autoSync}
          label="Sync every night at 03:00 (UTC+2)"
          mb={8}
          onChange={(event) => setAutoSync(event.currentTarget.checked)}
        />
        <Select
          allowDeselect={false}
          data={categories.map((c) => ({ label: `${c.name} (${c.id})`, value: String(c.id) }))}
          disabled={!autoSync}
          label="Funnel"
          miw={260}
          placeholder="Choose a funnel"
          value={categoryID}
          onChange={setCategoryID}
        />
        <NumberInput disabled={!autoSync} label="Days back" max={365} min={1} value={days} w={110} onChange={setDays} />
        <Button
          disabled={!dirty || invalid}
          loading={saveMutation.isPending}
          variant="light"
          onClick={() => saveMutation.mutate()}
        >
          Save
        </Button>
      </Group>
      <Text c="dimmed" mt={4} size="xs">
        A few days back is enough: each night adds new sends to what is already loaded, and a missed night is
        caught up by the next one.
      </Text>
    </div>
  );
}
