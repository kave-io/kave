type RuntimeEnv = Record<string, string | undefined>

export const runtimeEnv: RuntimeEnv =
  ((import.meta as unknown as { env?: RuntimeEnv }).env ?? {})
