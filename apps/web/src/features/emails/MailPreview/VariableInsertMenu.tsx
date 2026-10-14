import { ActionIcon, Menu, Text, Tooltip } from "@mantine/core";
import { BracketsCurlyIcon } from "@phosphor-icons/react";

import type { BitrixVariable } from "../../bitrix-variables/api";

// VariableInsertMenu offers the board's Bitrix24 variables for a field: a
// link is replaced by the variable, text gets it appended.
export function VariableInsertMenu({
  disabled,
  variables,
  onInsert,
}: {
  disabled: boolean;
  variables: BitrixVariable[];
  onInsert: (expression: string) => void;
}) {
  return (
    <Menu position="bottom-end" shadow="md" width={260} withinPortal={false}>
      <Menu.Target>
        <Tooltip label="Insert Bitrix variable" withinPortal={false}>
          <ActionIcon aria-label="Insert Bitrix variable" disabled={disabled} size="sm" variant="subtle">
            <BracketsCurlyIcon aria-hidden="true" size={14} />
          </ActionIcon>
        </Tooltip>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>Filled in by Bitrix24 on send</Menu.Label>
        {variables.map((variable) => (
          <Menu.Item key={variable.expression} onClick={() => onInsert(variable.expression)}>
            <Text size="sm">{variable.label}</Text>
            {variable.note ? (
              <Text c="dimmed" size="xs">
                {variable.note}
              </Text>
            ) : null}
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  );
}
