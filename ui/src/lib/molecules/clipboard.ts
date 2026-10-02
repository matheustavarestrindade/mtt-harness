export async function copyToClipboard(text: string): Promise<void> {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text);
    return;
  }
  // The deployed Tailscale URL can use HTTP, where the Clipboard API is unavailable.
  const previousFocus = document.activeElement;
  const field = document.createElement('textarea');
  field.value = text;
  field.style.position = 'fixed';
  field.style.opacity = '0';
  field.readOnly = true;
  document.body.append(field);
  field.focus({ preventScroll: true });
  field.select();
  try {
    if (!document.execCommand('copy'))
      throw new Error('Clipboard access is unavailable. Select the text to copy it.');
  } finally {
    field.remove();
    if (previousFocus instanceof HTMLElement) previousFocus.focus({ preventScroll: true });
  }
}
