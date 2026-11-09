import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ModelNode } from '../service/types';
import { DeviceTree, flattenTree, ROW_HEIGHT } from './DeviceTree';

const node = (ref: string, kind: ModelNode['kind'], children: number): ModelNode => ({
  name: ref.split(/[/.]/).pop() ?? ref,
  ref,
  kind,
  fcs: [],
  children,
});

/** A model with one LD, n logical nodes, each with two data objects. */
function bigModel(n: number) {
  const ld = node('IED1LD0', 'ld', n);
  const lns = Array.from({ length: n }, (_, i) => node(`IED1LD0/GGIO${i + 1}`, 'ln', 2));
  const childrenOf: Record<string, ModelNode[]> = { '': [ld], [ld.ref]: lns };
  const expanded: Record<string, boolean> = { [ld.ref]: true };
  for (const ln of lns) {
    childrenOf[ln.ref] = [node(`${ln.ref}.Ind1`, 'do', 0), node(`${ln.ref}.Ind2`, 'do', 0)];
    expanded[ln.ref] = true;
  }
  return { childrenOf, expanded, lns };
}

describe('flattenTree', () => {
  it('lists the roots and the loaded children of expanded nodes, depth first', () => {
    const { childrenOf, expanded } = bigModel(2);
    expanded['IED1LD0/GGIO2'] = false;
    const rows = flattenTree(childrenOf, expanded);
    expect(rows.map((r) => [r.node.ref, r.depth, r.pos, r.size])).toEqual([
      ['IED1LD0', 0, 1, 1],
      ['IED1LD0/GGIO1', 1, 1, 2],
      ['IED1LD0/GGIO1.Ind1', 2, 1, 2],
      ['IED1LD0/GGIO1.Ind2', 2, 2, 2],
      ['IED1LD0/GGIO2', 1, 2, 2],
    ]);
  });

  it('skips an expanded node whose children are not loaded yet', () => {
    const ld = node('LD', 'ld', 3);
    expect(flattenTree({ '': [ld] }, { LD: true })).toHaveLength(1);
  });
});

describe('DeviceTree', () => {
  const props = (m: ReturnType<typeof bigModel>, selected: ModelNode | null = null) => ({
    childrenOf: m.childrenOf,
    expanded: m.expanded,
    selected,
    onToggle: vi.fn(),
    onSelect: vi.fn(),
  });

  it('renders only the rows in the scrolled window of a large tree', () => {
    const m = bigModel(5000); // 15,001 visible rows
    render(<DeviceTree {...props(m)} />);
    const items = screen.getAllByRole('treeitem');
    // setup.ts gives every element a 2400 px viewport: 86 rows plus overscan.
    expect(items.length).toBeLessThan(120);
    expect(screen.getByRole('tree')).toHaveStyle({ height: `${15001 * ROW_HEIGHT}px` });
    expect(items[0]).toHaveAttribute('aria-level', '1');
    expect(screen.queryByText('GGIO4000')).toBeNull();

    const scroller = screen.getByRole('tree').parentElement!;
    scroller.scrollTop = 3 * 3999 * ROW_HEIGHT; // GGIO4000 is row 1 + 3 * 3999
    fireEvent.scroll(scroller);
    const row = screen.getByText('GGIO4000').closest('[role="treeitem"]')!;
    expect(row).toHaveAttribute('aria-level', '2');
    expect(row).toHaveAttribute('aria-posinset', '4000');
    expect(row).toHaveAttribute('aria-setsize', '5000');
    expect(row).toHaveStyle({ top: `${(1 + 3 * 3999) * ROW_HEIGHT}px` });
  });

  it('scrolls a row selected elsewhere into view', () => {
    const m = bigModel(5000);
    const { rerender } = render(<DeviceTree {...props(m)} />);
    const scroller = screen.getByRole('tree').parentElement!;
    expect(scroller.scrollTop).toBe(0);
    const target = m.lns[2999];
    rerender(<DeviceTree {...props(m, target)} />);
    const index = 1 + 3 * 2999;
    expect(scroller.scrollTop).toBe(index * ROW_HEIGHT + ROW_HEIGHT - 2400);
    fireEvent.scroll(scroller);
    expect(screen.getByText('GGIO3000').closest('[role="treeitem"]')).toHaveAttribute('aria-selected', 'true');
  });

  it('toggles and selects through the rendered rows', () => {
    const m = bigModel(3);
    const p = props(m);
    render(<DeviceTree {...p} />);
    fireEvent.click(screen.getByRole('button', { name: 'Collapse GGIO2' }));
    expect(p.onToggle).toHaveBeenCalledWith(m.lns[1], false);
    expect(p.onSelect).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText('GGIO3'));
    expect(p.onSelect).toHaveBeenCalledWith(m.lns[2]);
  });
});
