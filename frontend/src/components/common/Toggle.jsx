import React from 'react';

export function Toggle({ active, onChange }) {
  return (
    <div 
      className={`toggle ${active ? 'active' : ''}`} 
      onClick={() => onChange(!active)}
      role="switch"
      aria-checked={active}
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === ' ' || e.key === 'Enter') {
          e.preventDefault();
          onChange(!active);
        }
      }}
    >
      <div className="toggle-thumb" />
    </div>
  );
}
