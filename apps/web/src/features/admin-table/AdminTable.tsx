import { ScrollArea, Table } from "@mantine/core";
import { flexRender, type Table as ReactTable } from "@tanstack/react-table";
import styles from "./AdminTable.module.css";

function formatSortIndicator(isSorted: false | "asc" | "desc") {
  if (isSorted === "asc") {
    return " ↑";
  }
  if (isSorted === "desc") {
    return " ↓";
  }
  return "";
}

export function AdminTable<TData>({
  table,
}: {
  table: ReactTable<TData>;
}) {
  const rows = table.getRowModel().rows;

  return (
    <ScrollArea type="auto">
      <Table
        className={styles.table}
        highlightOnHover
        horizontalSpacing="md"
        style={{ width: table.getTotalSize() }}
        verticalSpacing="sm"
      >
        <Table.Thead>
          {table.getHeaderGroups().map((headerGroup) => (
            <Table.Tr key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <Table.Th
                  className={styles.resizableTh}
                  key={header.id}
                  style={{ width: header.getSize() }}
                >
                  {header.isPlaceholder ? null : (
                    <button
                      className={styles.sortButton}
                      type="button"
                      onClick={header.column.getToggleSortingHandler()}
                    >
                      {flexRender(
                        header.column.columnDef.header,
                        header.getContext()
                      )}
                      <span className={styles.sortIndicator}>
                        {formatSortIndicator(header.column.getIsSorted())}
                      </span>
                    </button>
                  )}
                  {header.column.getCanResize() ? (
                    <button
                      aria-label={`Resize ${header.column.columnDef.header} column`}
                      className={styles.resizeHandle}
                      type="button"
                      onDoubleClick={() => header.column.resetSize()}
                      onMouseDown={header.getResizeHandler()}
                      onTouchStart={header.getResizeHandler()}
                    />
                  ) : null}
                </Table.Th>
              ))}
            </Table.Tr>
          ))}
        </Table.Thead>
        <Table.Tbody>
          {rows.map((row) => (
            <Table.Tr key={row.id}>
              {row.getVisibleCells().map((cell) => (
                <Table.Td key={cell.id} style={{ width: cell.column.getSize() }}>
                  {flexRender(
                    cell.column.columnDef.cell,
                    cell.getContext()
                  )}
                </Table.Td>
              ))}
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </ScrollArea>
  );
}
