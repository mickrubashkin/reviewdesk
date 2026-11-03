# Translations

EN is the source of every email. A translation is linked to the EN email it came from, so ReviewDesk knows when it falls behind.

## Create a translation

1. Open the board in the **Matrix** view. An empty cell is a language the slot does not have yet.
2. Click the translate icon (**Create from EN**) in the empty cell. It is there for the languages AI translates into (DE, ES, PL, BR) when the slot has an EN email and you can edit that language.
3. Choose:
   - **Translate and create** — AI translates the subject, the preheader and every text field into a new draft. Terms from the translation glossary are kept; fields where Bitrix24 expressions would break stay in English.
   - **Create with EN text** — a draft with the English text, to translate by hand.
4. Check the draft, [edit](editing.md) what needs it, and leave a comment if something is unclear.

## Update a translation after EN changed

A translation whose EN email changed shows **EN changed**. Your list is in **My tasks → Translations to update**.

1. Take the email in **My tasks**, so others who translate the same language know you are on it.
2. Open it. **Details** lists the fields that changed in EN, with the added and removed words marked.
3. Update only those fields:
   - by hand, then **Mark up to date**, or
   - **Suggest translation**: AI drafts the changed fields next to the current text; edit them and click **Apply and mark up to date**.

## Link a translation by hand

If a translation is not linked to its EN email (ReviewDesk could not pair them), pick the EN email in **Details → Translation of**.

## Translation glossary

The translation glossary tells the AI how to handle key terms: keep them in English (product names, CoPilot) or translate them exactly as given (Deals → Aufträge). It applies to **Translate and create** and **Suggest translation**. Open it from the account menu → **Translation glossary**.

### Suggest a term

- In an email, select the word and click the glossary icon next to **Comment**. In an EN email the selection becomes the English term; in a translation it becomes the wording for that language, and you add the English term.
- Or open **Translation glossary** and click **Suggest a term**.

A CRM operator adds it or rejects it. **Your suggestions** on the glossary page shows which.

### Decide on suggestions (CRM operators)

**Settings → Content → Translation glossary** lists the suggestions above the terms. Edit a suggestion if needed and click **Accept**: a new term is added, or the translation is set on a term already in the glossary. **Reject** drops it.
