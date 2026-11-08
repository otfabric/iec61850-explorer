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

/** The discovered model, loaded level by level as the user opens it. */
export function DeviceTree(props: DeviceTreeProps) {
  const roots = props.childrenOf[''] ?? [];
  return (
    <nav aria-label="Device tree" className="min-h-0 overflow-auto py-2">
      <p className="ot-label px-3 pb-2">Device tree</p>
      <ul role="tree">
        {roots.map((node) => (
          <TreeItem key={node.ref} node={node} depth={0} {...props} />
        ))}
      </ul>
    </nav>
  );
}

function TreeItem({ node, depth, ...props }: DeviceTreeProps & { node: ModelNode; depth: number }) {
  const open = props.expanded[node.ref] === true;
  const kids = props.childrenOf[node.ref];
  const isSelected = props.selected?.ref === node.ref;
  return (
    <li role="treeitem" aria-expanded={node.children > 0 ? open : undefined} aria-selected={isSelected}>
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
      {open && kids && (
        <ul role="group">
          {kids.map((child) => (
            <TreeItem key={child.ref} node={child} depth={depth + 1} {...props} />
          ))}
        </ul>
      )}
    </li>
  );
}
