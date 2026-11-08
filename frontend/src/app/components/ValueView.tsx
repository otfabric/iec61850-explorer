import type { Value } from '../service/types';

const QUALITY_TONE: Record<string, string> = {
  good: 'text-accent',
  questionable: 'text-wire',
  invalid: 'text-danger',
  reserved: 'text-danger',
};

/** One value and its components, with quality and time decoded. */
export function ValueView({ value, depth = 0 }: { value: Value; depth?: number }) {
  const tone = value.quality ? QUALITY_TONE[value.quality.validity] : value.type === 'data-access-error' ? 'text-danger' : 'text-fg-strong';
  return (
    <>
      <tr className="border-b border-line">
        <td className="py-1 pr-3 font-mono text-[13px]" style={{ paddingLeft: depth * 16 }}>
          {value.name || (depth === 0 ? 'value' : '·')}
        </td>
        <td className={`py-1 pr-3 font-mono text-[13px] ${tone}`}>
          {value.type === 'structure' || value.type === 'array' ? <span className="text-muted">{value.text}</span> : value.text}
          {value.time && (value.time.clock_failure || value.time.clock_not_synchronized) && (
            <span className="ml-2 text-wire">{value.time.clock_failure ? 'clock failure' : 'clock not synchronized'}</span>
          )}
        </td>
        <td className="py-1 text-[12px] text-muted">
          {value.type}
          {value.bits ? ` (${value.bits})` : ''}
        </td>
      </tr>
      {value.children?.map((child, i) => (
        <ValueView key={`${child.name ?? ''}${i}`} value={child} depth={depth + 1} />
      ))}
    </>
  );
}
