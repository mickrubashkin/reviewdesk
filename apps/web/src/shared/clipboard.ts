// copyText puts text on the clipboard, or throws when the browser refuses.
//
// Inside the Bitrix24 portal the app runs in an iframe without the
// clipboard-write permission, so navigator.clipboard.writeText rejects there.
// The legacy execCommand("copy") is not gated by that permission and works
// during a click, so it goes first; the async API is the fallback for
// browsers that have dropped execCommand.
export async function copyText(value: string): Promise<void> {
  if (copyWithSelection(value)) {
    return;
  }
  if (!navigator.clipboard) {
    throw new Error("Clipboard is not available.");
  }
  await navigator.clipboard.writeText(value);
}

function copyWithSelection(value: string): boolean {
  const active = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  // Inside a modal, keep the textarea within the dialog so its focus trap
  // does not pull focus back before the copy runs.
  const host = active?.closest<HTMLElement>("[role='dialog']") ?? document.body;
  const textarea = document.createElement("textarea");

  textarea.value = value;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.top = "0";
  textarea.style.left = "0";
  textarea.style.width = "1px";
  textarea.style.height = "1px";
  textarea.style.opacity = "0";
  host.appendChild(textarea);

  let copied: boolean;
  try {
    textarea.focus({ preventScroll: true });
    textarea.select();
    copied = document.execCommand("copy");
  } catch {
    copied = false;
  } finally {
    textarea.remove();
    active?.focus({ preventScroll: true });
  }
  return copied;
}
