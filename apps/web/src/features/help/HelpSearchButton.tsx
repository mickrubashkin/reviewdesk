import { Button, Highlight, Kbd, Text } from "@mantine/core";
import { Spotlight, spotlight } from "@mantine/spotlight";
import { MagnifyingGlassIcon } from "@phosphor-icons/react";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import { helpSections } from "./content";
import { helpSearchTerms, searchHelp } from "./helpSearch";
import styles from "./HelpPage.module.css";

const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);

// HelpSearchButton is the search of the help: a button that opens it, and Ctrl+K
// (⌘K on a Mac) while the help is open. Each result is a heading of a page;
// choosing it opens the page at that heading.
export function HelpSearchButton() {
  const [query, setQuery] = useState("");
  const navigate = useNavigate();
  const results = useMemo(() => searchHelp(helpSections, query), [query]);
  const terms = helpSearchTerms(query);

  return (
    <>
      <Button
        leftSection={<MagnifyingGlassIcon aria-hidden="true" size={16} />}
        onClick={spotlight.open}
        rightSection={<Kbd size="xs" visibleFrom="sm">{isMac ? "⌘K" : "Ctrl K"}</Kbd>}
        variant="default"
      >
        Search help
      </Button>

      <Spotlight.Root classNames={{ action: styles.searchAction }} onQueryChange={setQuery} query={query} scrollable>
        <Spotlight.Search
          aria-label="Search help"
          leftSection={<MagnifyingGlassIcon aria-hidden="true" size={18} />}
          placeholder="Search help…"
        />
        <Spotlight.ActionsList>
          {results.map(({ section, href, snippet }) => (
            <Spotlight.Action key={href} onClick={() => navigate(href)}>
              <div>
                <Text c="dimmed" size="xs">
                  {section.anchor ? section.pageTitle : "Page"}
                </Text>
                <Highlight fw={500} highlight={terms} size="sm">
                  {section.heading}
                </Highlight>
                {snippet ? (
                  <Highlight c="dimmed" highlight={terms} size="xs">
                    {snippet}
                  </Highlight>
                ) : null}
              </div>
            </Spotlight.Action>
          ))}
          {terms.length > 0 && results.length === 0 ? <Spotlight.Empty>Nothing found</Spotlight.Empty> : null}
        </Spotlight.ActionsList>
      </Spotlight.Root>
    </>
  );
}
