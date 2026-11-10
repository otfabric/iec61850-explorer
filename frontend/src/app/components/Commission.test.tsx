import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { AuditRecord, ControlResult } from '../service/types';
import { AuditPanel, ControlOutcome } from './Commission';
import { stateText } from './summary';

describe('Dbpos', () => {
  it('names the four positions and leaves other values alone', () => {
    expect(stateText({ type: 'bit-string', bits: 2, text: '00' })).toBe('intermediate-state');
    expect(stateText({ type: 'bit-string', bits: 2, text: '01' })).toBe('off');
    expect(stateText({ type: 'bit-string', bits: 2, text: '10' })).toBe('on');
    expect(stateText({ type: 'bit-string', bits: 2, text: '11' })).toBe('bad-state');
    expect(stateText({ type: 'bit-string', bits: 3, text: '101' })).toBe('101');
    expect(stateText({ type: 'integer', text: '10' })).toBe('10');
    expect(stateText(undefined)).toBe('?');
  });

  it('shows a breaker position by name in the control outcome, with the bits in the title', () => {
    const result: ControlResult = {
      ref: 'AA1D1Q01A1CTRL/CSWI1.Pos',
      at: '2026-11-10T10:00:00Z',
      duration_ms: 120,
      ctl_model: 'sbo-with-enhanced-security',
      ctl_num: 1,
      steps: [{ step: 'operate', ok: true, duration_ms: 3 }],
      outcome: 'ok',
      before: { name: 'stVal', type: 'bit-string', bits: 2, text: '10' },
      after: { name: 'stVal', type: 'bit-string', bits: 2, text: '01' },
    };
    render(<ControlOutcome result={result} />);
    const line = screen.getByText(/^stVal/);
    expect(line).toHaveTextContent('stVal on → off');
    expect(line).not.toHaveTextContent('10');
    expect(line).toHaveAttribute('title', 'stVal 10 → 01');
  });

  it('shows the position before and after a control in the audit trail', () => {
    const record: AuditRecord = {
      seq: 1,
      at: '2026-11-10T10:00:00Z',
      session_id: 's1',
      target: '127.0.0.1:102',
      action: 'control',
      outcome: 'ok',
      ref: 'AA1D1Q01A1CTRL/CSWI1.Pos',
      before: 'on (10)',
      written: 'false',
      after: 'off (01)',
    };
    render(<AuditPanel records={[record]} label={() => 'AA1D1Q01A1'} />);
    expect(screen.getByText('stVal on (10) → off (01)')).toBeInTheDocument();
  });
});
