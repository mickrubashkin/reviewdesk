import { Alert, Anchor, Text, type MantineSpacing } from "@mantine/core";
import { Link } from "react-router-dom";

import { masterInReview } from "./masterInReview";
import type { EmailListItem } from "./types";

// MasterInReviewNote tells a translator that EN is still being reviewed, so
// its text may change again: better to translate once it settles.
export function MasterInReviewNote({
  email,
  emails,
  mb,
}: {
  email: { translation_of: string | null };
  emails: EmailListItem[];
  mb?: MantineSpacing;
}) {
  const master = masterInReview(email, emails);
  if (!master) {
    return null;
  }
  const comments = master.open_comment_count === 1 ? "1 open comment" : `${master.open_comment_count} open comments`;
  return (
    <Alert color="gray" mb={mb} py="xs" title={`EN is still a draft · ${comments}`} variant="light">
      <Text size="xs">
        Its text may change again, and this translation with it. Translate once EN is settled.{" "}
        <Anchor component={Link} size="xs" to={`/emails/${encodeURIComponent(master.id)}/review`}>
          Open EN
        </Anchor>
      </Text>
    </Alert>
  );
}
