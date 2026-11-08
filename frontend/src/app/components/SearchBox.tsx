import { useState, type FormEvent } from 'react';
import { SearchIcon } from '../icons';
import type { ModelNode, SearchQuery, SearchResult } from '../service/types';
import { TextInput } from './ui';

const FCS = ['', 'ST', 'MX', 'CF', 'DC', 'SP', 'SV', 'SG', 'SE', 'EX', 'CO', 'BL', 'OR'];

interface SearchBoxProps {
  search: { query: SearchQuery; result: SearchResult } | null;
  onSearch: (query: SearchQuery | null) => void;
  onPick: (node: ModelNode) => void;
  selected: ModelNode | null;
}

/** Searches the discovered model by reference text, LN class and FC. */
export function SearchBox({ search, onSearch, onPick, selected }: SearchBoxProps) {
  const [text, setText] = useState(search?.query.text ?? '');
  const [lnClass, setLnClass] = useState(search?.query.ln_class ?? '');
  const [fc, setFc] = useState(search?.query.fc ?? '');

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!text.trim() && !lnClass.trim() && !fc) {
      onSearch(null);
      return;
    }
    onSearch({ text: text.trim() || undefined, ln_class: lnClass.trim() || undefined, fc: fc || undefined });
  }

  return (
    <div className="flex flex-col border-b border-line">
      <form role="search" className="flex flex-wrap gap-1 p-2" onSubmit={submit}>
        <TextInput aria-label="Search references" placeholder="Search, e.g. Hz" value={text} onChange={(e) => setText(e.target.value)} className="min-w-0 flex-1" />
        <TextInput aria-label="LN class" placeholder="LN class" value={lnClass} onChange={(e) => setLnClass(e.target.value)} className="w-20" />
        <select aria-label="Functional constraint" value={fc} onChange={(e) => setFc(e.target.value)} className="h-8 rounded border border-control bg-bg px-1 text-[13px] text-fg-strong">
          {FCS.map((f) => (
            <option key={f} value={f}>
              {f || 'FC'}
            </option>
          ))}
        </select>
        <button type="submit" aria-label="Search" className="inline-flex size-8 cursor-pointer items-center justify-center rounded border border-control hover:text-accent">
          <SearchIcon size={16} />
        </button>
      </form>
      {search && (
        <div className="max-h-64 overflow-auto pb-2">
          <p className="ot-label flex items-center px-3 pb-1">
            {search.result.hits.length}
            {search.result.truncated ? '+' : ''} matches
            <button
              type="button"
              className="ml-auto cursor-pointer normal-case tracking-normal text-accent"
              onClick={() => {
                setText('');
                setLnClass('');
                setFc('');
                onSearch(null);
              }}
            >
              Clear
            </button>
          </p>
          <ul aria-label="Search results">
            {search.result.hits.map((n) => (
              <li key={n.ref}>
                <button
                  type="button"
                  className={`w-full cursor-pointer truncate px-3 py-0.5 text-left font-mono text-[12px] ${selected?.ref === n.ref ? 'bg-accent-soft text-accent-strong' : 'hover:text-accent'}`}
                  onClick={() => onPick(n)}
                >
                  {n.ref}
                  {n.fcs.length > 0 && <span className="text-muted"> [{n.fcs.join(',')}]</span>}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
