import { useState } from "react";
import {
  Alert,
  Button,
  Paper,
  Stack,
  Text,
  TextInput,
  Title,
} from "@mantine/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { ApiError } from "../../../shared/api";
import { demoLogin, devLogin, requestLoginCode, verifyLoginCode } from "../api";
import styles from "./Login.module.css";

const allowedDomains = getAllowedDomains();
const allowedDomainsLabel = allowedDomains.map((domain) => `@${domain}`).join(", ");
const storedEmailKey = "reviewdesk_login_email";
const devLoginEnabled = import.meta.env.VITE_AUTH_DEV_LOGIN_ENABLED === "true";
const demoLoginEnabled = import.meta.env.VITE_AUTH_DEMO_LOGIN_ENABLED === "true";

export function Login() {
  const queryClient = useQueryClient();
  const [email, setEmail] = useState(readStoredEmail);
  const [otpCode, setOTPCode] = useState("");
  const [domainError, setDomainError] = useState<string | null>(null);

  const requestCodeMutation = useMutation({
    mutationFn: requestLoginCode,
  });
  const verifyCodeMutation = useMutation({
    mutationFn: ({ email, code }: { email: string; code: string }) =>
      verifyLoginCode(email, code),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
  });
  const devLoginMutation = useMutation({
    mutationFn: devLogin,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
  });
  const demoLoginMutation = useMutation({
    mutationFn: demoLogin,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
  });

  return (
    <main className={styles.page}>
      <Paper className={styles.panel} p="xl" radius="md" shadow="xl">
        <form
          onSubmit={(event) => {
            event.preventDefault();

            const normalizedEmail = email.trim().toLowerCase();
            if (!isAllowedEmailDomain(normalizedEmail)) {
              setDomainError(
                `Access is only available for ${allowedDomainsLabel} emails.`
              );
              return;
            }

            setDomainError(null);
            requestCodeMutation.mutate(normalizedEmail);
          }}
        >
          <Stack gap="md">
            <Stack gap={4}>
              <Title order={2}>ReviewDesk</Title>
              <Text c="dimmed" size="sm">
                {demoLoginEnabled
                  ? "Public demo workspace for reviewing HTML email journeys."
                  : "Private workspace for authorized email review teams."}
              </Text>
              <Text c="dimmed" size="sm">
                Operated by Mikhail Rubashkin for collaborative email review.
              </Text>
              <Text c="dimmed" size="sm">
                This tool never asks for your email password.
              </Text>
            </Stack>

            <TextInput
              autoComplete="email"
              label="Email"
              placeholder={`name@${allowedDomains[0] ?? "example.com"}`}
              required
              type="email"
              value={email}
              onChange={(event) => {
                const nextEmail = event.currentTarget.value;
                setEmail(nextEmail);
                writeStoredEmail(nextEmail);
                setDomainError(null);
                requestCodeMutation.reset();
                verifyCodeMutation.reset();
              }}
            />

            {domainError ? (
              <Alert color="yellow" title="Email domain is not allowed">
                {domainError}
              </Alert>
            ) : null}

            {devLoginEnabled ? (
              <Alert color="blue" title="Local development login">
                Email delivery is bypassed for this local environment.
              </Alert>
            ) : null}

            {demoLoginEnabled ? (
              <Alert color="teal" title="Public demo">
                Enter with a demo reviewer account. Demo data is disposable.
              </Alert>
            ) : null}

            {demoLoginEnabled ? (
              <Button
                loading={demoLoginMutation.isPending}
                type="button"
                fullWidth
                onClick={() => {
                  demoLoginMutation.mutate();
                }}
              >
                Enter demo
              </Button>
            ) : null}

            {devLoginEnabled ? (
              <Button
                loading={devLoginMutation.isPending}
                type="button"
                variant="light"
                fullWidth
                onClick={() => {
                  const normalizedEmail = email.trim().toLowerCase();
                  if (!isAllowedEmailDomain(normalizedEmail)) {
                    setDomainError(
                      `Access is only available for ${allowedDomainsLabel} emails.`
                    );
                    return;
                  }

                  setDomainError(null);
                  devLoginMutation.mutate(normalizedEmail);
                }}
              >
                Continue locally
              </Button>
            ) : null}

            <Button
              loading={requestCodeMutation.isPending}
              type="submit"
              fullWidth
            >
              Send one-time code
            </Button>

            <Stack gap="xs">
              <TextInput
                autoComplete="one-time-code"
                inputMode="numeric"
                label="One-time code"
                maxLength={6}
                placeholder="123456"
                value={otpCode}
                onChange={(event) => {
                  setOTPCode(event.currentTarget.value.replace(/\D/g, "").slice(0, 6));
                  verifyCodeMutation.reset();
                }}
              />
              <Text c="dimmed" size="xs">
                Enter the 6-digit code sent to your email. Do not enter your email
                password.
              </Text>
              <Button
                disabled={otpCode.trim().length !== 6}
                loading={verifyCodeMutation.isPending}
                variant="light"
                onClick={() => {
                  const normalizedEmail = email.trim().toLowerCase();
                  if (!isAllowedEmailDomain(normalizedEmail)) {
                    setDomainError(
                      `Access is only available for ${allowedDomainsLabel} emails.`
                    );
                    return;
                  }

                  setDomainError(null);
                  verifyCodeMutation.mutate({
                    email: normalizedEmail,
                    code: otpCode,
                  });
                }}
              >
                Sign in
              </Button>
            </Stack>

            {requestCodeMutation.isSuccess ? (
              <Alert color="green" title="Code sent">
                Check your email for the one-time code.
              </Alert>
            ) : null}

            {requestCodeMutation.isError ? (
              <Alert color="red" title="Could not send code">
                {requestCodeMutation.error instanceof ApiError &&
                requestCodeMutation.error.status === 429
                  ? "Too many code requests. Wait 10 minutes and try again."
                  : "Check that the API server is reachable."}
              </Alert>
            ) : null}

            {verifyCodeMutation.isError ? (
              <Alert color="red" title="Could not sign in">
                Check your email and one-time code.
              </Alert>
            ) : null}

            {devLoginMutation.isError ? (
              <Alert color="red" title="Could not sign in locally">
                Check that dev login is enabled on the API server.
              </Alert>
            ) : null}

            {demoLoginMutation.isError ? (
              <Alert color="red" title="Could not enter demo">
                Check that demo login is enabled on the API server.
              </Alert>
            ) : null}
          </Stack>
        </form>
      </Paper>
    </main>
  );
}

export default Login;

function isAllowedEmailDomain(email: string) {
  const [, domain] = email.split("@");
  return Boolean(domain) && allowedDomains.includes(domain);
}

function readStoredEmail() {
  if (typeof window === "undefined") {
    return "";
  }

  return window.localStorage.getItem(storedEmailKey) ?? "";
}

function writeStoredEmail(email: string) {
  window.localStorage.setItem(storedEmailKey, email);
}

function getAllowedDomains() {
  const value = String(
    import.meta.env.VITE_AUTH_ALLOWED_DOMAINS ??
      import.meta.env.VITE_AUTH_ALLOWED_DOMAIN ??
      "example.com"
  );

  return value
    .split(",")
    .map((domain) => domain.trim().toLowerCase().replace(/^@/, ""))
    .filter(Boolean);
}
