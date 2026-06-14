import type { Config } from 'tailwindcss';

export default {
  darkMode: ['class', '[data-theme="dark"]'],
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        panel: {
          bg: 'rgb(var(--bg) / <alpha-value>)',
          surface: 'rgb(var(--surface) / <alpha-value>)',
          muted: 'rgb(var(--muted) / <alpha-value>)',
          line: 'rgb(var(--line) / <alpha-value>)',
          text: 'rgb(var(--text) / <alpha-value>)',
          soft: 'rgb(var(--soft) / <alpha-value>)',
        },
      },
      boxShadow: {
        panel: '0 1px 2px rgba(16, 24, 40, 0.06), 0 0 0 1px rgb(var(--line) / 1)',
      },
    },
  },
  plugins: [],
} satisfies Config;
