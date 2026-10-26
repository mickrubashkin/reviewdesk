import { Button, Menu } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { MouseEvent } from "react";

import { ApiError } from "../../shared/api";
import { markEmailUploaded } from "../emails/api";
import { robotsBehind } from "../emails/reviewStatus";
import type { EmailListItem } from "../emails/types";
import type { UploadSuggestion } from "./api";
import { suggestionsKey } from "./uploadSuggestions";

// MarkSeenUploadedButton confirms an upload the portal saw. The portal does
// not tell robots apart, so when several robots lag behind it asks which
// one sends the version.
export function MarkSeenUploadedButton({ email, suggestion }: { email: EmailListItem; suggestion: UploadSuggestion }) {
  const queryClient = useQueryClient();
  const behind = robotsBehind(email).map((robot) => robot.robot);
  const mutation = useMutation({
    mutationFn: (robots: string[]) => markEmailUploaded(email.id, true, suggestion.version, robots),
    onSuccess: () => {
      notifications.show({ color: "green", message: `Version ${suggestion.version} is marked uploaded.`, title: "Uploaded" });
    },
    onError: (error) => {
      // The server's version conflict message speaks of copying; here the
      // email changed after the portal saw the suggested version.
      const changed = error instanceof ApiError && error.status === 409 && /changed/.test(error.message);
      notifications.show({
        color: "red",
        message: changed ? "The email changed after the portal saw this version. Check it again." : error.message,
        title: "Not marked",
      });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      void queryClient.invalidateQueries({ queryKey: suggestionsKey });
      void queryClient.invalidateQueries({ queryKey: ["email-activity", email.id] });
    },
  });
  // The button often sits in a clickable row, and React events bubble out
  // of the menu's portal too.
  const mark = (event: MouseEvent, robots: string[]) => {
    event.stopPropagation();
    mutation.mutate(robots);
  };

  if (behind.length <= 1) {
    return (
      <Button component="span" loading={mutation.isPending} size="compact-xs" onClick={(event) => mark(event, behind)}>
        Mark v{suggestion.version} uploaded
      </Button>
    );
  }
  return (
    <Menu position="bottom-start" withinPortal>
      <Menu.Target>
        <Button component="span" loading={mutation.isPending} size="compact-xs" onClick={(event) => event.stopPropagation()}>
          Mark v{suggestion.version} uploaded...
        </Button>
      </Menu.Target>
      <Menu.Dropdown onClick={(event) => event.stopPropagation()}>
        <Menu.Label>Whose robot sends it?</Menu.Label>
        {behind.map((robot) => (
          <Menu.Item key={robot} onClick={(event) => mark(event, [robot])}>
            {robot}
          </Menu.Item>
        ))}
        <Menu.Item onClick={(event) => mark(event, behind)}>All of them</Menu.Item>
      </Menu.Dropdown>
    </Menu>
  );
}
