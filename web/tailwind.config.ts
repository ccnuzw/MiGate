import type { Config } from 'tailwindcss';

export default {
  darkMode: ['class', '[data-theme="dark"]'],
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        panel: {
          bg: 'rgb(var(--bg))',
          surface: 'rgb(var(--surface))',
          muted: 'rgb(var(--muted))',
          line: 'rgb(var(--line))',
          text: 'rgb(var(--text))',
          soft: 'rgb(var(--soft))',
        },
      },
      boxShadow: {
        panel: '0 1px 2px rgba(16, 24, 40, 0.06), 0 0 0 1px rgb(var(--line) / 1)',
      },
    },
  },
  plugins: [],
} satisfies Config;
