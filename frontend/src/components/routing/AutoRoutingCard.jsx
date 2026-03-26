import React from 'react';
import useSWR from 'swr';
import { apiClient } from '../../api/client';
import { Card } from '../common/Card';
import { Spinner } from '../common/Spinner';
import { Activity, Network, TrendingUp, AlertTriangle } from 'lucide-react';

export function AutoRoutingCard() {
  const { data: status, error: statusError } = useSWR('/autoroute/status', () => apiClient.getAutoRouteStatus(), { refreshInterval: 5000 });
  const { data: journal, error: journalError } = useSWR('/autoroute/journal', () => apiClient.getAutoRouteJournal(10), { refreshInterval: 5000 });

  if (!status && !statusError) {
    return (
      <Card>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', color: 'var(--color-text-secondary)' }}>
          <Spinner size="sm" /> Loading Auto-Routing Intelligence...
        </div>
      </Card>
    );
  }

  const enabled = status && status.enabled !== false;
  if (!enabled) {
    return null; // Don't show anything if AutoRouting is completely disabled natively
  }

  const weights = status?.weights || {};
  const currentWindowRQS = status?.window_rqs || 0;
  
  return (
    <Card style={{ marginBottom: 'var(--space-6)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-4)' }}>
        <div style={{ backgroundColor: 'rgba(59, 130, 246, 0.1)', color: 'var(--color-primary)', padding: 'var(--space-2)', borderRadius: 'var(--radius-md)' }}>
          <Network size={20} />
        </div>
        <div>
          <h2 style={{ fontSize: 'var(--text-lg)', fontWeight: 'var(--font-semibold)' }}>Auto-Routing Intelligence</h2>
          <p style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }}>Live Provider Optimization & RQS (Routing Quality Score)</p>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 'var(--space-4)', marginBottom: 'var(--space-6)' }}>
        <div style={{ padding: 'var(--space-4)', backgroundColor: 'var(--color-bg-secondary)', borderRadius: 'var(--radius-md)', border: '1px solid var(--color-border)' }}>
          <div style={{ fontSize: 'var(--text-xs)', textTransform: 'uppercase', color: 'var(--color-text-secondary)', fontWeight: 'var(--font-semibold)', marginBottom: 'var(--space-1)' }}>Global RQS (Window)</div>
          <div style={{ fontSize: 'var(--text-2xl)', fontWeight: 'var(--font-bold)', color: currentWindowRQS > 0 ? 'var(--color-success)' : (currentWindowRQS < 0 ? 'var(--color-error)' : 'var(--color-text)') }}>
            {currentWindowRQS > 0 ? '+' : ''}{currentWindowRQS.toFixed(2)}
          </div>
        </div>
        
        <div style={{ padding: 'var(--space-4)', backgroundColor: 'var(--color-bg-secondary)', borderRadius: 'var(--radius-md)', border: '1px solid var(--color-border)' }}>
          <div style={{ fontSize: 'var(--text-xs)', textTransform: 'uppercase', color: 'var(--color-text-secondary)', fontWeight: 'var(--font-semibold)', marginBottom: 'var(--space-1)' }}>Decisions Rendered</div>
          <div style={{ fontSize: 'var(--text-2xl)', fontWeight: 'var(--font-bold)' }}>{status?.total_decisions || 0}</div>
        </div>
        
        <div style={{ padding: 'var(--space-4)', backgroundColor: 'var(--color-bg-secondary)', borderRadius: 'var(--radius-md)', border: '1px solid var(--color-border)' }}>
          <div style={{ fontSize: 'var(--text-xs)', textTransform: 'uppercase', color: 'var(--color-text-secondary)', fontWeight: 'var(--font-semibold)', marginBottom: 'var(--space-1)' }}>Active Hypothesis</div>
          <div style={{ fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)' }}>
            {status?.hypothesis_active ? (
              <span style={{ color: 'var(--color-primary)', display: 'flex', alignItems: 'center', gap: '4px' }}><Activity size={14}/> Testing Variation</span>
            ) : (
              <span style={{ color: 'var(--color-text-tertiary)' }}>Baseline Stability</span>
            )}
          </div>
        </div>
      </div>

      <h3 style={{ fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)', marginBottom: 'var(--space-3)' }}>Provider Weights</h3>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)', marginBottom: 'var(--space-6)' }}>
        {Object.entries(weights).map(([provider, weight]) => (
          <div key={provider} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
            <div style={{ width: '120px', fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)' }}>{provider}</div>
            <div style={{ flex: 1, height: '8px', backgroundColor: 'var(--color-bg-secondary)', borderRadius: '4px', overflow: 'hidden' }}>
              <div style={{ height: '100%', width: `${Math.min(100, Math.max(0, weight * 100))}%`, backgroundColor: 'var(--color-primary)', transition: 'width 0.5s ease-out' }}></div>
            </div>
            <div style={{ width: '60px', fontSize: 'var(--text-xs)', fontFamily: 'var(--font-mono)', textAlign: 'right', color: 'var(--color-text-secondary)' }}>
              {weight.toFixed(3)}
            </div>
          </div>
        ))}
      </div>

      <h3 style={{ fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)', marginBottom: 'var(--space-3)' }}>Recent Routing Decisions</h3>
      {journal?.entries?.length > 0 ? (
        <div style={{ overflowX: 'auto', border: '1px solid var(--color-border)', borderRadius: 'var(--radius-md)' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: 'var(--text-xs)' }}>
            <thead style={{ backgroundColor: 'var(--color-bg-secondary)', borderBottom: '1px solid var(--color-border)' }}>
              <tr>
                <th style={{ padding: 'var(--space-2) var(--space-3)', fontWeight: 'var(--font-semibold)' }}>Time</th>
                <th style={{ padding: 'var(--space-2) var(--space-3)', fontWeight: 'var(--font-semibold)' }}>Model</th>
                <th style={{ padding: 'var(--space-2) var(--space-3)', fontWeight: 'var(--font-semibold)' }}>Selected Provider</th>
                <th style={{ padding: 'var(--space-2) var(--space-3)', fontWeight: 'var(--font-semibold)' }}>Outcome/RQS</th>
              </tr>
            </thead>
            <tbody>
              {journal.entries.map((entry, i) => (
                <tr key={i} style={{ borderBottom: i < journal.entries.length - 1 ? '1px solid var(--color-border)' : 'none' }}>
                  <td style={{ padding: 'var(--space-2) var(--space-3)', color: 'var(--color-text-secondary)' }}>{new Date(entry.timestamp).toLocaleTimeString()}</td>
                  <td style={{ padding: 'var(--space-2) var(--space-3)', fontFamily: 'var(--font-mono)' }}>{entry.model}</td>
                  <td style={{ padding: 'var(--space-2) var(--space-3)', fontWeight: 'var(--font-medium)' }}>{entry.routed_provider}</td>
                  <td style={{ padding: 'var(--space-2) var(--space-3)' }}>
                    {entry.error ? (
                      <span style={{ color: 'var(--color-error)', display: 'flex', alignItems: 'center', gap: '4px' }}><AlertTriangle size={12}/> Failed</span>
                    ) : (
                      <span style={{ color: entry.success ? 'var(--color-success)' : 'var(--color-text-secondary)' }}>
                        +{entry.tokens_per_second.toFixed(1)} t/s
                        {entry.rqs_delta && ` (RQS: ${entry.rqs_delta > 0 ? '+' : ''}${entry.rqs_delta.toFixed(2)})`}
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div style={{ padding: 'var(--space-4)', textAlign: 'center', color: 'var(--color-text-tertiary)', border: '1px dashed var(--color-border)', borderRadius: 'var(--radius-md)', fontSize: 'var(--text-sm)' }}>
          No recent auto-routing decisions recorded yet in this cycle.
        </div>
      )}
    </Card>
  );
}
