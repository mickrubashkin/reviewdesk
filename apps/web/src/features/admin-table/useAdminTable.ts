import { useMemo } from "react";
import {
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { usePersistedColumnSizing } from "./usePersistedColumnSizing";
import { usePersistedSort } from "./usePersistedSort";

export function useAdminTable<TData, TSortKey extends string>({
  columns,
  data,
  defaultSort,
  sortKeys,
  storageKey,
}: {
  columns: ColumnDef<TData, unknown>[];
  data: TData[];
  defaultSort: { direction: "asc" | "desc"; key: TSortKey };
  sortKeys: readonly TSortKey[];
  storageKey: string;
}) {
  const [sort, setSort] = usePersistedSort(
    `reviewdesk:admin-table-sort:${storageKey}`,
    defaultSort,
    sortKeys
  );
  const [columnSizing, setColumnSizing] = usePersistedColumnSizing(
    `reviewdesk:admin-table-column-sizing:${storageKey}`
  );

  const sorting = useMemo<SortingState>(
    () => [{ desc: sort.direction === "desc", id: sort.key }],
    [sort]
  );

  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    columnResizeMode: "onChange",
    columns,
    data,
    enableColumnResizing: true,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    onColumnSizingChange: setColumnSizing,
    onSortingChange: (updater) => {
      const nextSorting =
        typeof updater === "function" ? updater(sorting) : updater;
      const nextSort = nextSorting[0];
      if (!nextSort || !sortKeys.includes(nextSort.id as TSortKey)) {
        setSort(defaultSort);
        return;
      }
      setSort({
        direction: nextSort.desc ? "desc" : "asc",
        key: nextSort.id as TSortKey,
      });
    },
    state: {
      columnSizing,
      sorting,
    },
  });

  return {
    table,
    rows: table.getRowModel().rows,
    sort,
    columnSizing,
  };
}
