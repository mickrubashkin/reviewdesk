import { Text } from "@mantine/core";

import { useUserName } from "../../shared/userNames";
import { splitMentions } from "./mentions";

// MessageBody shows a comment message with its @mentions highlighted.
export function MessageBody({ body, mentionEmails }: { body: string; mentionEmails?: string[] }) {
  const userName = useUserName();
  const labels = (mentionEmails ?? []).map((email) => userName(email));
  return (
    <Text size="sm">
      {splitMentions(body, labels).map((part, index) =>
        part.mention ? (
          <Text c="blue" component="span" fw={600} inherit key={index}>
            {part.text}
          </Text>
        ) : (
          part.text
        )
      )}
    </Text>
  );
}
