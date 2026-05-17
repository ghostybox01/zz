/**
 * TableToolbar — shared row-management strip for Fleet / Hits / Lists panels.
 *
 * Provides:
 *   - case-insensitive filter input (controlled by parent)
 *   - "Select all" / "Clear" / optional "Select dead" buttons
 *   - "Delete selected (N)" — only rendered when selectedCount > 0,
 *     and guarded by window.confirm before invoking the callback
 *
 * Subtle visual styling so it slots inside an existing card-block__head
 * region without re-doing layout.
 */

type Props = {
  totalRows: number
  selectedCount: number
  filter: string
  onFilterChange: (v: string) => void
  onSelectAll: () => void
  onClearSelection: () => void
  /** Fleet-only: select all 'offline' / 'removed' nodes in one click. */
  onSelectDead?: () => void
  onDeleteSelected: () => void
  /** Optional placeholder override for the filter input. */
  filterPlaceholder?: string
}

export function TableToolbar({
  totalRows,
  selectedCount,
  filter,
  onFilterChange,
  onSelectAll,
  onClearSelection,
  onSelectDead,
  onDeleteSelected,
  filterPlaceholder,
}: Props) {
  const handleDelete = () => {
    if (selectedCount === 0) return
    const ok = window.confirm(
      `Delete ${selectedCount} selected row${selectedCount === 1 ? '' : 's'}? This cannot be undone.`,
    )
    if (ok) onDeleteSelected()
  }

  return (
    <div className="table-toolbar" role="toolbar" aria-label="Row management">
      <input
        type="search"
        className="table-toolbar__filter"
        placeholder={filterPlaceholder ?? 'Filter rows…'}
        value={filter}
        onChange={(e) => onFilterChange(e.target.value)}
        aria-label="Filter rows"
      />
      <span className="table-toolbar__count muted" aria-live="polite">
        {selectedCount > 0
          ? `${selectedCount} selected · ${totalRows} visible`
          : `${totalRows} visible`}
      </span>
      <div className="table-toolbar__actions">
        <button
          type="button"
          className="btn-glass btn-glass--xs"
          onClick={onSelectAll}
          disabled={totalRows === 0}
        >
          Select all
        </button>
        <button
          type="button"
          className="btn-glass btn-glass--xs"
          onClick={onClearSelection}
          disabled={selectedCount === 0}
        >
          Clear
        </button>
        {onSelectDead && (
          <button
            type="button"
            className="btn-glass btn-glass--xs"
            onClick={onSelectDead}
            title="Select offline + pruned nodes"
          >
            Select dead
          </button>
        )}
        {selectedCount > 0 && (
          <button
            type="button"
            className="btn-danger-outline btn-glass--xs"
            onClick={handleDelete}
          >
            Delete selected ({selectedCount})
          </button>
        )}
      </div>
    </div>
  )
}
