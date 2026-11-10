import { textLabel } from "../portal/emailSends";
import type { EmailListItem, SplitText } from "./types";

// The API looks this far back for the texts an email went out in.
export const splitWindowDays = 14;

// splitTexts are the texts an email goes out in side by side: each manager of
// a region has a robot of their own, and one still holding an older text
// sends it next to the current one. Empty unless there are several; the API
// has already left out a text that was replaced.
export function splitTexts(email: Pick<EmailListItem, "portal_split_texts">): SplitText[] {
  const texts = email.portal_split_texts ?? [];
  return texts.length > 1 ? texts : [];
}

// splitDetail is the tooltip of the "N texts" mark: the texts named by the
// day the portal first sent them, as on the Sends tab.
export function splitDetail(texts: SplitText[]): string {
  const named = texts.map((text) => `${textLabel(text)} (${text.sends} ${text.sends === 1 ? "send" : "sends"})`);
  return `Over the last ${splitWindowDays} days robots sent ${texts.length} different texts of this email: ${named.join(", ")}. One of its robots likely still has an older text.`;
}
