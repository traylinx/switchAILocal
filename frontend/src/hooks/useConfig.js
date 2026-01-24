import useSWR from 'swr';
import { apiClient } from '../api/client';
import { useConfigStore } from '../stores/configStore';
import { useEffect } from 'react';

export function useConfig() {
  const { setConfig } = useConfigStore();
  
  const { data, error, mutate, isLoading } = useSWR(
    '/config',
    () => apiClient.getConfig(),
    {
      revalidateOnFocus: false,
      revalidateOnReconnect: true,
    }
  );
  
  // Update store when data arrives
  useEffect(() => {
    if (data) {
      setConfig(data);
    }
  }, [data, setConfig]);
  
  const updateConfig = async (updates) => {
    const previousData = data;
    // Optimistic update
    mutate({ ...data, ...updates }, false);
    
    try {
      await apiClient.updateConfig({ ...data, ...updates });
      mutate(); // Revalidate to be sure
    } catch (error) {
      // Revert on error
      mutate(previousData, false);
      throw error;
    }
  };
  
  return {
    config: data,
    isLoading,
    isError: error,
    updateConfig,
  };
}
