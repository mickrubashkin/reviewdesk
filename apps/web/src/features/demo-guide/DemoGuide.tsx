import { Anchor, Button, CloseButton, Group, Paper, Stack, Text, ThemeIcon, UnstyledButton } from "@mantine/core";
import { CaretDownIcon, CheckIcon, CompassIcon } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { Link, useLocation } from "react-router-dom";

import { fetchEmails } from "../emails/api";
import type { AuthUser } from "../emails/types";
import { findHeroEmail, guideSteps, type GuideStepKey } from "./demoGuideSteps";
import styles from "./DemoGuide.module.css";

const stepText: Record<GuideStepKey, ReactNode> = {
  open: (
    <>
      Open <b>Complete Setup</b>: a reviewer left a blocking comment on it.
    </>
  ),
  edit: (
    <>
      Fix the paragraph the comment is about: click it in the email, then the pencil.
    </>
  ),
  resolve: <>Resolve the blocking comment in the Comments tab.</>,
  ready: (
    <>
      Mark the email <b>Ready</b> in the status menu at the top.
    </>
  ),
};

// DemoGuide walks a demo visitor through the review loop on the scenario's
// hero email and ticks steps off as they happen. Shown only in the demo build.
export function DemoGuide({ currentUser }: { currentUser: AuthUser }) {
  const boardKey = currentUser.managed_boards?.[0] ?? "";
  const location = useLocation();
  // Folded on phones until the visitor opens it: there it would cover a
  // third of the screen.
  const [collapsed, setCollapsed] = useState(() =>
    readFlag("demo-guide:collapsed", window.matchMedia("(max-width: 40em)").matches)
  );
  // Per board: a visitor who comes back after the reset gets a fresh sandbox.
  const dismissedKey = `demo-guide:${boardKey}:dismissed`;
  const openedKey = `demo-guide:${boardKey}:opened`;
  const [dismissed, setDismissed] = useState(() => readFlag(dismissedKey));


  const emailsQuery = useQuery({
    queryKey: ["emails", boardKey],
    queryFn: () => fetchEmails(boardKey),
    enabled: boardKey !== "" && !dismissed,
  });
  const hero = findHeroEmail(emailsQuery.data ?? []);
  const heroPath = hero ? `/emails/${encodeURIComponent(hero.id)}/review` : "";
  const onHero = heroPath !== "" && location.pathname === heroPath;
  // On an email the preview is on the left, so the guide sits on the right.
  const side = location.pathname.startsWith("/emails/") ? "right" : "left";

  // Remembered so the step stays ticked after the visitor leaves the email.
  const opened = onHero || readFlag(openedKey);
  useEffect(() => {
    if (onHero) {
      writeFlag(openedKey, true);
    }
  }, [onHero, openedKey]);

  if (dismissed || !hero) {
    return null;
  }

  const steps = guideSteps(hero, opened);
  const doneCount = steps.filter((step) => step.done).length;
  const finished = doneCount === steps.length;
  const toggle = () => {
    setCollapsed(!collapsed);
    writeFlag("demo-guide:collapsed", !collapsed);
  };

  if (collapsed) {
    return (
      <UnstyledButton className={styles.pill} data-side={side} onClick={toggle}>
        <CompassIcon aria-hidden="true" size={16} />
        <span>Try the review loop</span>
        <span className={styles.pillCount}>
          {doneCount}/{steps.length}
        </span>
      </UnstyledButton>
    );
  }

  return (
    <Paper className={styles.guide} data-side={side} radius="md" shadow="lg" withBorder>
      <Group justify="space-between" wrap="nowrap" gap="xs">
        <UnstyledButton className={styles.title} onClick={toggle}>
          <CompassIcon aria-hidden="true" size={16} />
          <Text fw={700} size="sm">
            Try the review loop
          </Text>
          <Text c="dimmed" size="xs">
            {doneCount}/{steps.length}
          </Text>
          <CaretDownIcon aria-hidden="true" size={14} />
        </UnstyledButton>
        {finished ? (
          <CloseButton
            aria-label="Hide the guide"
            size="sm"
            onClick={() => {
              setDismissed(true);
              writeFlag(dismissedKey, true);
            }}
          />
        ) : null}
      </Group>

      <Stack component="ol" className={styles.steps} gap={8}>
        {steps.map((step, index) => (
          <li key={step.key} className={styles.step} data-done={step.done || undefined}>
            <ThemeIcon
              color={step.done ? "green" : "gray"}
              radius="xl"
              size={20}
              variant={step.done ? "filled" : "light"}
            >
              {step.done ? <CheckIcon aria-hidden="true" size={12} weight="bold" /> : <Text size="xs">{index + 1}</Text>}
            </ThemeIcon>
            <Text className={styles.stepText} size="sm">
              {stepText[step.key]}
            </Text>
          </li>
        ))}
      </Stack>

      {finished ? (
        <Text size="sm" mt="sm">
          That is the whole loop: the email is ready to paste into the CRM. The same board also
          comes as{" "}
          <Anchor component={Link} to={`/boards/${encodeURIComponent(boardKey)}?view=cards`}>
            cards
          </Anchor>{" "}
          and a{" "}
          <Anchor component={Link} to={`/boards/${encodeURIComponent(boardKey)}?view=matrix`}>
            language matrix
          </Anchor>
          , and an email has an AI review in its AI tab.
        </Text>
      ) : !onHero ? (
        <Button component={Link} to={heroPath} fullWidth mt="sm" size="xs">
          Open Complete Setup
        </Button>
      ) : null}
    </Paper>
  );
}

// The guide's state is a per-visitor convenience: without storage it simply
// starts over.
function readFlag(key: string, fallback = false): boolean {
  try {
    const value = window.localStorage.getItem(key);
    return value === null ? fallback : value === "true";
  } catch {
    return fallback;
  }
}

function writeFlag(key: string, value: boolean) {
  try {
    window.localStorage.setItem(key, String(value));
  } catch {
    // Storage is off: the flag lasts until the page reloads.
  }
}
