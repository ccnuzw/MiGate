import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { Gauge } from 'lucide-react';
import { SpinnerButton } from './ui';

let root: Root | null = null;
let container: HTMLDivElement | null = null;

afterEach(() => {
  if (root) {
    act(() => root?.unmount());
  }
  root = null;
  container?.remove();
  container = null;
});

describe('SpinnerButton', () => {
  it('replaces the action icon with the loading spinner', () => {
    render(<SpinnerButton loading><Gauge data-testid="action-icon" className="h-4 w-4" />测速</SpinnerButton>);

    const button = document.querySelector('button');
    expect(button).toHaveTextContent('测速');
    expect(button?.querySelector('[data-testid="action-icon"]')).toBeNull();
    expect(button?.querySelector('.animate-spin')).toBeInTheDocument();
  });
});

function render(node: React.ReactNode) {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => {
    root!.render(node);
  });
}
