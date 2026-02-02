import React, { useState } from 'react';
import { useStateBoxStatus } from '../../hooks/useStateBoxStatus';
import { Lock } from 'lucide-react';

export function ReadOnlyIndicator() {
  const { status } = useStateBoxStatus();
  const [showTooltip, setShowTooltip] = useState(false);

  if (!status?.read_only) {
    return null;
  }

  return (
    <div 
      className="read-only-indicator"
      onMouseEnter={() => setShowTooltip(true)}
      onMouseLeave={() => setShowTooltip(false)}
    >
      <Lock size={14} />
      <span>Read-Only Mode</span>
      {showTooltip && (
        <div className="read-only-tooltip">
          The system is running in read-only mode. All save and edit operations are disabled.
          This typically occurs in immutable environments or when SWITCHAI_READONLY=1 is set.
        </div>
      )}
    </div>
  );
}
