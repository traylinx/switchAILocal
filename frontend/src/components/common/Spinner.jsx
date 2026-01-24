import React from 'react';

export function Spinner({ size = 'md' }) {
  const sizePx = size === 'sm' ? '16px' : size === 'lg' ? '32px' : '24px';
  
  return (
    <div style={{
      display: 'inline-block',
      width: sizePx,
      height: sizePx,
      border: '2px solid var(--color-bg-tertiary)',
      borderTopColor: 'var(--color-primary)',
      borderRadius: '50%',
      animation: 'spin 1s linear infinite'
    }} />
  );
}

// Add animation to keyframes (could be in main.css too)
const style = document.createElement('style');
style.innerHTML = `
@keyframes spin {
  to { transform: rotate(360deg); }
}
`;
if (typeof document !== 'undefined') {
  document.head.appendChild(style);
}
