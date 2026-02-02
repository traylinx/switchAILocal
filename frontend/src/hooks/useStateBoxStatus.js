import useSWR from 'swr';
import { apiClient } from '../api/client';

export function useStateBoxStatus() {
  const { data, error, isLoading } = useSWR(
    '/state-box/status',
    () => apiClient.getStateBoxStatus(),
    {
      revalidateOnFocus: false,
      revalidateOnReconnect: true,
      refreshInterval: 30000, // Refresh every 30 seconds
    }
  );
  
  return {
    status: data,
    isLoading,
    error,
  };
}
