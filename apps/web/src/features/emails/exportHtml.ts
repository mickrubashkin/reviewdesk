import { notifications } from "@mantine/notifications";

import type { EmailDetail } from "./types";

export async function copyRenderedHTML(html: string) {
  return copyHTML(html, {
    failureMessage: "Browser blocked clipboard access. Try downloading the HTML instead.",
    successMessage: "Rendered HTML copied to clipboard",
  });
}

export function downloadRenderedHTML(email: EmailDetail, html: string) {
  downloadHTML(html, buildHTMLFileName(email, "rendered"), "Rendered HTML downloaded");
}

async function copyHTML(
  html: string,
  messages: {
    failureMessage: string;
    successMessage: string;
  }
) {
  try {
    await navigator.clipboard.writeText(html);
    notifications.show({
      color: "green",
      message: messages.successMessage,
      title: "Copied",
    });
  } catch {
    notifications.show({
      color: "red",
      message: messages.failureMessage,
      title: "Copy failed",
    });
  }
}

function downloadHTML(html: string, fileName: string, title: string) {
  const blob = new Blob([html], {
    type: "text/html;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");

  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);

  notifications.show({
    color: "green",
    message: `${link.download} is ready`,
    title,
  });
}

function buildHTMLFileName(email: EmailDetail, suffix?: string) {
  const baseName = email.slug || email.title || "email";
  const sanitizedBaseName = baseName
    .toLowerCase()
    .trim()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9_-]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
  const name = sanitizedBaseName || "email";

  return `${suffix ? `${name}-${suffix}` : name}.html`;
}
