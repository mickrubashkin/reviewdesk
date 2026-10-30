import { Alert, Anchor, Button, Group, NavLink, Paper, Stack, Text, Title, Typography } from "@mantine/core";
import { HouseIcon } from "@phosphor-icons/react";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import Markdown, { type Components } from "react-markdown";
import { Link, useLocation, useParams } from "react-router-dom";
import rehypeSlug from "rehype-slug";
import remarkGfm from "remark-gfm";

import { demoMode, markWhatsNewSeen } from "../auth/api";
import type { AuthUser } from "../emails/types";
import { helpPages } from "./content";
import { helpHref, helpRole, isExternalHref } from "./help";
import { HelpSearchButton } from "./HelpSearchButton";
import styles from "./HelpPage.module.css";
import { hasUnreadNews, latestNews } from "./whatsNew";

const markdownComponents: Components = {
  a: ({ href = "", children }) => {
    if (isExternalHref(href)) {
      return (
        <Anchor href={href} rel="noreferrer" target="_blank">
          {children}
        </Anchor>
      );
    }
    return (
      <Anchor component={Link} to={helpHref(href)}>
        {children}
      </Anchor>
    );
  },
  // Wide tables scroll on their own instead of the page.
  table: ({ children }) => (
    <div className={styles.tableScroll}>
      <table>{children}</table>
    </div>
  ),
};

// HelpPage shows the user help (docs/help/en/) at /help and /help/:page,
// with the pages listed beside it. The index points the reader to the part
// written for their role.
export function HelpPage({ currentUser }: { currentUser: AuthUser }) {
  const { page = "" } = useParams();
  const { hash } = useLocation();
  const current = helpPages.find((candidate) => candidate.slug === page);
  const role = helpRole(currentUser);
  const queryClient = useQueryClient();
  const readsNews = page === "whats-new" && !demoMode && hasUnreadNews(currentUser);

  // Opening "What's new" takes the mark off the account menu.
  useEffect(() => {
    if (!readsNews) {
      return;
    }
    markWhatsNewSeen(latestNews)
      .then(({ whats_new_seen }) =>
        queryClient.setQueryData<AuthUser>(["auth", "me"], (user) => (user ? { ...user, whats_new_seen } : user))
      )
      .catch(() => {
        // Not saved: the mark stays until the next visit, which is harmless.
      });
  }, [readsNews, queryClient]);

  // The router does not scroll to anchors: go to the heading, or to the top
  // of a newly opened page.
  useEffect(() => {
    const target = hash ? document.getElementById(decodeURIComponent(hash.slice(1))) : null;
    if (target) {
      target.scrollIntoView();
    } else {
      window.scrollTo(0, 0);
    }
  }, [page, hash]);

  return (
    <div className={styles.page}>
      <Stack className={styles.content} gap="lg">
        <Group justify="space-between" wrap="nowrap">
          <Title order={2}>Help</Title>
          <Group gap="xs" wrap="nowrap">
            <HelpSearchButton />
            <Button component={Link} leftSection={<HouseIcon aria-hidden="true" size={16} />} to="/" variant="light">
              Home
            </Button>
          </Group>
        </Group>

        <div className={styles.layout}>
          <nav aria-label="Help pages" className={styles.nav}>
            {helpPages.map((candidate) => (
              <NavLink
                active={candidate.slug === page}
                component={Link}
                key={candidate.slug}
                label={candidate.slug === "" ? "Start here" : candidate.title}
                to={candidate.slug === "" ? "/help" : `/help/${candidate.slug}`}
              />
            ))}
          </nav>

          <Paper className={styles.article} p="lg" radius="md" withBorder>
            {current ? (
              <Stack gap="md">
                {current.slug === "" ? (
                  <Alert color="blue" variant="light">
                    You are signed in as a <b>{role.label}</b>.{" "}
                    <Anchor component={Link} to={`/help#${role.anchor}`}>
                      Read your part
                    </Anchor>
                  </Alert>
                ) : null}
                <Typography>
                  <Markdown components={markdownComponents} rehypePlugins={[rehypeSlug]} remarkPlugins={[remarkGfm]}>
                    {current.markdown}
                  </Markdown>
                </Typography>
              </Stack>
            ) : (
              <Stack gap="xs">
                <Text>There is no such help page.</Text>
                <Anchor component={Link} to="/help">
                  Open the help
                </Anchor>
              </Stack>
            )}
          </Paper>
        </div>
      </Stack>
    </div>
  );
}
