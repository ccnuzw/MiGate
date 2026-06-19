import { coreAPI } from './core';
import { inboundsAPI } from './inbounds';
import { outboundsAPI } from './outbounds';
import { routingAPI } from './routing';
import { sessionAPI } from './session';
import { settingsAPI } from './settings';
import { trafficAPI } from './traffic';

export const api = {
  ...sessionAPI,
  ...inboundsAPI,
  ...outboundsAPI,
  ...routingAPI,
  ...trafficAPI,
  ...coreAPI,
  ...settingsAPI,
};

export type API = typeof api;
