import '@testing-library/jest-dom/vitest';

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const values = new Map<string, string>();

Object.defineProperty(globalThis, 'localStorage', {
  value: {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key: string) => values.get(String(key)) ?? null,
    key: (index: number) => {
      const normalized = Math.trunc(Number(index));
      if (!Number.isFinite(normalized) || normalized < 0) return null;
      return Array.from(values.keys())[normalized] ?? null;
    },
    removeItem: (key: string) => {
      values.delete(String(key));
    },
    setItem: (key: string, value: string) => {
      values.set(String(key), String(value));
    },
  },
  configurable: true,
});
