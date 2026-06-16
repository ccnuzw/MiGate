import { afterEach, describe, expect, it, vi } from 'vitest';
import { copyToClipboard } from './clipboard';

const clipboardDescriptor = Object.getOwnPropertyDescriptor(Navigator.prototype, 'clipboard');
const execCommandDescriptor = Object.getOwnPropertyDescriptor(Document.prototype, 'execCommand');

afterEach(() => {
  if (clipboardDescriptor) {
    Object.defineProperty(Navigator.prototype, 'clipboard', clipboardDescriptor);
  } else {
    delete (Navigator.prototype as { clipboard?: Clipboard }).clipboard;
  }
  if (execCommandDescriptor) {
    Object.defineProperty(Document.prototype, 'execCommand', execCommandDescriptor);
  } else {
    delete (Document.prototype as { execCommand?: Document['execCommand'] }).execCommand;
  }
  vi.restoreAllMocks();
});

describe('copyToClipboard', () => {
  it('uses the Clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(Navigator.prototype, 'clipboard', {
      configurable: true,
      get: () => ({ writeText }),
    });
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(Document.prototype, 'execCommand', { configurable: true, value: execCommand });

    await copyToClipboard('vless://client');

    expect(writeText).toHaveBeenCalledWith('vless://client');
    expect(execCommand).not.toHaveBeenCalled();
  });

  it('falls back to a hidden textarea when the Clipboard API is unavailable', async () => {
    Object.defineProperty(Navigator.prototype, 'clipboard', {
      configurable: true,
      get: () => undefined,
    });
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(Document.prototype, 'execCommand', { configurable: true, value: execCommand });

    await copyToClipboard('vless://fallback');

    expect(execCommand).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('throws when the fallback copy command fails', async () => {
    Object.defineProperty(Navigator.prototype, 'clipboard', {
      configurable: true,
      get: () => undefined,
    });
    Object.defineProperty(Document.prototype, 'execCommand', { configurable: true, value: vi.fn().mockReturnValue(false) });

    await expect(copyToClipboard('vless://failed')).rejects.toThrow('copy_command_failed');
  });
});
