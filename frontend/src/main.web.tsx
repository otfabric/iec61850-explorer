// Web entry, served by iec61850-explorerd. Nothing reachable from here may
// import @wailsio/runtime: the daemon's bundle must stay free of it.

import { HttpService } from './adapters/web/http';
import { WebPlatform } from './adapters/web/platform';
import { takeToken } from './adapters/web/token';
import { mount } from './app/mount';
import type { Service } from './app/service/service';

async function createService(platform: WebPlatform): Promise<Service> {
  // Development only: `?mock` runs against an in-memory IED.
  if (import.meta.env.DEV && new URLSearchParams(location.search).has('mock')) {
    const { MockService } = await import('./app/service/mock');
    return new MockService();
  }
  return new HttpService(takeToken(window), platform);
}

const platform = new WebPlatform();
void createService(platform).then((service) => mount(service, platform));
