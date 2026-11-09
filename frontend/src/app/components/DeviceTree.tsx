import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { ChevronIcon } from '../icons';
import type { ModelNode } from '../service/types';

interface DeviceTreeProps {
  childrenOf: Record<string, ModelNode[]>;
  expanded: Record<string, boolean>;
  selected: ModelNode | null;
  onToggle: (node: ModelNode, open: boolean) => void;
  onSelect: (node: ModelNode) => void;
}

const KIND_LABEL: Record<ModelNode['kind'], string> = { ld: 'LD', ln: 'LN', do: 'DO', child: '' };

/** Height of one row in pixels (Tailwind h-7); every row has it. */
export const ROW_HEIGHT = 28;
/** Rows rendered above and below the visible window. */
const OVERSCAN = 12;

/** One visible row of the flattened tree. */
export interface TreeRow {
  node: ModelNode;
  depth: number;
  /** Position among its siblings, 1-based, and the sibling count (ARIA). */
  pos: number;
  size: number;
}

/** flattenTree lists the rows that are visible: the roots and the loaded
 * children of every expanded node, depth first. */
export function flattenTree(childrenOf: Record<string, ModelNode[]>, expanded: Record<string, boolean>): TreeRow[] {
  const rows: TreeRow[] = [];
  const walk = (parent: string, depth: number) => {
    const kids = childrenOf[parent];
    if (!kids) return;
    kids.forEach((node, i) => {
      rows.push({ node, depth, pos: i + 1, size: kids.length });
      if (expanded[node.ref] === true) walk(node.ref, depth + 1);
    });
  };
  walk('', 0);
  return rows;
}

/**
 * The discovered model, loaded level by level as the user opens it.
 *
 * The tree is virtualized: it is flattened into the visible rows and only
 * the rows in the scrolled window (plus an overscan) are in the DOM, so a
 * model with tens of thousands of expanded nodes stays responsive. Rows
 * carry aria-level, aria-setsize and aria-posinset, the flat form of the
 * ARIA tree pattern.
 */
export function DeviceTree(props: DeviceTreeProps) {
  const rows = useMemo(() => flattenTree(props.childrenOf, props.expanded), [props.childrenOf, props.expanded]);
  const scroller = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [height, setHeight] = useState(0);

  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const measure = () => setHeight(el.clientHeight);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Keep the selected row in view when it is picked elsewhere (search).
  const selectedRef = props.selected?.ref;
  useEffect(() => {
    const el = scroller.current;
    if (!el || !selectedRef) return;
    const i = rows.findIndex((r) => r.node.ref === selectedRef);
    if (i < 0) return;
    const top = i * ROW_HEIGHT;
    if (top < el.scrollTop) el.scrollTop = top;
    else if (top + ROW_HEIGHT > el.scrollTop + el.clientHeight) el.scrollTop = top + ROW_HEIGHT - el.clientHeight;
    // Only on a new selection, not on every expansion: rows is read, not a dependency.
  }, [selectedRef]);

  const first = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const last = Math.min(rows.length, Math.ceil((scrollTop + height) / ROW_HEIGHT) + OVERSCAN);
  const visible = rows.slice(first, last);

  return (
    <nav aria-label="Device tree" className="flex min-h-0 flex-1 flex-col pt-2">
      <p className="ot-label px-3 pb-2">Device tree</p>
      <div
        ref={scroller}
        className="min-h-0 flex-1 overflow-auto pb-2"
        onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
      >
        <ul role="tree" className="relative" style={{ height: rows.length * ROW_HEIGHT }}>
          {visible.map((row, i) => (
            <TreeItem key={row.node.ref} row={row} top={(first + i) * ROW_HEIGHT} {...props} />
          ))}
        </ul>
      </div>
    </nav>
  );
}

function TreeItem({ row, top, ...props }: DeviceTreeProps & { row: TreeRow; top: number }) {
  const { node, depth } = row;
  const open = props.expanded[node.ref] === true;
  const isSelected = props.selected?.ref === node.ref;
  return (
    <li
      role="treeitem"
      aria-level={depth + 1}
      aria-setsize={row.size}
      aria-posinset={row.pos}
      aria-expanded={node.children > 0 ? open : undefined}
      aria-selected={isSelected}
      className="absolute inset-x-0"
      style={{ top, height: ROW_HEIGHT }}
    >
      <div
        className={`flex h-7 cursor-pointer items-center gap-1 pr-2 font-mono text-[13px] ${isSelected ? 'bg-accent-soft text-accent-strong' : 'hover:text-accent'}`}
        style={{ paddingLeft: 8 + depth * 14 }}
        onClick={() => props.onSelect(node)}
      >
        {node.children > 0 ? (
          <button
            type="button"
            aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`}
            className="inline-flex size-5 cursor-pointer items-center justify-center"
            onClick={(event) => {
              event.stopPropagation();
              props.onToggle(node, !open);
            }}
          >
            <ChevronIcon size={14} className={open ? 'rotate-90' : ''} />
          </button>
        ) : (
          <span className="inline-block size-5" />
        )}
        <span className="truncate">{node.name}</span>
        {KIND_LABEL[node.kind] && <span className="ml-auto pl-2 text-[10px] text-muted">{KIND_LABEL[node.kind]}</span>}
      </div>
    </li>
  );
}
