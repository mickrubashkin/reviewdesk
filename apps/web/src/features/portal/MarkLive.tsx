import { useState } from "react";
import { Button, Popover, Stack, TextInput } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation } from "@tanstack/react-query";

import { markEmailLive } from "../emails/api";

// MarkLiveButton confirms by hand that an email is live (super admin only),
// for emails the portal sync cannot see, e.g. sent from the admin panel.
export function MarkLiveButton({ emailId, onDone }: { emailId: string; onDone: () => void }) {
  const [opened, setOpened] = useState(false);
  const [note, setNote] = useState("");
  const mutation = useMutation({
    mutationFn: () => markEmailLive(emailId, { live: true, note }),
    onSuccess: () => {
      setOpened(false);
      setNote("");
      onDone();
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });

  return (
    <Popover opened={opened} position="bottom-end" shadow="md" trapFocus width={300} withinPortal onChange={setOpened}>
      <Popover.Target>
        <Button size="compact-xs" variant="light" onClick={() => setOpened((o) => !o)}>
          Mark live
        </Button>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack gap="xs">
          <TextInput
            data-autofocus
            label="Where is it sent from?"
            placeholder="Partner admin panel"
            size="xs"
            value={note}
            onChange={(event) => setNote(event.currentTarget.value)}
          />
          <Button loading={mutation.isPending} size="xs" onClick={() => mutation.mutate()}>
            Confirm it is live
          </Button>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  );
}

export function UnmarkLiveButton({ emailId, onDone }: { emailId: string; onDone: () => void }) {
  const mutation = useMutation({
    mutationFn: () => markEmailLive(emailId, { live: false }),
    onSuccess: onDone,
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });
  return (
    <Button color="red" loading={mutation.isPending} size="compact-xs" variant="subtle" onClick={() => mutation.mutate()}>
      Unmark
    </Button>
  );
}
