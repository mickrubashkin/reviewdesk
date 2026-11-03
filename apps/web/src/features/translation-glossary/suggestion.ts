import { glossaryLanguages, type GlossarySuggestionInput } from "./api";

// suggestionFromSelection turns text picked in an email into a draft: in an
// EN email it is the term, in a translation it is that language's wording.
export function suggestionFromSelection(text: string, language: string, emailId: string): GlossarySuggestionInput {
  const selected = text.trim();
  const translated = glossaryLanguages.some((item) => item.code === language);
  return {
    term: translated ? "" : selected,
    language: translated ? language : "",
    translation: translated ? selected : "",
    note: "",
    email_id: emailId,
  };
}
