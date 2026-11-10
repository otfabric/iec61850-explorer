import type { Quality, TimeValue, Value } from '../service/types';

export interface Summary {
  text: string;
  quality?: Quality;
  time?: TimeValue;
}

const PRIMARY = ['stVal', 'mag', 'f', 'i', 'cVal', 'instMag', 'setVal', 'general'];

/**
 * The value an engineer reads first: the attribute itself, or for a data
 * object its stVal or mag.f, with the quality and time next to it.
 */
export function summarize(value: Value): Summary {
  if (!value.children || value.children.length === 0) {
    return { text: value.text, quality: value.quality, time: value.time };
  }
  const byName = new Map(value.children.map((c) => [c.name ?? '', c]));
  const quality = byName.get('q')?.quality;
  const time = byName.get('t')?.time;
  for (const name of PRIMARY) {
    const child = byName.get(name);
    if (child) {
      const inner = summarize(child);
      return { text: inner.text, quality: quality ?? inner.quality, time: time ?? inner.time };
    }
  }
  return { text: value.text, quality, time };
}

/** The IEC 61850-7-2 names of a double point position (Dbpos). */
export const DBPOS_NAMES: Record<string, string> = { '00': 'intermediate-state', '01': 'off', '10': 'on', '11': 'bad-state' };

/**
 * A control's stVal as a word: a two-bit bit string is a Dbpos (the only
 * stVal of that type in IEC 61850-7-3) and is named; anything else is its
 * text.
 */
export function stateText(value?: Value): string {
  if (!value) {
    return '?';
  }
  if (value.type === 'bit-string' && value.bits === 2) {
    return DBPOS_NAMES[value.text] ?? value.text;
  }
  return value.text;
}
