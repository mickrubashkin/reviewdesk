import { Badge, Tooltip } from "@mantine/core";

import { uploadTodo } from "./reviewStatus";
import type { EmailListItem } from "./types";

// UploadBadge flags ready emails that need a Bitrix24 upload: never
// uploaded, or a robot has an older version than the current one.
export function UploadBadge({ email }: { email: EmailListItem }) {
  const todo = uploadTodo(email);
  if (!todo) {
    return null;
  }
  return (
    <Tooltip label={todo.detail} multiline maw={320}>
      <Badge color={todo.color} size="xs" style={{ flexShrink: 0 }} variant="light">
        {todo.label.charAt(0).toUpperCase() + todo.label.slice(1)}
      </Badge>
    </Tooltip>
  );
}
