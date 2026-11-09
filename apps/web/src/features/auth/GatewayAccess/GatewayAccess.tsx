import { Loader, Paper, Stack, Text, Title } from "@mantine/core";

import { gatewayAccessState } from "./gatewayAccessState";

// GatewayAccess stands in for the sign-in page on VibeCode, where the
// Bitrix24 account signs people in.
export function GatewayAccess({ error }: { error: unknown }) {
  const state = gatewayAccessState(error);
  return (
    <Stack align="center" justify="center" h="100dvh" p="md">
      <Paper withBorder p="xl" radius="md" maw={420}>
        <Stack align="center" gap="sm" ta="center">
          {state === "pending" ? (
            <>
              <Loader />
              <Title order={3}>Setting up your access</Title>
              <Text c="dimmed">The first visit takes a few seconds while ReviewDesk finds your Bitrix24 account.</Text>
            </>
          ) : state === "refused" ? (
            <>
              <Title order={3}>No access yet</Title>
              <Text c="dimmed">Your Bitrix24 account isn't in ReviewDesk. Ask an administrator to add you, then open ReviewDesk again.</Text>
            </>
          ) : (
            <>
              <Title order={3}>Open ReviewDesk from Bitrix24</Title>
              <Text c="dimmed">ReviewDesk signs you in with your Bitrix24 account. Open it from the Bitrix24 menu.</Text>
            </>
          )}
        </Stack>
      </Paper>
    </Stack>
  );
}
