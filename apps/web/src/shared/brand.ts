// The team's real sender and sign-in domain stay out of the public demo:
// Vite replaces the flag at build time, so the demo bundle carries only the
// demo values.
const demo = import.meta.env.VITE_AUTH_DEMO_LOGIN_ENABLED === "true";

// previewSender is who an email appears to come from in the preview.
export const previewSender = demo
  ? { name: "Acme Cloud", initial: "A", replyTo: "hello@acme-cloud.example" }
  : { name: "Acme Cloud", initial: "A", replyTo: "hello@acme-cloud.example" };

// defaultLoginDomain is the sign-in domain when the build names none.
export const defaultLoginDomain = demo ? "example.com" : "example.com";
