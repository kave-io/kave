import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { PriceBook } from '@/types/api'
import { settingsClient } from '@/lib/rpc/clients'

export function usePricingBook() {
  return useQuery({
    queryKey: ['settings', 'pricing'],
    queryFn: () => settingsClient.getPricing(),
  })
}

export function useSavePricingBook() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: PriceBook) => settingsClient.savePricing(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['settings', 'pricing'] }),
  })
}
