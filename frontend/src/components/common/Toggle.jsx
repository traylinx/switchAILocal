import React from 'react';

export function Toggle({ active, onChange, disabled = false }) {
  const handleClick = () => {
    if (!disabled) {
      onChange(!active);
    }
  };

  return (
    <div 
      className={`toggle ${active ? 'active' : ''} ${disabled ? 'disabled' : ''}`} 
      onClick={handleClick}
      role="switch"
      aria-checked={active}
      aria-disabled={disabled}
      tabIndex={disabled ? -1 : 0}
      onKeyDown={(e) => {
        if (!disabled && (e.key === ' ' || e.key === 'Enter')) {
          e.preventDefault();
          onChange(!active);
        }
      }}
      style={{ cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.5 : 1 }}
    >
      <div className="toggle-thumb" />
    </div>
  );
}
