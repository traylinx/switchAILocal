import React from 'react';

export function Button({ 
  children, 
  variant = 'secondary', 
  size = 'md', 
  className = '', 
  ...props 
}) {
  const baseClass = 'button';
  const variantClass = `button-${variant}`;
  const sizeClass = size === 'sm' ? 'button-sm' : '';
  
  return (
    <button 
      className={`${baseClass} ${variantClass} ${sizeClass} ${className}`} 
      {...props}
    >
      {children}
    </button>
  );
}
