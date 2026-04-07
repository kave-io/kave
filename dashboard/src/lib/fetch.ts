export interface FetchOptions<TBody = unknown> {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  headers?: Record<string, string>
  body?: TBody
  timeout?: number
}

export interface FetchResponse<TData = unknown> {
  ok: boolean
  status: number
  data: TData
  error?: string
}

const DEFAULT_TIMEOUT = 10000
const BASE_URL = import.meta.env.VITE_API_URL || '/api/v1'

export class ApiError extends Error {
  constructor(
    public status: number,
    public data: unknown,
    message: string
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function parseResponse<TData>(response: Response): Promise<TData> {
  const contentType = response.headers.get('content-type')
  if (contentType?.includes('application/json')) {
    return response.json() as Promise<TData>
  }
  return response.text() as Promise<TData>
}

export async function fetch<TData = unknown, TBody = unknown>(
  endpoint: string,
  options: FetchOptions<TBody> = {}
): Promise<TData> {
  const { method = 'GET', headers = {}, body, timeout = DEFAULT_TIMEOUT } = options

  const url = endpoint.startsWith('http') ? endpoint : `${BASE_URL}${endpoint}`
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeout)

  const fetchOptions: RequestInit = {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...headers,
    },
    signal: controller.signal,
  }

  if (body && method !== 'GET') {
    fetchOptions.body = JSON.stringify(body)
  }

  try {
    const response = await globalThis.fetch(url, fetchOptions)
    const data = await parseResponse<TData>(response)

    if (!response.ok) {
      throw new ApiError(
        response.status,
        data,
        `API Error: ${response.status}`
      )
    }

    return data
  } catch (error) {
    if (error instanceof ApiError) {
      throw error
    }

    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError(0, null, 'Request timeout')
    }

    const message = error instanceof Error ? error.message : 'Unknown error'
    throw new ApiError(0, error, message)
  } finally {
    clearTimeout(timeoutId)
  }
}

// Convenience methods
export const api = {
  get: <TData = unknown>(endpoint: string, options?: Omit<FetchOptions, 'method'>) =>
    fetch<TData>(endpoint, { ...options, method: 'GET' }),

  post: <TData = unknown, TBody = unknown>(endpoint: string, body?: TBody, options?: Omit<FetchOptions<TBody>, 'method' | 'body'>) =>
    fetch<TData>(endpoint, { ...options, method: 'POST', body }),

  put: <TData = unknown, TBody = unknown>(endpoint: string, body?: TBody, options?: Omit<FetchOptions<TBody>, 'method' | 'body'>) =>
    fetch<TData>(endpoint, { ...options, method: 'PUT', body }),

  delete: <TData = unknown>(endpoint: string, options?: Omit<FetchOptions, 'method'>) =>
    fetch<TData>(endpoint, { ...options, method: 'DELETE' }),

  patch: <TData = unknown, TBody = unknown>(endpoint: string, body?: TBody, options?: Omit<FetchOptions<TBody>, 'method' | 'body'>) =>
    fetch<TData>(endpoint, { ...options, method: 'PATCH', body }),
}
