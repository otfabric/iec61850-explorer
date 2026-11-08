import { expect, it } from 'vitest';
import type { Platform } from '../../app/platform/platform';
import { WebPlatform } from './platform';

it('is the browser mode', () => {
  const platform: Platform = new WebPlatform();
  expect(platform.mode).toEqual({ kind: 'browser' });
});
